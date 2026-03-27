package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/integration"
	tocsync "github.com/tiny-oc/toc/internal/sync"
	"github.com/tiny-oc/toc/internal/ui"
)

const maxToolOutputBytes = 64 * 1024

type nativeToolContext struct {
	SessionDir string
	Workspace  string
	Agent      string
	SessionID  string
	State      *State
	Manifest   *integration.PermissionManifest
	Config     *SessionConfig
	SpawnFunc  SubAgentSpawnFunc
	Trace      *traceWriter
}

type toolExecution struct {
	Message string
	Step    Step
}

func nativeRead(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateFilesystemPermission(ctx.Manifest, "read", ctx.Agent); err != nil {
		return toolFailure("Read", "", "", err)
	}
	var args struct {
		FilePath  string `json:"file_path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Read", "", "", err)
	}
	path, err := resolveSessionPath(ctx.SessionDir, args.FilePath)
	if err != nil {
		return toolFailure("Read", args.FilePath, "", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return toolFailure("Read", args.FilePath, "", err)
	}

	text := string(data)
	if args.StartLine > 0 || args.EndLine > 0 {
		text = sliceLines(text, args.StartLine, args.EndLine)
	}
	text = truncateToolOutput("Read", text)
	lines := 0
	if text != "" {
		lines = strings.Count(text, "\n") + 1
	}
	return toolSuccess("Read", args.FilePath, text, Step{
		Type:    "tool",
		Tool:    "Read",
		Path:    args.FilePath,
		Lines:   lines,
		Success: boolPtr(true),
	})
}

func nativeWrite(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateFilesystemPermission(ctx.Manifest, "write", ctx.Agent); err != nil {
		return toolFailure("Write", "", "", err)
	}
	var args struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Write", "", "", err)
	}
	path, err := resolveSessionPath(ctx.SessionDir, args.FilePath)
	if err != nil {
		return toolFailure("Write", args.FilePath, "", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return toolFailure("Write", args.FilePath, "", err)
	}
	if err := writeFilePreserveMode(path, []byte(args.Content), 0644); err != nil {
		return toolFailure("Write", args.FilePath, "", err)
	}

	lines := 0
	if args.Content != "" {
		lines = strings.Count(args.Content, "\n") + 1
	}
	return toolSuccess("Write", args.FilePath, fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), args.FilePath), Step{
		Type:    "tool",
		Tool:    "Write",
		Path:    args.FilePath,
		Lines:   lines,
		Success: boolPtr(true),
	})
}

func nativeEdit(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateFilesystemPermission(ctx.Manifest, "write", ctx.Agent); err != nil {
		return toolFailure("Edit", "", "", err)
	}
	var args struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Edit", "", "", err)
	}
	if args.OldString == "" {
		return toolFailure("Edit", args.FilePath, "", fmt.Errorf("old_string must not be empty"))
	}
	path, err := resolveSessionPath(ctx.SessionDir, args.FilePath)
	if err != nil {
		return toolFailure("Edit", args.FilePath, "", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return toolFailure("Edit", args.FilePath, "", err)
	}
	content := string(data)
	count := strings.Count(content, args.OldString)
	if count == 0 {
		return toolFailure("Edit", args.FilePath, "", fmt.Errorf("old_string not found in %s", args.FilePath))
	}
	if count > 1 && !args.ReplaceAll {
		return toolFailure("Edit", args.FilePath, "", fmt.Errorf("old_string matched %d times in %s; set replace_all to true", count, args.FilePath))
	}

	var updated string
	if args.ReplaceAll {
		updated = strings.ReplaceAll(content, args.OldString, args.NewString)
	} else {
		updated = strings.Replace(content, args.OldString, args.NewString, 1)
	}
	if err := writeFilePreserveMode(path, []byte(updated), 0644); err != nil {
		return toolFailure("Edit", args.FilePath, "", err)
	}

	return toolSuccess("Edit", args.FilePath, fmt.Sprintf("Edited %s", args.FilePath), Step{
		Type:    "tool",
		Tool:    "Edit",
		Path:    args.FilePath,
		Added:   strings.Count(args.NewString, "\n") + 1,
		Removed: strings.Count(args.OldString, "\n") + 1,
		Success: boolPtr(true),
	})
}

func nativeGlob(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateFilesystemPermission(ctx.Manifest, "read", ctx.Agent); err != nil {
		return toolFailure("Glob", "", "", err)
	}
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Glob", "", "", err)
	}
	baseDir := ctx.SessionDir
	if args.Path != "" {
		resolved, err := resolveSessionPath(ctx.SessionDir, args.Path)
		if err != nil {
			return toolFailure("Glob", args.Path, args.Pattern, err)
		}
		baseDir = resolved
	}

	var matches []string
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == baseDir {
			return nil
		}
		if d.IsDir() && shouldSkipToolDir(d.Name()) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if tocsync.MatchesAny(rel, []string{args.Pattern}) {
			matches = append(matches, toolResultPath(args.Path, rel))
		}
		return nil
	})
	if err != nil {
		return toolFailure("Glob", args.Path, args.Pattern, err)
	}
	sort.Strings(matches)
	matches = dedupeStrings(matches)

	return toolSuccess("Glob", args.Path, strings.Join(matches, "\n"), Step{
		Type:    "tool",
		Tool:    "Glob",
		Path:    args.Path,
		Content: args.Pattern,
		Success: boolPtr(true),
	})
}

func nativeGrep(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateFilesystemPermission(ctx.Manifest, "read", ctx.Agent); err != nil {
		return toolFailure("Grep", "", "", err)
	}
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Grep", "", "", err)
	}
	searchRoot := "."
	if args.Path != "" {
		resolved, err := resolveSessionPath(ctx.SessionDir, args.Path)
		if err != nil {
			return toolFailure("Grep", args.Path, args.Pattern, err)
		}
		rel, err := filepath.Rel(ctx.SessionDir, resolved)
		if err != nil {
			return toolFailure("Grep", args.Path, args.Pattern, err)
		}
		searchRoot = rel
	}

	cmd := exec.Command("rg", "-n", "--color", "never", "--hidden", "--glob", "!.git", "--glob", "!.git/**", "--glob", "!.toc-native", "--glob", "!.toc-native/**", args.Pattern, searchRoot)
	cmd.Dir = ctx.SessionDir
	output, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		err = nil
	}
	if err != nil && len(output) == 0 {
		return toolFailure("Grep", args.Path, args.Pattern, err)
	}

	return toolSuccess("Grep", args.Path, truncateToolOutput("Grep", string(output)), Step{
		Type:    "tool",
		Tool:    "Grep",
		Path:    args.Path,
		Content: args.Pattern,
		Success: boolPtr(err == nil),
	})
}

func nativeBash(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateFilesystemPermission(ctx.Manifest, "execute", ctx.Agent); err != nil {
		return toolFailure("Bash", "", "", err)
	}
	var args struct {
		Command   string `json:"command"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Bash", "", "", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return toolFailure("Bash", "", "", fmt.Errorf("command is required"))
	}
	timeout := 30 * time.Second
	if args.TimeoutMS > 0 {
		timeout = time.Duration(args.TimeoutMS) * time.Millisecond
	}
	ctxExec, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	// sh -lc loads the user's login shell profile so tools like nvm, rbenv,
	// and custom PATH entries are available. This can cause surprising failures
	// if the user's shell profile (.zshrc, .bashrc, etc.) contains errors.
	cmd := exec.CommandContext(ctxExec, "sh", "-lc", args.Command)
	cmd.Dir = ctx.SessionDir
	output, err := cmd.CombinedOutput()
	durationMS := time.Since(start).Milliseconds()
	step := Step{
		Type:       "tool",
		Tool:       "Bash",
		Command:    args.Command,
		DurationMS: durationMS,
	}
	if ctxExec.Err() == context.DeadlineExceeded {
		step.TimedOut = true
		step.Success = boolPtr(false)
		return toolExecution{
			Message: joinToolMessage(string(output), fmt.Sprintf("command timed out after %s", timeout)),
			Step:    step,
		}
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		step.ExitCode = exitErr.ExitCode()
		step.Success = boolPtr(false)
		return toolExecution{
			Message: joinToolMessage(string(output), fmt.Sprintf("command exited with code %d", exitErr.ExitCode())),
			Step:    step,
		}
	}
	if err != nil {
		step.Success = boolPtr(false)
		return toolExecution{
			Message: joinToolMessage(string(output), err.Error()),
			Step:    step,
		}
	}
	step.ExitCode = 0
	step.Success = boolPtr(true)
	return toolExecution{
		Message: truncateToolOutput("Bash", string(output)),
		Step:    step,
	}
}

