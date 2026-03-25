package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TokenUsage holds aggregated token counts for a session.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheCreate  int64
}

// Total returns the sum of all token fields.
func (u TokenUsage) Total() int64 {
	return u.InputTokens + u.OutputTokens + u.CacheRead + u.CacheCreate
}

// FormatTotal returns a human-readable token count.
func (u TokenUsage) FormatTotal() string {
	t := u.Total()
	if t == 0 {
		return ""
	}
	if t >= 1_000_000 {
		return fmt.Sprintf("%.1fM tokens", float64(t)/1_000_000)
	}
	if t >= 1_000 {
		return fmt.Sprintf("%.1fk tokens", float64(t)/1_000)
	}
	return fmt.Sprintf("%d tokens", t)
}

// ForSession reads Claude Code's local session data and sums up token usage.
// workspacePath is the session's working directory (e.g. /tmp/toc-sessions/agent-123).
// sessionID is the Claude session UUID.
func ForSession(workspacePath, sessionID string) TokenUsage {
	projectDir := claudeProjectDir(workspacePath)
	if projectDir == "" {
		return TokenUsage{}
	}

	// Try exact session ID match first (sessions launched with --session-id)
	jsonlPath := filepath.Join(projectDir, sessionID+".jsonl")
	u := parseJSONL(jsonlPath)
	if u.Total() > 0 {
		return u
	}

	// Fallback: scan all JSONL files in the project directory.
	// This handles older sessions that were spawned without --session-id,
	// where Claude Code generated its own session UUID for the filename.
	// Each toc session workspace maps to one Claude project dir, so summing
	// all JSONL files gives the correct total for that session.
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return TokenUsage{}
	}
	var total TokenUsage
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		t := parseJSONL(filepath.Join(projectDir, e.Name()))
		total.InputTokens += t.InputTokens
		total.OutputTokens += t.OutputTokens
		total.CacheRead += t.CacheRead
		total.CacheCreate += t.CacheCreate
	}
	return total
}

// claudeProjectDir derives the ~/.claude/projects/<encoded-path>/ directory
// for a given workspace path.
func claudeProjectDir(workspacePath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// On macOS, /tmp is a symlink to /private/tmp
	resolved, err := filepath.EvalSymlinks(workspacePath)
	if err != nil {
		resolved = workspacePath
	}

	// Claude Code encodes paths by replacing "/" with "-" and removing "."
	encoded := strings.ReplaceAll(resolved, "/", "-")
	encoded = strings.ReplaceAll(encoded, ".", "")

	return filepath.Join(home, ".claude", "projects", encoded)
}

type jsonlMessage struct {
	Message *struct {
		Usage *struct {
			InputTokens             int64 `json:"input_tokens"`
			OutputTokens            int64 `json:"output_tokens"`
			CacheReadInputTokens    int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func parseJSONL(path string) TokenUsage {
	f, err := os.Open(path)
	if err != nil {
		return TokenUsage{}
	}
	defer f.Close()

	var usage TokenUsage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		var msg jsonlMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Message != nil && msg.Message.Usage != nil {
			u := msg.Message.Usage
			usage.InputTokens += u.InputTokens
			usage.OutputTokens += u.OutputTokens
			usage.CacheRead += u.CacheReadInputTokens
			usage.CacheCreate += u.CacheCreationInputTokens
		}
	}
	return usage
}
