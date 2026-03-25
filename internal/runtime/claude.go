package runtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	tocsync "github.com/tiny-oc/toc/internal/sync"
)

type claudeProvider struct{}

func (claudeProvider) Name() string { return DefaultRuntime }

func (claudeProvider) DefaultModel() string { return runtimeinfo.DefaultModel(DefaultRuntime) }

func (claudeProvider) ModelOptions() []ModelOption {
	options := runtimeinfo.ModelOptions(DefaultRuntime)
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

func (claudeProvider) ValidateModel(model string) error {
	return runtimeinfo.ValidateModel(DefaultRuntime, model)
}

func (claudeProvider) PrepareSession(workDir, agentDir string, cfg *SessionConfig, sessionID string) error {
	if err := provisionClaudeInstructions(workDir, cfg, sessionID); err != nil {
		return err
	}
	return setupClaudeHooks(workDir, agentDir, cfg)
}

func (claudeProvider) SkillsDir(workDir string) string {
	return filepath.Join(workDir, ".claude", "skills")
}

func (claudeProvider) PostSessionSync(workDir, agentDir string, patterns []string) ([]string, error) {
	return tocsync.SyncBackWithOptions(workDir, agentDir, patterns, tocsync.Options{
		PathMapper: claudeInstructionPathMapper,
		SkipDirs:   map[string]bool{".claude": true},
	})
}

func (claudeProvider) LaunchInteractive(opts LaunchOptions) error {
	args := []string{"--dangerously-skip-permissions"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume {
		args = append(args, "--resume", opts.SessionID)
	} else {
		args = append(args, "--session-id", opts.SessionID)
	}

	cmd := exec.Command("claude", args...)
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
			return nil
		}
		return fmt.Errorf("failed to launch claude: %w", err)
	}
	return nil
}

func (claudeProvider) LaunchDetached(opts DetachedOptions) error {
	promptPath := filepath.Join(opts.Dir, "toc-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0644); err != nil {
		return err
	}

	scriptPath := filepath.Join(opts.Dir, "toc-run.sh")
	if err := os.WriteFile(scriptPath, []byte(BuildClaudeDetachedScript(opts, promptPath)), 0755); err != nil {
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

// BuildClaudeDetachedScript builds the wrapper script for a detached Claude session.
func BuildClaudeDetachedScript(opts DetachedOptions, promptPath string) string {
	args := "claude --dangerously-skip-permissions"
	if opts.Resume {
		args += " --continue"
	}
	args += fmt.Sprintf(" -p \"$(cat %q)\"", promptPath)
	if opts.Model != "" {
		args += fmt.Sprintf(" --model %s", opts.Model)
	}
	if opts.SessionID != "" {
		args += fmt.Sprintf(" --session-id %s", opts.SessionID)
	}

	tmpOutputPath := opts.OutputPath + ".tmp"
	pidPath := filepath.Join(opts.Dir, "toc-pid.txt")
	exitCodePath := filepath.Join(opts.Dir, "toc-exit-code.txt")

	return fmt.Sprintf(`#!/bin/sh
echo $$ > %q
cd %q
export TOC_WORKSPACE=%q
export TOC_AGENT=%q
export TOC_SESSION_ID=%q
%s < /dev/null > %q 2>&1
TOC_EXIT=$?
echo $TOC_EXIT > %q
mv %q %q
`, pidPath, opts.Dir, opts.Workspace, opts.AgentName, opts.SessionID, args, tmpOutputPath, exitCodePath, tmpOutputPath, opts.OutputPath)
}

func (claudeProvider) ExpectedSessionLogPath(sess *session.Session) string {
	workspacePath := sess.WorkspacePath
	sessionID := sess.ID
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	for _, path := range claudeWorkspaceCandidates(workspacePath) {
		encoded := strings.NewReplacer("/", "-", "_", "-").Replace(path)
		projectDir := filepath.Join(projectsDir, encoded)
		if _, err := os.Stat(projectDir); err == nil {
			return filepath.Join(projectDir, sessionID+".jsonl")
		}
	}

	encoded := strings.NewReplacer("/", "-", "_", "-").Replace(workspacePath)
	return filepath.Join(projectsDir, encoded, sessionID+".jsonl")
}

func (claudeProvider) SessionLogPath(sess *session.Session) string {
	workspacePath := sess.WorkspacePath
	sessionID := sess.ID
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	for _, path := range claudeWorkspaceCandidates(workspacePath) {
		encoded := strings.NewReplacer("/", "-", "_", "-").Replace(path)
		projectDir := filepath.Join(projectsDir, encoded)

		exact := filepath.Join(projectDir, sessionID+".jsonl")
		if _, err := os.Stat(exact); err == nil {
			return exact
		}

		entries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				return filepath.Join(projectDir, e.Name())
			}
		}
	}

	return ""
}

func claudeWorkspaceCandidates(workspacePath string) []string {
	candidates := []string{workspacePath}
	if resolved, err := filepath.EvalSymlinks(workspacePath); err == nil && resolved != workspacePath {
		candidates = append(candidates, resolved)
	}
	if !strings.HasPrefix(workspacePath, "/private") {
		candidates = append(candidates, "/private"+workspacePath)
	}
	return candidates
}

type jsonlEntry struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp,omitempty"`
}

type messageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Content  json.RawMessage `json:"content,omitempty"`
	IsError  bool            `json:"is_error,omitempty"`
}

