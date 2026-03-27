package runtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	tocsync "github.com/tiny-oc/toc/internal/sync"
)

const (
	codexInstructionFile = "AGENTS.md"
	codexEventsFile      = "toc-codex-events.jsonl"
	codexMetadataFile    = ".toc-codex-session.json"
)

type codexProvider struct{}

type codexSessionMetadata struct {
	ConversationID string `json:"conversation_id,omitempty"`
	LogPath        string `json:"log_path,omitempty"`
}

func (codexProvider) Name() string { return runtimeinfo.CodexRuntime }

func (codexProvider) DefaultModel() string { return runtimeinfo.DefaultModel(runtimeinfo.CodexRuntime) }

func (codexProvider) ModelOptions() []ModelOption {
	options := runtimeinfo.ModelOptions(runtimeinfo.CodexRuntime)
	result := make([]ModelOption, 0, len(options))
	for _, opt := range options {
		result = append(result, ModelOption{
			ID:          opt.ID,
			Label:       opt.Label,
			Description: opt.Description,
		})
	}
	return result
}

func (codexProvider) ValidateModel(model string) error {
	return runtimeinfo.ValidateModel(runtimeinfo.CodexRuntime, model)
}

func (codexProvider) PrepareSession(workDir, agentDir string, cfg *SessionConfig, sessionID string) error {
	content, err := ComposePrompt(workDir, cfg, sessionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) != "" {
		agentsPath := filepath.Join(workDir, codexInstructionFile)
		if err := os.WriteFile(agentsPath, []byte(content+"\n"), 0644); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join(workDir, "agent.md")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return ensureCodexGitRepo(workDir)
}

func (codexProvider) SkillsDir(workDir string) string {
	return filepath.Join(workDir, ".codex", "skills")
}

func (codexProvider) PostSessionSync(workDir, agentDir string, patterns []string) ([]string, error) {
	return tocsync.SyncBackWithOptions(workDir, agentDir, patterns, tocsync.Options{
		PathMapper: codexInstructionPathMapper,
		SkipDirs:   map[string]bool{".codex": true},
	})
}

func (codexProvider) LaunchInteractive(opts LaunchOptions) error {
	var cmd *exec.Cmd

	if opts.Prompt != "" {
		args, err := buildCodexExecArgs(opts.Dir, opts.Model, opts.Prompt, opts.Resume, opts.SessionID, "")
		if err != nil {
			return err
		}
		cmd = exec.Command("codex", args...)
	} else {
		args, err := buildCodexInteractiveArgs(opts.Dir, opts.Model, opts.Resume, opts.SessionID)
		if err != nil {
			return err
		}
		cmd = exec.Command("codex", args...)
	}

	cmd.Dir = opts.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"TOC_WORKSPACE="+opts.Workspace,
		"TOC_AGENT="+opts.AgentName,
		"TOC_SESSION_ID="+opts.SessionID,
	)

	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			_ = refreshCodexSessionMetadata(opts.Dir, opts.SessionID, time.Time{})
			return nil
		}
		return fmt.Errorf("failed to launch codex: %w", err)
	}
	return refreshCodexSessionMetadata(opts.Dir, opts.SessionID, time.Time{})
}

func (codexProvider) LaunchDetached(opts DetachedOptions) error {
	promptPath := filepath.Join(opts.Dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0644); err != nil {
		return err
	}

	var resumeConversationID string
	if opts.Resume {
		id, err := resolveCodexConversationID(opts.Dir, opts.SessionID)
		if err != nil {
			return err
		}
		resumeConversationID = id
	}

	scriptPath := filepath.Join(opts.Dir, "toc-run.sh")
	helperExe, err := os.Executable()
	if err != nil {
		return err
	}
	script := BuildCodexDetachedScript(helperExe, opts, promptPath, resumeConversationID)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}

	cmd := exec.Command("sh", scriptPath)
	cmd.Dir = opts.Dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (codexProvider) SessionLogPath(sess *session.Session) string {
	if sess == nil {
		return ""
	}

	localPath := filepath.Join(sess.WorkspacePath, codexEventsFile)
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	meta, _ := loadCodexSessionMetadata(sess.WorkspacePath)
	if meta != nil {
		if meta.LogPath != "" {
			if _, err := os.Stat(meta.LogPath); err == nil {
				return meta.LogPath
			}
		}
		if meta.ConversationID != "" {
			if logPath := findCodexLogByConversationID(meta.ConversationID); logPath != "" {
				meta.LogPath = logPath
				_ = saveCodexSessionMetadata(sess.WorkspacePath, meta)
				return logPath
			}
		}
	}

	logPath, conversationID := findCodexLogForWorkspace(sess.WorkspacePath, sess.CreatedAt)
	if logPath == "" {
		return ""
	}
	_ = saveCodexSessionMetadata(sess.WorkspacePath, &codexSessionMetadata{
		ConversationID: conversationID,
		LogPath:        logPath,
	})
	return logPath
}

