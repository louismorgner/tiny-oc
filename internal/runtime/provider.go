package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

const DefaultRuntime = runtimeinfo.DefaultRuntime

type ModelOption struct {
	ID          string
	Label       string
	Description string
}

// Step represents a single action in a runtime session timeline.
type Step struct {
	Type       string `json:"type"`                  // "thinking", "text", "tool", "skill", "error", "crash"
	Content    string `json:"content,omitempty"`     // thinking text, output text, error message
	Tool       string `json:"tool,omitempty"`        // tool name (Read, Edit, Write, Bash, etc.)
	StackTrace string `json:"stack_trace,omitempty"` // crash stack trace
	Path       string `json:"path,omitempty"`        // file path for file-related tools
	Lines      int    `json:"lines,omitempty"`       // lines read/written
	Added      int    `json:"added,omitempty"`       // lines added (Edit)
	Removed    int    `json:"removed,omitempty"`     // lines removed (Edit)
	Command    string `json:"command,omitempty"`     // bash command
	ExitCode   int    `json:"exit_code,omitempty"`   // bash exit code
	DurationMS int64  `json:"duration_ms,omitempty"` // tool duration
	TimedOut   bool   `json:"timed_out,omitempty"`   // bash timeout status
	Success    *bool  `json:"success,omitempty"`     // tool success/failure
	Skill      string `json:"skill,omitempty"`       // skill name
}

// Event is the normalized, toc-owned session event format.
type Event struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Step      Step      `json:"step"`
}

// ParsedLog is the structured representation of a runtime session log.
type ParsedLog struct {
	Events  []Event
	Steps   []Step
	FirstTS time.Time
	LastTS  time.Time
}

type LaunchOptions struct {
	Dir       string
	Model     string
	SessionID string
	AgentName string
	Workspace string
	Resume    bool
	Prompt    string // If set, run this prompt non-interactively and exit
}

type DetachedOptions struct {
	Dir        string
	Model      string
	Prompt     string
	Workspace  string
	AgentName  string
	SessionID  string
	OutputPath string
	Resume     bool
}

// Provider owns runtime-specific process launch and observability behavior.
type Provider interface {
	Name() string
	DefaultModel() string
	ModelOptions() []ModelOption
	ValidateModel(model string) error
	PrepareSession(workDir, agentDir string, cfg *SessionConfig, sessionID string) error
	SkillsDir(workDir string) string
	PostSessionSync(workDir, agentDir string, patterns []string) ([]string, error)
	LaunchInteractive(opts LaunchOptions) error
	LaunchDetached(opts DetachedOptions) error
	SessionLogPath(sess *session.Session) string
	ExpectedSessionLogPath(sess *session.Session) string
	ParseSessionLog(path string) (*ParsedLog, error)
	ParseSessionLogLineEvents(line []byte) []Event
}

func Get(name string) (Provider, error) {
	switch name {
	case "", DefaultRuntime:
		return claudeProvider{}, nil
	case runtimeinfo.NativeRuntime:
		return nativeProvider{}, nil
	default:
		return nil, fmt.Errorf("runtime '%s' is not supported", name)
	}
}

func Supported() []string {
	return runtimeinfo.Supported()
}

// TruncateThinking truncates thinking text to maxLen chars for display.
func TruncateThinking(s string, maxLen int) string {
	// Replace newlines with spaces for single-line display.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
