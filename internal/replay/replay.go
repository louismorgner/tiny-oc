package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/usage"
)

// Step represents a single action in the session timeline.
type Step struct {
	Type    string `json:"type"`              // "thinking", "text", "tool", "skill", "error"
	Content string `json:"content,omitempty"` // thinking text, output text, error message
	Tool    string `json:"tool,omitempty"`    // tool name (Read, Edit, Write, Bash, etc.)
	Path    string `json:"path,omitempty"`    // file path for file-related tools
	Lines   int    `json:"lines,omitempty"`   // lines read/written
	Added   int    `json:"added,omitempty"`   // lines added (Edit)
	Removed int    `json:"removed,omitempty"` // lines removed (Edit)
	Command string `json:"command,omitempty"` // bash command
	Success *bool  `json:"success,omitempty"` // tool success/failure
	Skill   string `json:"skill,omitempty"`   // skill name
}

// Replay is the parsed timeline for a session.
type Replay struct {
	SessionID    string            `json:"session_id"`
	Agent        string            `json:"agent"`
	DurationSecs float64           `json:"duration_secs"`
	Tokens       usage.TokenUsage  `json:"tokens"`
	Steps        []Step            `json:"steps"`
	FilesChanged []string          `json:"files_changed"`
	ToolCount    int               `json:"tool_count"`
	ErrorCount   int               `json:"error_count"`
}

// FormatDuration returns a human-readable duration string.
func (r *Replay) FormatDuration() string {
	d := time.Duration(r.DurationSecs * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, s)
}

// ForSession parses a Claude Code session JSONL and returns a structured Replay.
func ForSession(sess *session.Session) (*Replay, error) {
	jsonlPath := SessionJSONLPath(sess.WorkspacePath, sess.ID)
	if jsonlPath == "" {
		return nil, fmt.Errorf("could not resolve JSONL path for session '%s'", sess.ID)
	}

	parsed, err := parseSessionJSONLFull(jsonlPath)
	if err != nil {
		return nil, err
	}

	tokens := usage.ForSession(sess.WorkspacePath, sess.ID)

	filesChanged := collectFilesChanged(parsed.Steps)
	toolCount := 0
	errorCount := 0
	for _, s := range parsed.Steps {
		if s.Type == "tool" || s.Type == "skill" {
			toolCount++
		}
		if s.Type == "error" {
			errorCount++
		}
		if s.Success != nil && !*s.Success {
			errorCount++
		}
	}

	// Compute duration from JSONL timestamps (first to last entry).
	// Falls back to time since creation only for active sessions with no timestamps.
	var durationSecs float64
	if !parsed.FirstTS.IsZero() && !parsed.LastTS.IsZero() {
		durationSecs = parsed.LastTS.Sub(parsed.FirstTS).Seconds()
	} else if !sess.CreatedAt.IsZero() {
		durationSecs = time.Since(sess.CreatedAt).Seconds()
	}

	return &Replay{
		SessionID:    sess.ID,
		Agent:        sess.Agent,
		DurationSecs: durationSecs,
		Tokens:       tokens,
		Steps:        parsed.Steps,
		FilesChanged: filesChanged,
		ToolCount:    toolCount,
		ErrorCount:   errorCount,
	}, nil
}

// ExpectedJSONLPath returns the expected path to a Claude Code JSONL session log
// without checking whether the file exists. Use this when you need to poll for
// a file that may not exist yet (e.g., a session that just started).
func ExpectedJSONLPath(workspacePath, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(home, ".claude", "projects")

	// Try multiple path variants — macOS resolves /var to /private/var via symlinks,
	// and sessions.yaml may store either form while Claude Code uses the resolved form.
	candidates := []string{workspacePath}
	if resolved, err := filepath.EvalSymlinks(workspacePath); err == nil && resolved != workspacePath {
		candidates = append(candidates, resolved)
	}
	if !strings.HasPrefix(workspacePath, "/private") {
		candidates = append(candidates, "/private"+workspacePath)
	}

	// Return the first candidate whose project directory exists (or the primary
	// candidate if none do). The exact JSONL file may not exist yet.
	for _, path := range candidates {
		encoded := strings.NewReplacer("/", "-", "_", "-").Replace(path)
		projectDir := filepath.Join(projectsDir, encoded)
		if _, err := os.Stat(projectDir); err == nil {
			return filepath.Join(projectDir, sessionID+".jsonl")
		}
	}

	// Fallback: use the primary workspace path even if the directory doesn't exist yet.
	encoded := strings.NewReplacer("/", "-", "_", "-").Replace(workspacePath)
	return filepath.Join(projectsDir, encoded, sessionID+".jsonl")
}

