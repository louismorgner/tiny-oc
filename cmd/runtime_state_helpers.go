package cmd

import (
	"fmt"
	"os"

	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/usage"
)

type runtimeStateSummary struct {
	Model              string
	Status             string
	ResumeCount        int
	RecoveryCount      int
	CompactionCount    int
	LastError          string
	LastRecovery       string
	CrashInfo          *runtime.CrashInfo
	Todos              []runtime.TodoItem
	Tokens             usage.TokenUsage
	LastRequestContext int64 // input tokens from last API call (context pressure indicator)
}

func loadRuntimeStateSummary(s *session.Session) (*runtimeStateSummary, error) {
	if s == nil {
		return nil, nil
	}

	state, err := runtime.LoadState(s)
	if err != nil {
		if os.IsNotExist(err) {
			cfg, cfgErr := runtime.LoadSessionConfig(s)
			if cfgErr != nil && !os.IsNotExist(cfgErr) {
				return nil, cfgErr
			}
			if cfg == nil && s.RuntimeName() == runtimeinfo.NativeRuntime {
				return nil, nil
			}
			summary := &runtimeStateSummary{
				Tokens: usage.ForSession(s),
			}
			if cfg != nil {
				summary.Model = cfg.Model
			}
			return summary, nil
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
		Todos:           append([]runtime.TodoItem(nil), state.Todos...),
		Tokens: usage.TokenUsage{
			InputTokens:  state.Usage.InputTokens,
			OutputTokens: state.Usage.OutputTokens,
			CacheRead:    state.Usage.CacheRead,
			CacheCreate:  state.Usage.CacheCreate,
		},
		LastRequestContext: state.LastRequestUsage.InputTokens,
	}, nil
}

func formatTodoLines(todos []runtime.TodoItem) []string {
	if len(todos) == 0 {
		return nil
	}
	lines := make([]string, 0, len(todos))
	for _, todo := range todos {
		lines = append(lines, fmt.Sprintf("[%s/%s] %s", todo.Status, todo.Priority, todo.Content))
	}
	return lines
}