func (codexProvider) ExpectedSessionLogPath(sess *session.Session) string {
	if sess == nil {
		return ""
	}
	if sess.ParentSessionID != "" {
		return filepath.Join(sess.WorkspacePath, codexEventsFile)
	}
	return codexProvider{}.SessionLogPath(sess)
}

func (codexProvider) ParseSessionLog(path string) (*ParsedLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := &ParsedLog{}
	pending := map[string]codexRolloutFunctionCall{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	for scanner.Scan() {
		events := parseCodexSessionLine(scanner.Bytes(), pending)
		for _, event := range events {
			result.Events = append(result.Events, event)
			result.Steps = append(result.Steps, event.Step)
			if !event.Timestamp.IsZero() {
				if result.FirstTS.IsZero() {
					result.FirstTS = event.Timestamp
				}
				result.LastTS = event.Timestamp
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (codexProvider) ParseSessionLogLineEvents(line []byte) []Event {
	return parseCodexSessionLine(line, nil)
}

func ensureCodexGitRepo(workDir string) error {
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err == nil {
		return nil
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize git repo for codex runtime: %w", err)
	}
	return nil
}

func buildCodexInteractiveArgs(workDir, model string, resume bool, tocSessionID string) ([]string, error) {
	if !resume {
		args := append([]string{}, codexModelArgs(model)...)
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		return args, nil
	}

	conversationID, err := resolveCodexConversationID(workDir, tocSessionID)
	if err != nil {
		return nil, err
	}
	args := []string{"resume"}
	args = append(args, codexModelArgs(model)...)
	args = append(args, "--dangerously-bypass-approvals-and-sandbox", conversationID)
	return args, nil
}

func buildCodexExecArgs(workDir, model, prompt string, resume bool, tocSessionID, outputPath string) ([]string, error) {
	args := []string{"exec"}
	if resume {
		conversationID, err := resolveCodexConversationID(workDir, tocSessionID)
		if err != nil {
			return nil, err
		}
		args = append(args, "resume")
		args = append(args, codexModelArgs(model)...)
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		if outputPath != "" {
			args = append(args, "-o", outputPath)
		}
		args = append(args, conversationID, prompt)
		return args, nil
	}

	args = append(args, codexModelArgs(model)...)
	args = append(args, "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox")
	if outputPath != "" {
		args = append(args, "-o", outputPath)
	}
	args = append(args, prompt)
	return args, nil
}

func codexModelArgs(model string) []string {
	if strings.TrimSpace(model) == "" {
		return nil
	}
	return []string{"-m", model}
}

func BuildCodexDetachedScript(helperExecutable string, opts DetachedOptions, promptPath, resumeConversationID string) string {
	outputTmpPath := opts.OutputPath + ".tmp"
	eventsPath := filepath.Join(opts.Dir, codexEventsFile)
	stderrPath := filepath.Join(opts.Dir, "toc-codex-stderr.txt")
	pidPath := filepath.Join(opts.Dir, "toc-pid.txt")
	exitCodePath := filepath.Join(opts.Dir, "toc-exit-code.txt")
	notifyCommand := ""
	if opts.ParentSessionID != "" {
		notifyCommand = fmt.Sprintf("%q __notify-subagent-complete --workspace %q --parent-session-id %q --session-id %q --agent %q --prompt-file %q --output-file %q --exit-code-file %q >/dev/null 2>&1 || true\n",
			helperExecutable, opts.Workspace, opts.ParentSessionID, opts.SessionID, opts.AgentName, promptPath, opts.OutputPath, exitCodePath)
	}

	command := fmt.Sprintf("codex exec --dangerously-bypass-approvals-and-sandbox -m %q -o %q", opts.Model, outputTmpPath)
	if !opts.Resume {
		command += " --skip-git-repo-check"
		command += fmt.Sprintf(" - < %q", promptPath)
	} else {
		command = fmt.Sprintf("codex exec resume --dangerously-bypass-approvals-and-sandbox -m %q -o %q %q - < %q",
			opts.Model, outputTmpPath, resumeConversationID, promptPath)
	}

	return fmt.Sprintf(`#!/bin/sh
echo $$ > %q
cd %q
export TOC_WORKSPACE=%q
export TOC_AGENT=%q
export TOC_SESSION_ID=%q
%s > %q 2> %q
TOC_EXIT=$?
echo $TOC_EXIT > %q
if [ ! -f %q ]; then
  : > %q
fi
mv %q %q
%s
`, pidPath, opts.Dir, opts.Workspace, opts.AgentName, opts.SessionID, command, eventsPath, stderrPath, exitCodePath, outputTmpPath, outputTmpPath, outputTmpPath, opts.OutputPath, notifyCommand)
}

func codexInstructionPathMapper(rel string) string {
	if rel == codexInstructionFile {
		return "agent.md"
	}
	return rel
}

func resolveCodexConversationID(workDir, tocSessionID string) (string, error) {
	meta, _ := loadCodexSessionMetadata(workDir)
	if meta != nil && meta.ConversationID != "" {
		return meta.ConversationID, nil
	}

	localEventsPath := filepath.Join(workDir, codexEventsFile)
	if conversationID := parseCodexThreadIDFromExecLog(localEventsPath); conversationID != "" {
		_ = saveCodexSessionMetadata(workDir, &codexSessionMetadata{ConversationID: conversationID})
		return conversationID, nil
	}

	logPath, conversationID := findCodexLogForWorkspace(workDir, time.Time{})
	if conversationID != "" {
		_ = saveCodexSessionMetadata(workDir, &codexSessionMetadata{
			ConversationID: conversationID,
			LogPath:        logPath,
		})
		return conversationID, nil
	}

	return "", fmt.Errorf("could not resolve codex conversation for toc session %q", tocSessionID)
}

func refreshCodexSessionMetadata(workDir, tocSessionID string, createdAt time.Time) error {
	meta, _ := loadCodexSessionMetadata(workDir)
	if meta == nil {
		meta = &codexSessionMetadata{}
	}
	if meta.ConversationID == "" {
		meta.ConversationID = parseCodexThreadIDFromExecLog(filepath.Join(workDir, codexEventsFile))
	}
	if meta.LogPath == "" {
		logPath, conversationID := findCodexLogForWorkspace(workDir, createdAt)
		if conversationID != "" && meta.ConversationID == "" {
			meta.ConversationID = conversationID
		}
		meta.LogPath = logPath
	}
	if meta.ConversationID == "" && meta.LogPath == "" {
		return nil
	}
	return saveCodexSessionMetadata(workDir, meta)
}

func saveCodexSessionMetadata(workDir string, meta *codexSessionMetadata) error {
	if meta == nil {
		return nil
	}
	path := filepath.Join(workDir, codexMetadataFile)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func loadCodexSessionMetadata(workDir string) (*codexSessionMetadata, error) {
	data, err := os.ReadFile(filepath.Join(workDir, codexMetadataFile))
	if err != nil {
		return nil, err
	}
	var meta codexSessionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func parseCodexThreadIDFromExecLog(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			return event.ThreadID
		}
	}
	return ""
}

func findCodexLogByConversationID(conversationID string) string {
	if conversationID == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	patterns := []string{
		filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*"+conversationID+".jsonl"),
		filepath.Join(home, ".codex", "archived_sessions", "*"+conversationID+".jsonl"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) == 0 {
			continue
		}
		sort.Slice(matches, func(i, j int) bool {
			infoI, errI := os.Stat(matches[i])
			infoJ, errJ := os.Stat(matches[j])
			if errI != nil || errJ != nil {
				return matches[i] > matches[j]
			}
			return infoI.ModTime().After(infoJ.ModTime())
		})
		return matches[0]
	}
	return ""
}

func findCodexLogForWorkspace(workDir string, createdAt time.Time) (string, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}

	workspaceCandidates := codexWorkspaceCandidates(workDir)
	searchRoots := []string{
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, ".codex", "archived_sessions"),
	}

	var bestPath string
	var bestConversationID string
	var bestTime time.Time

	for _, root := range searchRoots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}
			if !createdAt.IsZero() && info.ModTime().Before(createdAt.Add(-2*time.Minute)) {
				return nil
			}

			conversationID, cwd, sessionTS := readCodexSessionMeta(path)
			if cwd == "" || !matchesCodexWorkspace(cwd, workspaceCandidates) {
				return nil
			}
			candidateTime := info.ModTime()
			if !sessionTS.IsZero() {
				candidateTime = sessionTS
			}
			if bestPath == "" || candidateTime.After(bestTime) {
				bestPath = path
				bestConversationID = conversationID
				bestTime = candidateTime
			}
			return nil
		})
	}

	return bestPath, bestConversationID
}