// SessionJSONLPath resolves the path to an existing Claude Code JSONL session log.
// Returns "" if the file cannot be found.
func SessionJSONLPath(workspacePath, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(home, ".claude", "projects")

	candidates := []string{workspacePath}
	if resolved, err := filepath.EvalSymlinks(workspacePath); err == nil && resolved != workspacePath {
		candidates = append(candidates, resolved)
	}
	if !strings.HasPrefix(workspacePath, "/private") {
		candidates = append(candidates, "/private"+workspacePath)
	}

	for _, path := range candidates {
		encoded := strings.NewReplacer("/", "-", "_", "-").Replace(path)
		projectDir := filepath.Join(projectsDir, encoded)

		// Try exact match first (interactive sessions where we pass --session-id)
		exact := filepath.Join(projectDir, sessionID+".jsonl")
		if _, err := os.Stat(exact); err == nil {
			return exact
		}

		// For sub-agents (--print mode), Claude Code generates its own session ID.
		// Scan the directory for any JSONL file as fallback.
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

// JSONL message types matching Claude Code's format.
type jsonlEntry struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp,omitempty"`
}

type messageEnvelope struct {
	Role    string            `json:"role"`
	Content json.RawMessage   `json:"content"`
	Usage   *usageBlock       `json:"usage"`
}

type usageBlock struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
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

type parsedJSONL struct {
	Steps    []Step
	FirstTS  time.Time
	LastTS   time.Time
}

func parseSessionJSONL(path string) ([]Step, error) {
	result, err := parseSessionJSONLFull(path)
	if err != nil {
		return nil, err
	}
	return result.Steps, nil
}

func parseSessionJSONLFull(path string) (*parsedJSONL, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open session log: %w", err)
	}
	defer f.Close()

	result := &parsedJSONL{}
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

		switch entry.Type {
		case "user":
			result.Steps = append(result.Steps, parseUserMessage(entry.Message)...)
		case "assistant":
			result.Steps = append(result.Steps, parseAssistantMessage(entry.Message)...)
		}
	}
	return result, nil
}

func parseUserMessage(raw json.RawMessage) []Step {
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

	// Array form: extract text blocks
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

func parseAssistantMessage(raw json.RawMessage) []Step {
	var msg messageEnvelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}

	// Content can be a string or array of blocks
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		// Try as string
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
			steps = append(steps, parseToolUse(b))
		case "tool_result":
			// Tool results are mostly in user messages; if we see one here with is_error, record it
			if b.IsError {
				var errText string
				_ = json.Unmarshal(b.Content, &errText)
				steps = append(steps, Step{Type: "error", Content: errText})
			}
		}
	}
	return steps
}

func parseToolUse(b contentBlock) Step {
	var inp toolInput
	_ = json.Unmarshal(b.Input, &inp)

	step := Step{Type: "tool", Tool: b.Name}

	switch b.Name {
	case "Read":
		step.Path = inp.FilePath
	case "Edit":
		step.Path = inp.FilePath
		if inp.OldStr != "" && inp.NewStr != "" {
			added := strings.Count(inp.NewStr, "\n") + 1
			removed := strings.Count(inp.OldStr, "\n") + 1
			step.Added = added
			step.Removed = removed
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
	default:
		// Generic tool
	}

	return step
}

func collectFilesChanged(steps []Step) []string {
	seen := map[string]bool{}
	var files []string
	for _, s := range steps {
		if s.Path != "" && (s.Tool == "Edit" || s.Tool == "Write") {
			if !seen[s.Path] {
				seen[s.Path] = true
				files = append(files, s.Path)
			}
		}
	}
	return files
}

// ParseJSONLLine parses a single JSONL line and returns any Steps found.
// Returns nil for non-assistant entries or unparseable lines.
func ParseJSONLLine(line []byte) []Step {
	var entry jsonlEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil
	}
	switch entry.Type {
	case "user":
		return parseUserMessage(entry.Message)
	case "assistant":
		return parseAssistantMessage(entry.Message)
	default:
		return nil
	}
}

// TruncateThinking truncates thinking text to maxLen chars for display.
func TruncateThinking(s string, maxLen int) string {
	// Replace newlines with spaces for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
