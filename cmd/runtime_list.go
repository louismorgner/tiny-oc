package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	runtimeListCmd.Flags().Bool("json", false, "Output structured JSON")
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
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				fmt.Println("[]")
				return nil
			}
			ui.Warn("No other agents found in the workspace.")
			return nil
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			type agentInfo struct {
				Name        string `json:"name"`
				Model       string `json:"model,omitempty"`
				Description string `json:"description,omitempty"`
			}
			var result []agentInfo
			for _, name := range targets {
				info := agentInfo{Name: name}
				if targetCfg, err := ctx.LoadTargetAgent(name); err == nil {
					info.Model = targetCfg.Model
					info.Description = targetCfg.Description
				}
				result = append(result, info)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
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