func codexWorkspaceCandidates(workspacePath string) []string {
	candidates := []string{workspacePath}
	if resolved, err := filepath.EvalSymlinks(workspacePath); err == nil && resolved != workspacePath {
		candidates = append(candidates, resolved)
	}
	if !strings.HasPrefix(workspacePath, "/private") {
		candidates = append(candidates, "/private"+workspacePath)
	}
	return candidates
}

func matchesCodexWorkspace(cwd string, candidates []string) bool {
	for _, candidate := range candidates {
		if cwd == candidate {
			return true
		}
	}
	return false
}

func readCodexSessionMeta(path string) (string, string, time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", time.Time{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	if !scanner.Scan() {
		return "", "", time.Time{}
	}

	var line struct {
		Timestamp string `json:"timestamp,omitempty"`
		Type      string `json:"type"`
		Payload   struct {
			ID        string `json:"id,omitempty"`
			Timestamp string `json:"timestamp,omitempty"`
			Cwd       string `json:"cwd,omitempty"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
		return "", "", time.Time{}
	}

	if line.Type == "thread.started" {
		var execLine struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &execLine); err == nil {
			return execLine.ThreadID, "", time.Time{}
		}
	}

	ts := parseRFC3339Time(line.Payload.Timestamp)
	if ts.IsZero() {
		ts = parseRFC3339Time(line.Timestamp)
	}
	return line.Payload.ID, line.Payload.Cwd, ts
}

func parseRFC3339Time(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return ts
}

type codexRolloutFunctionCall struct {
	Name      string
	Command   string
	Patch     string
	CallID    string
	Timestamp time.Time
}

func parseCodexSessionLine(line []byte, pending map[string]codexRolloutFunctionCall) []Event {
	var head struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp,omitempty"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return nil
	}

	switch head.Type {
	case "thread.started", "turn.started", "turn.completed", "turn.failed", "item.started", "item.updated", "item.completed", "error":
		return parseCodexExecLine(line)
	case "session_meta", "event_msg", "response_item", "turn_context":
		return parseCodexRolloutLine(line, pending)
	default:
		return nil
	}
}

func parseCodexExecLine(line []byte) []Event {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return nil
	}

	switch head.Type {
	case "item.completed":
		var item struct {
			Type   string `json:"type"`
			ItemID string `json:"id,omitempty"`
			Item   struct {
				ID               string `json:"id,omitempty"`
				Type             string `json:"type"`
				Text             string `json:"text,omitempty"`
				Command          string `json:"command,omitempty"`
				AggregatedOutput string `json:"aggregated_output,omitempty"`
				ExitCode         *int   `json:"exit_code,omitempty"`
				Status           string `json:"status,omitempty"`
				Query            string `json:"query,omitempty"`
				Tool             string `json:"tool,omitempty"`
				Message          string `json:"message,omitempty"`
				Changes          []struct {
					Path string `json:"path"`
					Kind string `json:"kind"`
				} `json:"changes,omitempty"`
			} `json:"item"`
		}
		if err := json.Unmarshal(line, &item); err != nil {
			return nil
		}
		return codexExecItemToEvents(item.Item)
	case "turn.failed":
		var failed struct {
			Type  string `json:"type"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &failed); err != nil {
			return nil
		}
		return []Event{{Step: Step{Type: "error", Content: failed.Error.Message}}}
	case "error":
		var failed struct {
			Type    string `json:"type"`
			Message string `json:"message,omitempty"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &failed); err != nil {
			return nil
		}
		msg := failed.Message
		if msg == "" {
			msg = failed.Error.Message
		}
		if msg == "" {
			return nil
		}
		return []Event{{Step: Step{Type: "error", Content: msg}}}
	default:
		return nil
	}
}

func codexExecItemToEvents(item struct {
	ID               string `json:"id,omitempty"`
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	Command          string `json:"command,omitempty"`
	AggregatedOutput string `json:"aggregated_output,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Status           string `json:"status,omitempty"`
	Query            string `json:"query,omitempty"`
	Tool             string `json:"tool,omitempty"`
	Message          string `json:"message,omitempty"`
	Changes          []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes,omitempty"`
}) []Event {
	switch item.Type {
	case "agent_message":
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return []Event{{Step: Step{Type: "text", Content: strings.TrimSpace(item.Text)}}}
	case "reasoning":
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return []Event{{Step: Step{Type: "thinking", Content: strings.TrimSpace(item.Text)}}}
	case "command_execution":
		step := Step{
			Type:    "tool",
			Tool:    "Bash",
			Command: item.Command,
			Content: strings.TrimSpace(item.AggregatedOutput),
		}
		if item.ExitCode != nil {
			step.ExitCode = *item.ExitCode
			step.Success = boolPtr(*item.ExitCode == 0)
		} else if item.Status == "declined" {
			step.Success = boolPtr(false)
		}
		return []Event{{Step: step}}
	case "file_change":
		if len(item.Changes) == 0 {
			return nil
		}
		events := make([]Event, 0, len(item.Changes))
		for _, change := range item.Changes {
			toolName := "Write"
			if change.Kind == "update" {
				toolName = "Edit"
			}
			events = append(events, Event{Step: Step{
				Type:    "tool",
				Tool:    toolName,
				Path:    change.Path,
				Content: change.Kind,
				Success: boolPtr(item.Status == "completed"),
			}})
		}
		return events
	case "mcp_tool_call":
		return []Event{{Step: Step{Type: "tool", Tool: item.Tool, Content: strings.TrimSpace(item.Message)}}}
	case "web_search":
		return []Event{{Step: Step{Type: "tool", Tool: "WebSearch", Content: strings.TrimSpace(item.Query)}}}
	case "collab_tool_call":
		return []Event{{Step: Step{Type: "tool", Tool: "SubAgent", Content: strings.TrimSpace(item.Tool)}}}
	case "error":
		msg := strings.TrimSpace(item.Message)
		if msg == "" {
			msg = strings.TrimSpace(item.Text)
		}
		if msg == "" {
			return nil
		}
		return []Event{{Step: Step{Type: "error", Content: msg}}}
	default:
		return nil
	}
}

