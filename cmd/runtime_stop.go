package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeCmd.AddCommand(runtimeStopCmd)
}

var runtimeStopCmd = &cobra.Command{
	Use:   "stop <session-id>",
	Short: "Stop a running sub-agent session",
	Long:  "Stop a sub-agent session by sending SIGTERM, escalating to SIGKILL if needed. Unlike 'cancel', this handles zombie sessions and ensures the process is dead.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		sessionID := args[0]

		s, err := session.FindByIDPrefixInWorkspace(ctx.Workspace, sessionID)
		if err != nil {
			return err
		}

		if s.ParentSessionID != ctx.SessionID {
			return fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID)
		}

		status := s.ResolvedStatus()
		switch status {
		case "active", session.StatusZombie:
			// OK to stop
		case session.StatusCancelled:
			ui.Warn("Session %s is already cancelled", s.ID[:8])
			return nil
		case session.StatusCompletedOK, session.StatusCompletedError, "completed":
			ui.Warn("Session %s has already completed", s.ID[:8])
			return nil
		default:
			return fmt.Errorf("session is in '%s' state", status)
		}

		pid, err := s.ReadPID()
		if err != nil {
			// No PID — just clean up status.
			if err := session.UpdateStatusInWorkspace(ctx.Workspace, s.ID, session.StatusCancelled); err != nil {
				return fmt.Errorf("failed to update session status: %w", err)
			}
			_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.stop", map[string]interface{}{
				"parent_session": ctx.SessionID,
				"session_id":     s.ID,
				"agent":          s.Agent,
				"reason":         "no_pid_file",
			})
			ui.Success("Marked sub-agent %s as stopped (no PID file)", ui.Bold(s.Agent))
			return nil
		}

		result, err := session.TerminateProcess(pid)
		if err != nil {
			return err
		}

		// Persist state updates.
		var persistErrors []error

		outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			markerPath := filepath.Join(s.WorkspacePath, "toc-cancelled.txt")
			if err := os.WriteFile(markerPath, []byte(fmt.Sprintf("stopped by parent session %s\n", ctx.SessionID)), 0644); err != nil {
				persistErrors = append(persistErrors, fmt.Errorf("write cancellation marker: %w", err))
			}
		}
		if err := session.UpdateStatusInWorkspace(ctx.Workspace, s.ID, session.StatusCancelled); err != nil {
			persistErrors = append(persistErrors, fmt.Errorf("update session status: %w", err))
		}

		state, err := runtime.LoadState(s)
		if err != nil && os.IsNotExist(err) {
			state = &runtime.State{
				Runtime:    s.RuntimeName(),
				SessionID:  s.ID,
				Agent:      s.Agent,
				Workspace:  ctx.Workspace,
				SessionDir: s.WorkspacePath,
				Status:     session.StatusCancelled,
			}
			err = nil
		}
		if err == nil && state != nil {
			state.Status = session.StatusCancelled
			state.LastError = fmt.Sprintf("session stopped by parent session %s", ctx.SessionID)
			if err := runtime.SaveState(s, state); err != nil {
				persistErrors = append(persistErrors, fmt.Errorf("save state: %w", err))
			}
		}

		if err := runtime.AppendEvent(s, runtime.Event{
			Timestamp: time.Now().UTC(),
			Step: runtime.Step{
				Type:    "error",
				Content: fmt.Sprintf("session stopped by parent session %s", ctx.SessionID),
			},
		}); err != nil {
			persistErrors = append(persistErrors, fmt.Errorf("append event: %w", err))
		}

		_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.stop", map[string]interface{}{
			"parent_session": ctx.SessionID,
			"session_id":     s.ID,
			"agent":          s.Agent,
			"pid":            pid,
			"already_dead":   result.AlreadyDead,
			"escalated":      result.Escalated,
		})

		switch {
		case result.AlreadyDead:
			ui.Success("Cleaned up sub-agent %s (%s) — process was already dead", ui.Bold(s.Agent), ui.Dim(s.ID[:8]))
		case result.Escalated:
			ui.Success("Force-killed sub-agent %s (%s)", ui.Bold(s.Agent), ui.Dim(s.ID[:8]))
		default:
			ui.Success("Stopped sub-agent %s (%s)", ui.Bold(s.Agent), ui.Dim(s.ID[:8]))
		}

		if len(persistErrors) > 0 {
			return fmt.Errorf("session stopped but state persistence had errors: %w", errors.Join(persistErrors...))
		}
		return nil
	},
}
