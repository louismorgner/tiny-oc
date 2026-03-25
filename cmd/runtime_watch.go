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

		// If session is already completed, replay all steps and exit
		status := s.ResolvedStatus()
		if status == "completed" || status == "failed" {
			return watchCompleted(s, jsonFlag, status)
		}

		// Resolve JSONL path — use existing file if available, otherwise
		// construct the expected path so the tailer can poll for it.
		jsonlPath := replay.SessionJSONLPath(s.WorkspacePath, s.ID)
		if jsonlPath == "" {
			jsonlPath = replay.ExpectedJSONLPath(s.WorkspacePath, s.ID)
		}

		// Set up signal handling
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if replay.SessionJSONLPath(s.WorkspacePath, s.ID) == "" {
			ui.Info("Waiting for session %s to start...", shortID(s.ID))
		} else {
			ui.Info("Watching session %s (%s)...", shortID(s.ID), s.Agent)
		}
		fmt.Println()

		events, err := tail.Tail(ctx, tail.Options{
			JSONLPath:     jsonlPath,
			WorkspacePath: s.WorkspacePath,
		})
		if err != nil {
			return err
		}

		for event := range events {
			if event.Finished {
				fmt.Println()
				if event.ExitCode == "0" {
					ui.Success("Session completed")
				} else {
					ui.Error("Session failed (exit code %s)", event.ExitCode)
				}
				return nil
			}

			// Compact mode: skip thinking steps for scannable output
			if !jsonFlag && event.Step.Type == "thinking" {
				continue
			}

			if jsonFlag {
				data, _ := json.Marshal(event.Step)
				fmt.Println(string(data))
			} else {
				printStep(event.Step, true)
			}
		}

		// Context was cancelled (Ctrl+C)
		fmt.Println()
		ui.Info("Stopped watching")
		return nil
	},
}

func watchCompleted(s *session.Session, jsonFlag bool, status string) error {
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
			// Compact mode: skip thinking, show tools and text
			if step.Type == "thinking" {
				continue
			}
			printStep(step, true)
		}
	}

	fmt.Println()
	if status == "completed" {
		ui.Success("Session already completed (%s)", r.FormatDuration())
	} else {
		ui.Error("Session failed (%s)", r.FormatDuration())
	}
	return nil
}