func parseCodexRolloutLine(line []byte, pending map[string]codexRolloutFunctionCall) []Event {
	var envelope struct {
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp,omitempty"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil
	}
	ts := parseRFC3339Time(envelope.Timestamp)

	switch envelope.Type {
	case "event_msg":
		var payload struct {
			Type    string `json:"type"`
			Text    string `json:"text,omitempty"`
			Message string `json:"message,omitempty"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return nil
		}
		switch payload.Type {
		case "agent_reasoning":
			if strings.TrimSpace(payload.Text) == "" {
				return nil
			}
			return []Event{{Timestamp: ts, Step: Step{Type: "thinking", Content: strings.TrimSpace(payload.Text)}}}
		case "agent_message":
			if strings.TrimSpace(payload.Message) == "" {
				return nil
			}
			return []Event{{Timestamp: ts, Step: Step{Type: "text", Content: strings.TrimSpace(payload.Message)}}}
		default:
			return nil
		}
	case "response_item":
		var payload struct {
			Type      string `json:"type"`
			Name      string `json:"name,omitempty"`
			Arguments string `json:"arguments,omitempty"`
			CallID    string `json:"call_id,omitempty"`
			Output    string `json:"output,omitempty"`
			Role      string `json:"role,omitempty"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"content,omitempty"`
		}
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return nil
		}
		switch payload.Type {
		case "function_call":
			if pending == nil || payload.CallID == "" {
				return nil
			}
			call := codexRolloutFunctionCall{Name: payload.Name, CallID: payload.CallID, Timestamp: ts}
			var args struct {
				Cmd     string `json:"cmd,omitempty"`
				Command string `json:"command,omitempty"`
			}
			if err := json.Unmarshal([]byte(payload.Arguments), &args); err == nil {
				call.Command = args.Cmd
				if call.Command == "" {
					call.Command = args.Command
				}
			}
			if payload.Name == "apply_patch" {
				call.Patch = payload.Arguments
			}
			pending[payload.CallID] = call
			return nil
		case "function_call_output":
			if pending == nil || payload.CallID == "" {
				return nil
			}
			call, ok := pending[payload.CallID]
			if !ok {
				return nil
			}
			delete(pending, payload.CallID)
			step := codexRolloutCallOutputToStep(call, payload.Output)
			if step.Type == "" {
				return nil
			}
			return []Event{{Timestamp: ts, Step: step}}
		case "message":
			if payload.Role != "assistant" {
				return nil
			}
			var parts []string
			for _, item := range payload.Content {
				if item.Type == "output_text" && strings.TrimSpace(item.Text) != "" {
					parts = append(parts, strings.TrimSpace(item.Text))
				}
			}
			if len(parts) == 0 {
				return nil
			}
			return []Event{{Timestamp: ts, Step: Step{Type: "text", Content: strings.Join(parts, "\n\n")}}}
		default:
			return nil
		}
	default:
		return nil
	}
}

func codexRolloutCallOutputToStep(call codexRolloutFunctionCall, output string) Step {
	switch call.Name {
	case "shell_command", "exec_command":
		exitCode, durationMS, body := parseCodexCommandOutput(output)
		step := Step{
			Type:       "tool",
			Tool:       "Bash",
			Command:    call.Command,
			Content:    body,
			ExitCode:   exitCode,
			DurationMS: durationMS,
		}
		step.Success = boolPtr(exitCode == 0)
		return step
	case "apply_patch":
		success := !strings.Contains(strings.ToLower(output), "error")
		return Step{
			Type:    "tool",
			Tool:    "Edit",
			Content: strings.TrimSpace(output),
			Success: boolPtr(success),
		}
	default:
		return Step{}
	}
}

func parseCodexCommandOutput(output string) (int, int64, string) {
	lines := strings.Split(output, "\n")
	exitCode := 0
	var durationMS int64
	var body []string
	inBody := false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "Exit code:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "Exit code:"))
			if parsed, err := strconv.Atoi(value); err == nil {
				exitCode = parsed
			}
		case strings.HasPrefix(line, "Wall time:"):
			value := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "Wall time:"), "seconds"))
			value = strings.TrimSpace(value)
			if seconds, err := strconv.ParseFloat(value, 64); err == nil {
				durationMS = int64(seconds * 1000)
			}
		case strings.TrimSpace(line) == "Output:":
			inBody = true
		default:
			if inBody {
				body = append(body, line)
			}
		}
	}

	return exitCode, durationMS, strings.TrimSpace(strings.Join(body, "\n"))
}
