package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/replay"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/tail"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeWatchCmd.Flags().Bool("json", false, "Output each step as newline-delimited JSON")
	runtimeWatchCmd.Flags().BoolP("verbose", "v", false, "Show full assistant messages and thinking blocks")
	runtimeCmd.AddCommand(runtimeWatchCmd)
}

var runtimeWatchCmd = &cobra.Command{
	Use:   "watch <session-id>",
	Short: "Live-tail a sub-agent session",
	Long:  "Stream formatted activity from a running sub-agent session. Shows tool calls, skills, and text output in real time.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]

		var s *session.Session

		// Try workspace-scoped lookup first if inside a toc session
		if ctx, err := runtime.FromEnv(); err == nil {
			s, _ = session.FindByIDInWorkspace(ctx.Workspace, sessionID)
			if s == nil {
				s, _ = session.FindByIDPrefixInWorkspace(ctx.Workspace, sessionID)
			}
		}

		// Fall back to global sessions
		if s == nil {
			var err error
			s, err = session.FindByID(sessionID)
			if err != nil {
				s, err = session.FindByIDPrefix(sessionID)
				if err != nil {
					return fmt.Errorf("session '%s' not found", sessionID)
				}
			}
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		verbose, _ := cmd.Flags().GetBool("verbose")

		// If session is already completed, replay all steps and exit
		status := s.ResolvedStatus()
		if status == "completed" || status == session.StatusCompletedOK || status == session.StatusCompletedError || status == session.StatusCancelled {
			return watchCompleted(s, jsonFlag, verbose, status)
		}

		provider, err := runtime.Get(s.RuntimeName())
		if err != nil {
			return err
		}

		// Resolve the runtime log path — use an existing file if available,
		// otherwise construct the expected path so the tailer can poll for it.
		logPath := provider.SessionLogPath(s)
		if logPath == "" {
			logPath = provider.ExpectedSessionLogPath(s)
		}
		isNativeEventLog := logPath != "" && logPath == runtime.EventLogPath(s)

		// Set up signal handling
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if provider.SessionLogPath(s) == "" {
			ui.Info("Waiting for session %s to start...", shortID(s.ID))
		} else {
			ui.Info("Watching session %s (%s)...", shortID(s.ID), s.Agent)
		}
		fmt.Println()

		events, err := tail.Tail(ctx, tail.Options{
			LogPath:       logPath,
			WorkspacePath: s.WorkspacePath,
			Provider:      provider,
		})
		if err != nil {
			return err
		}

		skippedCached := 0
		cachedCount := runtime.EventCount(s)

		for event := range events {
			if event.Finished {
				_, _ = runtime.EnsureEventLog(s)
				fmt.Println()
				printRuntimeWatchSummary(s)
				if currentStatus := s.ResolvedStatus(); currentStatus == session.StatusCancelled {
					ui.Warn("Session cancelled")
				} else if event.ExitCode == "0" {
					ui.Success("Session completed")
				} else {
					ui.Error("Session failed (exit code %s)", event.ExitCode)
				}
				return nil
			}

			if skippedCached < cachedCount {
				skippedCached++
				continue
			}

			if !isNativeEventLog {
				_ = runtime.AppendEvent(s, event.Event)
			}

			// Skip thinking unless verbose
			if !jsonFlag && !verbose && event.Step.Type == "thinking" {
				continue
			}

			if jsonFlag {
				data, _ := json.Marshal(event.Step)
				fmt.Println(string(data))
			} else {
				printStep(event.Step, printOpts{FullContent: verbose})
			}
		}

		// Context was cancelled (Ctrl+C)
		fmt.Println()
		ui.Info("Stopped watching")
		return nil
	},
}

func watchCompleted(s *session.Session, jsonFlag, verbose bool, status string) error {
	r, err := replay.ForSession(s)
	if err != nil {
		return err
	}

	if jsonFlag {
		for _, step := range r.Steps {
			data, _ := json.Marshal(step)
			fmt.Println(string(data))
		}
	} else {
		for _, step := range r.Steps {
			// Skip thinking unless verbose
			if !verbose && step.Type == "thinking" {
				continue
			}
			printStep(step, printOpts{FullContent: verbose})
		}
	}

	fmt.Println()
	printRuntimeWatchSummary(s)
	if status == "completed" || status == session.StatusCompletedOK {
		ui.Success("Session already completed (%s)", r.FormatDuration())
	} else if status == session.StatusCancelled {
		ui.Warn("Session cancelled (%s)", r.FormatDuration())
	} else {
		ui.Error("Session failed (%s)", r.FormatDuration())
	}
	return nil
}

func printRuntimeWatchSummary(s *session.Session) {
	summary, err := loadRuntimeStateSummary(s)
	if err != nil || summary == nil {
		return
	}

	if summary.Model != "" {
		ui.Info("Model: %s", ui.Dim(summary.Model))
	}
	if summary.ResumeCount > 0 {
		ui.Info("Resumes: %d", summary.ResumeCount)
	}
	if summary.RecoveryCount > 0 {
		ui.Info("Recoveries: %d", summary.RecoveryCount)
	}
	if summary.CompactionCount > 0 {
		ui.Info("Compactions: %d", summary.CompactionCount)
	}
	if total := summary.Tokens.FormatTotal(); total != "" {
		ui.Info("Tokens: %s", ui.Dim(total))
	}
	if summary.LastError != "" && s.ResolvedStatus() != session.StatusCompletedOK && s.ResolvedStatus() != "completed" {
		ui.Info("Last error: %s", ui.Dim(summary.LastError))
	}
	if summary.LastRecovery != "" {
		ui.Info("Last recovery: %s", ui.Dim(summary.LastRecovery))
	}
}
