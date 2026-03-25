package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/spawn"
	"github.com/tiny-oc/toc/internal/ui"
)

var (
	runtimeSpawnPrompt string
	runtimeSpawnResume string
)

func init() {
	runtimeSpawnCmd.Flags().StringVarP(&runtimeSpawnPrompt, "prompt", "p", "", "Task prompt for the sub-agent")
	runtimeSpawnCmd.Flags().StringVar(&runtimeSpawnResume, "resume", "", "Resume a failed or cancelled sub-agent session by ID")
	runtimeCmd.AddCommand(runtimeSpawnCmd)
}

var runtimeSpawnCmd = &cobra.Command{
	Use:   "spawn <agent-name>",
	Short: "Spawn a sub-agent session in the background",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		targetName := args[0]

		// Load parent agent and check permissions
		parentCfg, err := ctx.LoadAgentConfig()
		if err != nil {
			return fmt.Errorf("failed to load parent agent config: %w", err)
		}

		if !parentCfg.CanSpawn(targetName) {
			return fmt.Errorf("agent '%s' is not allowed to spawn '%s' — check sub-agents config in oc-agent.yaml", ctx.Agent, targetName)
		}

		// Resume path
		if runtimeSpawnResume != "" {
			return resumeSubAgent(ctx, targetName)
		}

		// Normal spawn path — prompt is required
		if runtimeSpawnPrompt == "" {
			return fmt.Errorf("--prompt is required when spawning a new sub-agent (use --resume to resume a failed session)")
		}

		// Load target agent
		targetCfg, err := ctx.LoadTargetAgent(targetName)
		if err != nil {
			return fmt.Errorf("agent '%s' not found in workspace", targetName)
		}

		ui.Info("Spawning sub-agent %s in background...", ui.Bold(targetName))

		result, err := spawn.SpawnSubSession(targetCfg, spawn.SubSpawnOpts{
			ParentSessionID: ctx.SessionID,
			Prompt:          runtimeSpawnPrompt,
			WorkspaceDir:    ctx.Workspace,
		})
		if err != nil {
			return err
		}

		_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.spawn", map[string]interface{}{
			"parent_agent":   ctx.Agent,
			"parent_session": ctx.SessionID,
			"target_agent":   targetName,
			"session_id":     result.SessionID,
			"prompt":         runtimeSpawnPrompt,
		})

		fmt.Println()
		ui.Success("Sub-agent %s spawned", ui.Bold(targetName))
		ui.Info("Session ID: %s", ui.Cyan(result.SessionID))
		ui.Info("Check status: %s", ui.Bold(fmt.Sprintf("toc runtime status %s", result.SessionID)))
		ui.Info("Read output:  %s", ui.Bold(fmt.Sprintf("toc runtime output %s", result.SessionID)))
		fmt.Println()
		return nil
	},
}

func resumeSubAgent(ctx *runtime.Context, targetName string) error {
	s, err := session.FindByIDInWorkspace(ctx.Workspace, runtimeSpawnResume)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if s.Agent != targetName {
		return fmt.Errorf("session '%s' belongs to agent '%s', not '%s'", runtimeSpawnResume, s.Agent, targetName)
	}

	if s.ParentSessionID != ctx.SessionID {
		return fmt.Errorf("session '%s' is not a sub-agent of this session", runtimeSpawnResume)
	}

	// Only allow resuming failed, zombie, or cancelled sessions
	status := s.ResolvedStatus()
	switch status {
	case session.StatusCompletedError, session.StatusZombie, session.StatusCancelled:
		// OK to resume
	default:
		return fmt.Errorf("cannot resume session in '%s' state (only failed, zombie, or cancelled sessions can be resumed)", status)
	}

	ui.Info("Resuming sub-agent %s...", ui.Bold(targetName))

	result, err := spawn.ResumeSubSession(s, spawn.SubSpawnOpts{
		ParentSessionID: ctx.SessionID,
		Prompt:          runtimeSpawnPrompt,
		WorkspaceDir:    ctx.Workspace,
	})
	if err != nil {
		return err
	}

	_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.resume", map[string]interface{}{
		"parent_agent":   ctx.Agent,
		"parent_session": ctx.SessionID,
		"target_agent":   targetName,
		"session_id":     result.SessionID,
		"prompt":         runtimeSpawnPrompt,
	})

	fmt.Println()
	ui.Success("Sub-agent %s resumed", ui.Bold(targetName))
	ui.Info("Session ID: %s", ui.Cyan(result.SessionID))
	ui.Info("Check status: %s", ui.Bold(fmt.Sprintf("toc runtime status %s", result.SessionID)))
	ui.Info("Read output:  %s", ui.Bold(fmt.Sprintf("toc runtime output %s", result.SessionID)))
	fmt.Println()
	return nil
}