func nativeSkill(ctx nativeToolContext, call ToolCall) toolExecution {
	var args struct {
		Skill string `json:"skill"`
		Name  string `json:"name"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Skill", "", "", err)
	}
	name := args.Skill
	if name == "" {
		name = args.Name
	}
	if name == "" {
		return toolExecution{
			Message: "missing skill name",
			Step: Step{
				Type:    "skill",
				Skill:   "",
				Success: boolPtr(false),
			},
		}
	}

	path := filepath.Join(ctx.SessionDir, ".toc-native", "skills", name, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return toolExecution{
			Message: err.Error(),
			Step: Step{
				Type:    "skill",
				Skill:   name,
				Success: boolPtr(false),
			},
		}
	}
	return toolSuccess("Skill", "", truncateToolOutput("Skill", string(data)), Step{
		Type:    "skill",
		Skill:   name,
		Success: boolPtr(true),
	})
}

func nativeTodoWrite(ctx nativeToolContext, call ToolCall) toolExecution {
	if ctx.State == nil {
		return toolFailure("TodoWrite", "", "", fmt.Errorf("todo state is not available"))
	}

	var args struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("TodoWrite", "", "", err)
	}

	normalized := make([]TodoItem, 0, len(args.Todos))
	for i, todo := range args.Todos {
		content := strings.TrimSpace(todo.Content)
		if content == "" {
			return toolFailure("TodoWrite", "", "", fmt.Errorf("todo %d content must not be empty", i+1))
		}
		status := strings.TrimSpace(todo.Status)
		if !isValidTodoStatus(status) {
			return toolFailure("TodoWrite", "", "", fmt.Errorf("todo %d has invalid status %q", i+1, todo.Status))
		}
		priority := strings.TrimSpace(todo.Priority)
		if !isValidTodoPriority(priority) {
			return toolFailure("TodoWrite", "", "", fmt.Errorf("todo %d has invalid priority %q", i+1, todo.Priority))
		}
		normalized = append(normalized, TodoItem{
			Content:  content,
			Status:   status,
			Priority: priority,
		})
	}

	if len(normalized) == 0 {
		ctx.State.Todos = nil
	} else {
		ctx.State.Todos = normalized
	}

	message := summarizeTodos(normalized)
	return toolSuccess("TodoWrite", "", message, Step{
		Type:    "tool",
		Tool:    "TodoWrite",
		Content: message,
		Success: boolPtr(true),
	})
}

// questionPollTimeout is the maximum time nativeQuestion will wait for an answer
// in non-interactive sessions. Overridable in tests.
var questionPollTimeout = 5 * time.Minute

func nativeQuestion(ctx nativeToolContext, call ToolCall) toolExecution {
	var args struct {
		Question string `json:"question"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("Question", "", "", err)
	}
	if strings.TrimSpace(args.Question) == "" {
		return toolFailure("Question", "", "", fmt.Errorf("question is required"))
	}

	if ui.IsTTY(os.Stdin) {
		fmt.Fprintf(os.Stdout, "\n%s\n> ", args.Question)
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return toolFailure("Question", "", "", fmt.Errorf("failed to read answer: %w", err))
		}
		answer = strings.TrimRight(answer, "\r\n")
		return toolSuccess("Question", "", answer, Step{
			Type:    "tool",
			Tool:    "Question",
			Content: args.Question,
			Success: boolPtr(true),
		})
	}

	// Non-interactive session: write the question to a well-known file and poll
	// for an answer file so a parent session or operator can respond.
	if ctx.Workspace == "" || ctx.SessionID == "" {
		return toolFailure("Question", "", "", fmt.Errorf("clarification not available: session context is missing"))
	}

	metaDir := MetadataDir(ctx.Workspace, ctx.SessionID)
	questionPath := filepath.Join(metaDir, "question.json")
	answerPath := filepath.Join(metaDir, "answer.json")

	type questionPayload struct {
		Question  string    `json:"question"`
		Timestamp time.Time `json:"timestamp"`
		SessionID string    `json:"session_id"`
		Agent     string    `json:"agent"`
	}
	payload, err := json.Marshal(questionPayload{
		Question:  args.Question,
		Timestamp: time.Now().UTC(),
		SessionID: ctx.SessionID,
		Agent:     ctx.Agent,
	})
	if err != nil {
		return toolFailure("Question", "", "", fmt.Errorf("failed to encode question: %w", err))
	}
	if err := os.MkdirAll(metaDir, 0700); err != nil {
		return toolFailure("Question", "", "", fmt.Errorf("failed to create session metadata directory: %w", err))
	}
	// Remove a stale answer before writing the question so we don't pick up an
	// old response.
	_ = os.Remove(answerPath)
	if err := os.WriteFile(questionPath, payload, 0600); err != nil {
		return toolFailure("Question", "", "", fmt.Errorf("failed to write question: %w", err))
	}

	deadline := time.Now().Add(questionPollTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		data, err := os.ReadFile(answerPath)
		if err != nil {
			continue
		}
		var ans struct {
			Answer string `json:"answer"`
		}
		if err := json.Unmarshal(data, &ans); err != nil {
			continue
		}
		_ = os.Remove(questionPath)
		return toolSuccess("Question", "", ans.Answer, Step{
			Type:    "tool",
			Tool:    "Question",
			Content: args.Question,
			Success: boolPtr(true),
		})
	}

	_ = os.Remove(questionPath)
	return toolSuccess("Question", "", "No answer was provided (timed out after waiting for a response).", Step{
		Type:    "tool",
		Tool:    "Question",
		Content: args.Question,
		Success: boolPtr(true),
	})
}

