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
	jsonlPath := sessionJSONLPath(sess.WorkspacePath, sess.ID)
	if jsonlPath == "" {
		return nil, fmt.Errorf("could not resolve JSONL path for session '%s'", sess.ID)
	}

	steps, err := parseSessionJSONL(jsonlPath)
	if err != nil {
		return nil, err
	}

	tokens := usage.ForSession(sess.WorkspacePath, sess.ID)

	filesChanged := collectFilesChanged(steps)
	toolCount := 0
	errorCount := 0
	for _, s := range steps {
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

	var durationSecs float64
	if !sess.CreatedAt.IsZero() {
		durationSecs = time.Since(sess.CreatedAt).Seconds()
	}

	return &Replay{
		SessionID:    sess.ID,
		Agent:        sess.Agent,
		DurationSecs: durationSecs,
		Tokens:       tokens,
		Steps:        steps,
		FilesChanged: filesChanged,
		ToolCount:    toolCount,
		ErrorCount:   errorCount,
	}, nil
}

func sessionJSONLPath(workspacePath, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	resolved, err := filepath.EvalSymlinks(workspacePath)
	if err != nil {
		resolved = workspacePath
	}

	encoded := strings.ReplaceAll(resolved, "/", "-")
	return filepath.Join(home, ".claude", "projects", encoded, sessionID+".jsonl")
}

// JSONL message types matching Claude Code's format.
type jsonlEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
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

func parseSessionJSONL(path string) ([]Step, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open session log: %w", err)
	}
	defer f.Close()

	var steps []Step
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "assistant":
			steps = append(steps, parseAssistantMessage(entry.Message)...)
		}
	}
	return steps, nil
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
