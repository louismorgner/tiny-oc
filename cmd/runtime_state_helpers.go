package cmd

import (
	"os"

	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/usage"
)

type runtimeStateSummary struct {
	Model           string
	Status          string
	ResumeCount     int
	RecoveryCount   int
	CompactionCount int
	LastError       string
	LastRecovery    string
	CrashInfo       *runtime.CrashInfo
	Tokens          usage.TokenUsage
}

func loadRuntimeStateSummary(s *session.Session) (*runtimeStateSummary, error) {
	if s == nil {
		return nil, nil
	}

	state, err := runtime.LoadState(s)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return &runtimeStateSummary{
		Model:           state.Model,
		Status:          state.Status,
		ResumeCount:     state.ResumeCount,
		RecoveryCount:   state.RecoveryCount,
		CompactionCount: state.CompactionCount,
		LastError:       state.LastError,
		LastRecovery:    state.LastRecovery,
		CrashInfo:       state.CrashInfo,
		Tokens: usage.TokenUsage{
			InputTokens:  state.Usage.InputTokens,
			OutputTokens: state.Usage.OutputTokens,
			CacheRead:    state.Usage.CacheRead,
			CacheCreate:  state.Usage.CacheCreate,
		},
	}, nil
}