func isValidTodoStatus(status string) bool {
	switch status {
	case "pending", "in_progress", "completed", "cancelled":
		return true
	default:
		return false
	}
}

func isValidTodoPriority(priority string) bool {
	switch priority {
	case "high", "medium", "low":
		return true
	default:
		return false
	}
}

func summarizeTodos(todos []TodoItem) string {
	if len(todos) == 0 {
		return "Cleared todo list."
	}

	counts := map[string]int{
		"pending":     0,
		"in_progress": 0,
		"completed":   0,
		"cancelled":   0,
	}
	for _, todo := range todos {
		counts[todo.Status]++
	}

	parts := []string{}
	if counts["in_progress"] > 0 {
		parts = append(parts, fmt.Sprintf("%d in progress", counts["in_progress"]))
	}
	if counts["pending"] > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", counts["pending"]))
	}
	if counts["completed"] > 0 {
		parts = append(parts, fmt.Sprintf("%d completed", counts["completed"]))
	}
	if counts["cancelled"] > 0 {
		parts = append(parts, fmt.Sprintf("%d cancelled", counts["cancelled"]))
	}

	return fmt.Sprintf("Updated %d todos: %s.", len(todos), strings.Join(parts, ", "))
}

func toolSuccess(toolName, path, message string, step Step) toolExecution {
	if step.Type == "" {
		step.Type = "tool"
	}
	return toolExecution{Message: message, Step: step}
}

