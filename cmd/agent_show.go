package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentShowCmd)
}

var agentShowCmd = &cobra.Command{
	Use:               "show <agent-name>",
	Short:             "Show agent configuration",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAgentNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]
		cfg, err := agent.Load(name)
		if err != nil {
			return err
		}

		problems := cfg.Validate()

		fmt.Println()
		if len(problems) > 0 {
			fmt.Printf("  %s %s %s\n", ui.Red("✗"), ui.BoldCyan(cfg.Name), ui.Red("("+strings.Join(problems, ", ")+")"))
		} else {
			fmt.Printf("  %s %s\n", ui.Green("✓"), ui.BoldCyan(cfg.Name))
		}
		fmt.Println()

		fmt.Printf("  %-16s %s\n", ui.Bold("Runtime:"), cfg.Runtime)
		fmt.Printf("  %-16s %s\n", ui.Bold("Model:"), cfg.Model)
		if cfg.Description != "" {
			fmt.Printf("  %-16s %s\n", ui.Bold("Description:"), cfg.Description)
		}
		fmt.Println()

		if len(cfg.Skills) > 0 {
			fmt.Printf("  %s\n", ui.Bold("Skills:"))
			for _, s := range cfg.Skills {
				fmt.Printf("    %s %s\n", ui.Dim("▪"), s)
			}
			fmt.Println()
		}

		if len(cfg.Context) > 0 {
			fmt.Printf("  %s\n", ui.Bold("Context sync:"))
			for _, c := range cfg.Context {
				fmt.Printf("    %s %s\n", ui.Dim("▪"), c)
			}
			fmt.Println()
		}

		if len(cfg.Compose) > 0 {
			fmt.Printf("  %s\n", ui.Bold("Compose:"))
			for _, c := range cfg.Compose {
				fmt.Printf("    %s %s\n", ui.Dim("▪"), c)
			}
			fmt.Println()
		}

		if len(cfg.SubAgents) > 0 {
			fmt.Printf("  %s %s\n", ui.Bold("Sub-agents:"), strings.Join(cfg.SubAgents, ", "))
			fmt.Println()
		}

		if cfg.OnEnd != "" {
			fmt.Printf("  %s\n", ui.Bold("On-end hook:"))
			fmt.Printf("    %s\n", ui.Dim(cfg.OnEnd))
			fmt.Println()
		}

		// Show agent.md preview
		mdPath := agent.Dir(name) + "/agent.md"
		if data, err := os.ReadFile(mdPath); err == nil {
			content := strings.TrimSpace(string(data))
			if content != "" {
				fmt.Printf("  %s %s\n", ui.Bold("Instructions:"), ui.Dim(mdPath))
				lines := strings.Split(content, "\n")
				limit := 10
				if len(lines) < limit {
					limit = len(lines)
				}
				for _, line := range lines[:limit] {
					fmt.Printf("    %s\n", ui.Dim(line))
				}
				if len(lines) > 10 {
					fmt.Printf("    %s\n", ui.Dim(fmt.Sprintf("... (%d more lines)", len(lines)-10)))
				}
				fmt.Println()
			}
		}

		fmt.Printf("  %s %s\n", ui.Bold("Config file:"), ui.Dim(agent.Dir(name)+"/oc-agent.yaml"))
		fmt.Println()
		return nil
	},
}
