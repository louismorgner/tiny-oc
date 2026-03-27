package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	rootCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:   "stop <session-id>",
	Short: "Stop a running agent session",
	Long:  "Stop an active agent session by sending SIGTERM, escalating to SIGKILL if needed. Also cleans up zombie sessions where the process has already died.",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeActiveSessionIDs(cmd, args, toComplete)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		sessionID := args[0]

		s, err := session.FindByIDPrefix(sessionID)
		if err != nil {
			return err
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
		case "stale":
			ui.Warn("Session %s is stale (workspace path missing)", s.ID[:8])
			return nil
		default:
			return fmt.Errorf("session is in '%s' state", status)
		}

		pid, err := s.ReadPID()
		if err != nil {
			// No PID file — can't send signals, just clean up status.
			if err := session.UpdateStatus(s.ID, session.StatusCancelled); err != nil {
				return fmt.Errorf("failed to update session status: %w", err)
			}
			writeCancellationMarker(s, "stopped by user (no PID file)")
			auditLog("session.stop", map[string]interface{}{
				"session_id": s.ID,
				"agent":      s.Agent,
				"reason":     "no_pid_file",
			})
			ui.Success("Marked session %s as stopped (no PID file found — process may predate PID tracking)", s.ID[:8])
			return nil
		}

		result, err := session.TerminateProcess(pid)
		if err != nil {
			return err
		}

		// Update status and write marker.
		if err := session.UpdateStatus(s.ID, session.StatusCancelled); err != nil {
			ui.Warn("Failed to update session status: %s", err)
		}
		writeCancellationMarker(s, "stopped by user")

		auditLog("session.stop", map[string]interface{}{
			"session_id":   s.ID,
			"agent":        s.Agent,
			"pid":          pid,
			"already_dead": result.AlreadyDead,
			"escalated":    result.Escalated,
		})

		switch {
		case result.AlreadyDead:
			ui.Success("Cleaned up session %s (%s) — process was already dead", ui.Bold(s.Agent), ui.Dim(s.ID[:8]))
		case result.Escalated:
			ui.Success("Force-killed session %s (%s) — SIGTERM was not enough, sent SIGKILL", ui.Bold(s.Agent), ui.Dim(s.ID[:8]))
		default:
			ui.Success("Stopped session %s (%s)", ui.Bold(s.Agent), ui.Dim(s.ID[:8]))
		}

		return nil
	},
}

func writeCancellationMarker(s *session.Session, reason string) {
	outputPath := filepath.Join(s.WorkspacePath, "toc-output.txt")
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		markerPath := filepath.Join(s.WorkspacePath, "toc-cancelled.txt")
		_ = os.WriteFile(markerPath, []byte(reason+"\n"), 0644)
	}
}

func completeActiveSessionIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 || !config.Exists() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	sf, err := session.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var completions []string
	for _, s := range sf.Sessions {
		status := s.ResolvedStatus()
		if status == "active" || status == session.StatusZombie {
			completions = append(completions, fmt.Sprintf("%s\t%s (%s)", s.ID, s.Agent, status))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