type toolInput struct {
	FilePath string `json:"file_path,omitempty"`
	Path     string `json:"path,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Command  string `json:"command,omitempty"`
	OldStr   string `json:"old_string,omitempty"`
	NewStr   string `json:"new_string,omitempty"`
	Content  string `json:"content,omitempty"`
	Skill    string `json:"skill,omitempty"`
}

func (claudeProvider) ParseSessionLog(path string) (*ParsedLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open session log: %w", err)
	}
	defer f.Close()

	result := &ParsedLog{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
				if result.FirstTS.IsZero() {
					result.FirstTS = t
				}
				result.LastTS = t
			}
		}

		var steps []Step
		switch entry.Type {
		case "user":
			steps = parseClaudeUserMessage(entry.Message)
		case "assistant":
			steps = parseClaudeAssistantMessage(entry.Message)
		}
		result.Steps = append(result.Steps, steps...)
		for _, step := range steps {
			result.Events = append(result.Events, Event{
				Timestamp: result.LastTS,
				Step:      step,
			})
		}
	}
	return result, nil
}

func (claudeProvider) ParseSessionLogLineEvents(line []byte) []Event {
	var entry jsonlEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil
	}
	var steps []Step
	switch entry.Type {
	case "user":
		steps = parseClaudeUserMessage(entry.Message)
	case "assistant":
		steps = parseClaudeAssistantMessage(entry.Message)
	default:
		return nil
	}
	if len(steps) == 0 {
		return nil
	}

	var ts time.Time
	if entry.Timestamp != "" {
		ts, _ = time.Parse(time.RFC3339Nano, entry.Timestamp)
	}

	events := make([]Event, 0, len(steps))
	for _, step := range steps {
		events = append(events, Event{
			Timestamp: ts,
			Step:      step,
		})
	}
	return events
}

func parseClaudeUserMessage(raw json.RawMessage) []Step {
	var msg messageEnvelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}
	if msg.Role != "user" {
		return nil
	}

	// Content can be a string or array of blocks
	var text string
	if err := json.Unmarshal(msg.Content, &text); err == nil && text != "" {
		return []Step{{Type: "user", Content: text}}
	}

	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return []Step{{Type: "user", Content: b.Text}}
			}
		}
	}

	return nil
}

func parseClaudeAssistantMessage(raw json.RawMessage) []Step {
	var msg messageEnvelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}

	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil && text != "" {
			return []Step{{Type: "text", Content: text}}
		}
		return nil
	}

	var steps []Step
	for _, b := range blocks {
		switch b.Type {
		case "thinking":
			if b.Thinking != "" {
				steps = append(steps, Step{Type: "thinking", Content: b.Thinking})
			}
		case "text":
			if b.Text != "" {
				steps = append(steps, Step{Type: "text", Content: b.Text})
			}
		case "tool_use":
			steps = append(steps, parseClaudeToolUse(b))
		case "tool_result":
			if b.IsError {
				var errText string
				_ = json.Unmarshal(b.Content, &errText)
				steps = append(steps, Step{Type: "error", Content: errText})
			}
		}
	}
	return steps
}

func parseClaudeToolUse(b contentBlock) Step {
	var inp toolInput
	_ = json.Unmarshal(b.Input, &inp)

	step := Step{Type: "tool", Tool: b.Name}
	switch b.Name {
	case "Read":
		step.Path = inp.FilePath
	case "Edit":
		step.Path = inp.FilePath
		if inp.OldStr != "" && inp.NewStr != "" {
			step.Added = strings.Count(inp.NewStr, "\n") + 1
			step.Removed = strings.Count(inp.OldStr, "\n") + 1
		}
	case "Write":
		step.Path = inp.FilePath
		if inp.Content != "" {
			step.Lines = strings.Count(inp.Content, "\n") + 1
		}
	case "Bash":
		step.Command = inp.Command
	case "Glob", "Grep":
		step.Path = inp.Path
		if inp.Pattern != "" {
			step.Content = inp.Pattern
		}
	case "Skill":
		step.Type = "skill"
		step.Skill = inp.Skill
	}

	return step
}

func provisionClaudeInstructions(workDir string, cfg *SessionConfig, sessionID string) error {
	claudeMD := filepath.Join(workDir, "CLAUDE.md")

	content, err := ComposePrompt(workDir, cfg, sessionID)
	if err != nil {
		return err
	}
	if content == "" {
		return nil
	}
	agentMD := filepath.Join(workDir, "agent.md")
	_ = os.Remove(agentMD)

	return os.WriteFile(claudeMD, []byte(content+"\n"), 0644)
}

func setupClaudeHooks(workDir, agentDir string, cfg *SessionConfig) error {
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return err
	}

	var syncScriptPath string
	if len(cfg.Context) > 0 {
		syncScriptPath = filepath.Join(claudeDir, "toc-sync.sh")
		script := buildClaudeSyncHookScript(workDir, agentDir, cfg.Context)
		if err := os.WriteFile(syncScriptPath, []byte(script), 0755); err != nil {
			return err
		}
	}

	var permScriptPath string
	if hasCustomPermissions(cfg.Permissions) {
		perms := cfg.Permissions
		permScriptPath = filepath.Join(claudeDir, "toc-permissions.sh")
		script := buildClaudePermissionHookScript(perms, cfg.Agent)
		if err := os.WriteFile(permScriptPath, []byte(script), 0755); err != nil {
			return err
		}
	}

	if syncScriptPath == "" && cfg.OnEnd == "" && permScriptPath == "" {
		return nil
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings, err := buildClaudeHookSettings(syncScriptPath, cfg.OnEnd, permScriptPath)
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, settings, 0644)
}

func hasCustomPermissions(perms agent.Permissions) bool {
	return len(perms.Integrations) > 0 ||
		len(perms.SubAgents) > 0 ||
		perms.Filesystem.Read != agent.PermOn ||
		perms.Filesystem.Write != agent.PermOn ||
		perms.Filesystem.Execute != agent.PermOn
}