func toolSuccessOrFailure(toolName, path, message string, step Step, err error) toolExecution {
	if err != nil {
		if step.Success == nil {
			step.Success = boolPtr(false)
		}
		if message == "" {
			message = err.Error()
		} else {
			message = strings.TrimSpace(message) + "\n" + err.Error()
		}
		return toolExecution{Message: message, Step: step}
	}
	if step.Success == nil {
		step.Success = boolPtr(true)
	}
	return toolExecution{Message: message, Step: step}
}

func toolFailure(toolName, path, content string, err error) toolExecution {
	step := Step{
		Type:    "tool",
		Tool:    toolName,
		Path:    path,
		Content: content,
		Success: boolPtr(false),
	}
	if toolName == "Skill" {
		step.Type = "skill"
		step.Skill = content
	}
	return toolExecution{
		Message: err.Error(),
		Step:    step,
	}
}

func decodeToolArgs(raw string, dst interface{}) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("tool arguments are empty")
	}
	return json.Unmarshal([]byte(raw), dst)
}

func resolveSessionPath(sessionDir, input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("path is required")
	}
	target := input
	if !filepath.IsAbs(target) {
		target = filepath.Join(sessionDir, target)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	absSessionDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absSessionDir, absTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the session workspace", input)
	}
	return absTarget, nil
}

func toolResultPath(basePath, rel string) string {
	if basePath == "" || rel == "" || rel == "." {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Join(basePath, rel))
}

func shouldSkipToolDir(name string) bool {
	return name == ".git" || name == ".toc-native"
}

func writeFilePreserveMode(path string, data []byte, defaultMode fs.FileMode) error {
	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func joinToolMessage(output, suffix string) string {
	output = truncateToolOutput("Bash", output)
	suffix = strings.TrimSpace(suffix)
	if output == "" {
		return suffix
	}
	if suffix == "" {
		return output
	}
	return strings.TrimSpace(output) + "\n" + suffix
}

func sliceLines(text string, startLine, endLine int) string {
	lines := strings.Split(text, "\n")
	start := 1
	if startLine > 0 {
		start = startLine
	}
	end := len(lines)
	if endLine > 0 && endLine < end {
		end = endLine
	}
	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		return ""
	}
	if end < start {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

func truncateString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	var buf bytes.Buffer
	buf.WriteString(s[:limit])
	buf.WriteString("\n...[truncated]")
	return buf.String()
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func boolPtr(v bool) *bool {
	return &v
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}
