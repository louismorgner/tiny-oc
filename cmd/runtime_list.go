package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeCmd.AddCommand(runtimeListCmd)
}

var runtimeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agents available to spawn as sub-agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		parentCfg, err := ctx.LoadAgentConfig()
		if err != nil {
			return fmt.Errorf("failed to load agent config: %w", err)
		}

		if !parentCfg.CanSpawnAny() {
			ui.Warn("Agent '%s' has no sub-agent permissions configured.", ctx.Agent)
			ui.Info("Add a %s block to oc-agent.yaml to enable sub-agent spawning.", ui.Bold("permissions.sub-agents"))
			return nil
		}

		// Resolve targets via runtime context so agent.List() sees the workspace
		allAgents, err := ctx.ListAgents()
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}

		var targets []string
		for _, a := range allAgents {
			if a.Name != ctx.Agent && parentCfg.CanSpawn(a.Name) {
				targets = append(targets, a.Name)
			}
		}

		if len(targets) == 0 {
			ui.Warn("No other agents found in the workspace.")
			return nil
		}

		fmt.Println()
		fmt.Printf("  %s\n", ui.Bold("Available sub-agents:"))
		fmt.Println()

		for _, name := range targets {
			targetCfg, err := ctx.LoadTargetAgent(name)
			if err != nil {
				fmt.Printf("  %s %s %s\n", ui.Dim("▪"), ui.Cyan(name), ui.Red("(config error)"))
				continue
			}
			desc := ""
			if targetCfg.Description != "" {
				desc = " " + ui.Dim("— "+targetCfg.Description)
			}
			fmt.Printf("  %s %s %s%s\n", ui.Dim("▪"), ui.Cyan(name), ui.Dim(targetCfg.Model), desc)
		}

		fmt.Println()
		ui.Info("Spawn with: %s", ui.Bold("toc runtime spawn <name> --prompt \"...\""))
		fmt.Println()
		return nil
	},
}
