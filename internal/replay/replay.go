package replay

import (
	"fmt"
	"time"

	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/usage"
)

// Replay is the parsed timeline for a session.
type Replay struct {
	SessionID       string           `json:"session_id"`
	Agent           string           `json:"agent"`
	Runtime         string           `json:"runtime,omitempty"`
	Model           string           `json:"model,omitempty"`
	Status          string           `json:"status,omitempty"`
	ResumeCount     int              `json:"resume_count,omitempty"`
	RecoveryCount   int              `json:"recovery_count,omitempty"`
	CompactionCount int              `json:"compaction_count,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	LastRecovery    string           `json:"last_recovery,omitempty"`
	DurationSecs    float64          `json:"duration_secs"`
	Tokens          usage.TokenUsage `json:"tokens"`
	Steps           []runtime.Step   `json:"steps"`
	FilesChanged    []string         `json:"files_changed"`
	ToolCount       int              `json:"tool_count"`
	ErrorCount      int              `json:"error_count"`
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

// ForSession parses a runtime session log and returns a structured Replay.
func ForSession(sess *session.Session) (*Replay, error) {
	parsed, err := runtime.EnsureEventLog(sess)
	if err != nil {
		return nil, err
	}

	tokens := usage.ForSession(sess)

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

	var durationSecs float64
	if !parsed.FirstTS.IsZero() && !parsed.LastTS.IsZero() {
		durationSecs = parsed.LastTS.Sub(parsed.FirstTS).Seconds()
	} else if !sess.CreatedAt.IsZero() {
		durationSecs = time.Since(sess.CreatedAt).Seconds()
	}

	var model, lastError, lastRecovery string
	var resumeCount, recoveryCount, compactionCount int
	if state, err := runtime.LoadState(sess); err == nil && state != nil {
		model = state.Model
		resumeCount = state.ResumeCount
		recoveryCount = state.RecoveryCount
		compactionCount = state.CompactionCount
		lastError = state.LastError
		lastRecovery = state.LastRecovery
	}

	return &Replay{
		SessionID:       sess.ID,
		Agent:           sess.Agent,
		Runtime:         sess.RuntimeName(),
		Model:           model,
		Status:          sess.ResolvedStatus(),
		ResumeCount:     resumeCount,
		RecoveryCount:   recoveryCount,
		CompactionCount: compactionCount,
		LastError:       lastError,
		LastRecovery:    lastRecovery,
		DurationSecs:    durationSecs,
		Tokens:          tokens,
		Steps:           parsed.Steps,
		FilesChanged:    filesChanged,
		ToolCount:       toolCount,
		ErrorCount:      errorCount,
	}, nil
}

func collectFilesChanged(steps []runtime.Step) []string {
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
