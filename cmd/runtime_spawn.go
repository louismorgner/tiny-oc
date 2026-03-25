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

var runtimeSpawnPrompt string
var runtimeSpawnResume string

func init() {
	runtimeSpawnCmd.Flags().StringVarP(&runtimeSpawnPrompt, "prompt", "p", "", "Task prompt for the sub-agent")
	runtimeSpawnCmd.Flags().StringVar(&runtimeSpawnResume, "resume", "", "Resume a failed or cancelled sub-agent session by ID")
	runtimeCmd.AddCommand(runtimeSpawnCmd)
}

var runtimeSpawnCmd = &cobra.Command{
	Use:   "spawn <agent-name>",
	Short: "Spawn a sub-agent session in the background",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// --prompt is required unless --resume is set
		if runtimeSpawnResume == "" && runtimeSpawnPrompt == "" {
			return fmt.Errorf("required flag \"prompt\" not set (use --prompt or --resume)")
		}
		return nil
	},
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

		// Resume flow
		if runtimeSpawnResume != "" {
			return resumeSubAgent(ctx, targetName, runtimeSpawnResume, runtimeSpawnPrompt)
		}

		// Normal spawn flow
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

func resumeSubAgent(ctx *runtime.Context, targetName, resumeID, prompt string) error {
	s, err := session.FindByIDPrefixInWorkspace(ctx.Workspace, resumeID)
	if err != nil {
		return fmt.Errorf("failed to find session to resume: %w", err)
	}

	if s.Agent != targetName {
		return fmt.Errorf("session '%s' belongs to agent '%s', not '%s'", s.ID, s.Agent, targetName)
	}

	if s.ParentSessionID != ctx.SessionID {
		return fmt.Errorf("session '%s' is not a sub-agent of this session", s.ID)
	}

	ui.Info("Resuming sub-agent %s (session %s)...", ui.Bold(targetName), ui.Cyan(s.ID[:8]))

	result, err := spawn.ResumeSubSession(s, spawn.SubResumeOpts{
		ParentSessionID: ctx.SessionID,
		Prompt:          prompt,
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
		"prompt":         prompt,
	})

	fmt.Println()
	ui.Success("Sub-agent %s resumed", ui.Bold(targetName))
	ui.Info("Session ID: %s", ui.Cyan(result.SessionID))
	ui.Info("Check status: %s", ui.Bold(fmt.Sprintf("toc runtime status %s", result.SessionID)))
	ui.Info("Read output:  %s", ui.Bold(fmt.Sprintf("toc runtime output %s", result.SessionID)))
	fmt.Println()
	return nil
}
