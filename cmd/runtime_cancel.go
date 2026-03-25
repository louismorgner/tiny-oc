package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeCmd.AddCommand(runtimeCancelCmd)
}

var runtimeCancelCmd = &cobra.Command{
	Use:   "cancel <session-id>",
	Short: "Cancel a running sub-agent session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		sessionID := args[0]

		s, err := session.FindByIDInWorkspace(ctx.Workspace, sessionID)
		if err != nil {
			return err
		}

		if s.ParentSessionID != ctx.SessionID {
			return fmt.Errorf("session '%s' is not a sub-agent of this session", sessionID)
		}

		status := s.ResolvedStatus()
		switch status {
		case "active":
			// OK to cancel
		default:
			return fmt.Errorf("cannot cancel session in '%s' state (only active sessions can be cancelled)", status)
		}

		pid, err := s.ReadPID()
		if err != nil {
			return fmt.Errorf("cannot read PID for session '%s': %w (session may predate PID tracking)", sessionID, err)
		}

		// Send SIGTERM to the process group (negative PID kills the group).
		// This ensures both the wrapper script and runtime process are terminated.
		killed := false
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
			// If group kill fails, try killing just the process
			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				if err == syscall.ESRCH {
					// Process already exited — mark as cancelled anyway
				} else {
					return fmt.Errorf("failed to kill process %d: %w", pid, err)
				}
			} else {
				killed = true
			}
		} else {
			killed = true
		}

		// Write cancellation marker so ResolvedStatus returns "cancelled".
		// Only write if the session hasn't already completed (race: process may
		// have finished between our status check and the kill signal).
		outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			markerPath := filepath.Join(s.WorkspacePath, "toc-cancelled.txt")
			_ = os.WriteFile(markerPath, []byte(fmt.Sprintf("cancelled by parent session %s\n", ctx.SessionID)), 0644)
		}
		_ = session.UpdateStatusInWorkspace(ctx.Workspace, s.ID, session.StatusCancelled)
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
			state.LastError = fmt.Sprintf("session cancelled by parent session %s", ctx.SessionID)
			_ = runtime.SaveState(s, state)
		}
		_ = runtime.AppendEvent(s, runtime.Event{
			Timestamp: time.Now().UTC(),
			Step: runtime.Step{
				Type:    "error",
				Content: fmt.Sprintf("session cancelled by parent session %s", ctx.SessionID),
			},
		})

		_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.cancel", map[string]interface{}{
			"parent_session": ctx.SessionID,
			"session_id":     sessionID,
			"agent":          s.Agent,
			"pid":            pid,
			"killed":         killed,
		})

		ui.Success("Cancelled sub-agent %s (session %s)", ui.Bold(s.Agent), ui.Dim(sessionID[:8]))
		return nil
	},
}
