package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
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

// ForSession reads local session data and sums up token usage for the runtime.
func ForSession(sess *session.Session) TokenUsage {
	if sess == nil {
		return TokenUsage{}
	}

	switch sess.RuntimeName() {
	case "", runtime.DefaultRuntime:
		projectDir := claudeProjectDir(sess.WorkspacePath)
		if projectDir == "" {
			return TokenUsage{}
		}
		return parseJSONL(filepath.Join(projectDir, sess.ID+".jsonl"))
	case runtimeinfo.NativeRuntime:
		state, err := runtime.LoadState(sess)
		if err != nil {
			return TokenUsage{}
		}
		return TokenUsage{
			InputTokens:  state.Usage.InputTokens,
			OutputTokens: state.Usage.OutputTokens,
			CacheRead:    state.Usage.CacheRead,
			CacheCreate:  state.Usage.CacheCreate,
		}
	default:
		return TokenUsage{}
	}
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

	return filepath.Join(home, ".claude", "projects", encoded)
}

type jsonlMessage struct {
	Message *struct {
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
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
