package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tiny-oc/toc/internal/session"
)

const StateVersion = 5

type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	Name         string        `json:"name,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content,omitempty"`
		Name       string          `json:"name,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	content, err := normalizeOpenRouterContent(raw.Content)
	if err != nil {
		return err
	}

	m.Role = raw.Role
	m.Content = content
	m.Name = raw.Name
	m.ToolCallID = raw.ToolCallID
	m.ToolCalls = raw.ToolCalls
	return nil
}

// State is the persisted runtime session state used for resume.
type State struct {
	Version           int                `json:"version"`
	Runtime           string             `json:"runtime"`
	SessionID         string             `json:"session_id"`
	Agent             string             `json:"agent"`
	Model             string             `json:"model,omitempty"`
	Workspace         string             `json:"workspace,omitempty"`
	SessionDir        string             `json:"session_dir,omitempty"`
	Mode              string             `json:"mode,omitempty"`
	Status            string             `json:"status,omitempty"`
	Prompt            string             `json:"prompt,omitempty"`
	ResumeCount       int                `json:"resume_count,omitempty"`
	RecoveryCount     int                `json:"recovery_count,omitempty"`
	CompactionCount   int                   `json:"compaction_count,omitempty"`
	CompactedMessages int                   `json:"compacted_messages,omitempty"`
	LastError         string                `json:"last_error,omitempty"`
	LastRecovery      string                `json:"last_recovery,omitempty"`
	LastCompactedAt   time.Time             `json:"last_compacted_at,omitempty"`
	LastRecoveredAt   time.Time             `json:"last_recovered_at,omitempty"`
	Usage             TokenUsageSnapshot    `json:"usage,omitempty"`
	LastRequestUsage  LastRequestUsage      `json:"last_request_usage,omitempty"`
	Messages          []Message             `json:"messages,omitempty"`
	Continuation      *ContinuationArtifact `json:"continuation,omitempty"`
	WorkingSet        *WorkingSet           `json:"working_set,omitempty"`
	PendingTurn       *TurnCheckpoint       `json:"pending_turn,omitempty"`
	CrashInfo         *CrashInfo            `json:"crash_info,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type CrashInfo struct {
	PanicMessage string    `json:"panic_message,omitempty"`
	StackTrace   string    `json:"stack_trace,omitempty"`
	LastToolCall string    `json:"last_tool_call,omitempty"`
	CrashTime    time.Time `json:"crash_time,omitempty"`
}

type TurnCheckpoint struct {
	Phase     string     `json:"phase,omitempty"`
	Prompt    string     `json:"prompt,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	StartedAt time.Time  `json:"started_at,omitempty"`
}

type TokenUsageSnapshot struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	CacheRead    int64 `json:"cache_read,omitempty"`
	CacheCreate  int64 `json:"cache_create,omitempty"`
}

// LastRequestUsage holds the token counts from the most recent API call,
// giving visibility into per-request context pressure and cache efficiency.
type LastRequestUsage struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	CacheRead    int64 `json:"cache_read,omitempty"`
	CacheCreate  int64 `json:"cache_create,omitempty"`
}

func MetadataDir(workspace, sessionID string) string {
	return filepath.Join(workspace, ".toc", "sessions", sessionID)
}

func StatePath(sess *session.Session) string {
	if dir := sess.MetadataDirPath(); dir != "" {
		return filepath.Join(dir, "state.json")
	}
	return ""
}

func StatePathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(MetadataDir(workspace, sessionID), "state.json")
}

func LoadState(sess *session.Session) (*State, error) {
	path := StatePath(sess)
	if path == "" {
		return nil, fmt.Errorf("session '%s' has no metadata directory for state storage", sess.ID)
	}
	return loadStateFromPath(path)
}

func LoadStateInWorkspace(workspace, sessionID string) (*State, error) {
	return loadStateFromPath(StatePathInWorkspace(workspace, sessionID))
}

func SaveState(sess *session.Session, state *State) error {
	path := StatePath(sess)
	if path == "" {
		return fmt.Errorf("session '%s' has no metadata directory for state storage", sess.ID)
	}
	return saveStateToPath(path, state)
}

func SaveStateInWorkspace(workspace, sessionID string, state *State) error {
	return saveStateToPath(StatePathInWorkspace(workspace, sessionID), state)
}

func loadStateFromPath(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveStateToPath(path string, state *State) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}

	now := time.Now()
	if state.Version == 0 {
		state.Version = StateVersion
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
