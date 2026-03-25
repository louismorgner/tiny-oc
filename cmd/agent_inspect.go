package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentInspectCmd)
}

var agentInspectCmd = &cobra.Command{
	Use:               "inspect <agent-name>",
	Short:             "Show effective permissions for an agent",
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

		perms := cfg.EffectivePermissions()
		source := "defaults (all on)"
		if cfg.Perms != nil {
			source = "permissions block"
		}

		fmt.Println()
		fmt.Printf("  %s %s\n", ui.BoldCyan(cfg.Name), ui.Dim("— effective permissions"))
		fmt.Printf("  %s %s\n", ui.Dim("Source:"), ui.Dim(source))
		fmt.Println()

		// Filesystem
		fmt.Printf("  %s\n", ui.Bold("Filesystem"))
		printPermRow("read", perms.Filesystem.Read)
		printPermRow("write", perms.Filesystem.Write)
		printPermRow("execute", perms.Filesystem.Execute)
		fmt.Println()

		// Integrations
		if len(perms.Integrations) > 0 {
			fmt.Printf("  %s\n", ui.Bold("Integrations"))
			names := sortedKeys(perms.Integrations)
			for _, n := range names {
				printPermRow(n, perms.Integrations[n])
			}
			fmt.Println()
		}

		// Sub-agents
		if len(perms.SubAgents) > 0 {
			fmt.Printf("  %s\n", ui.Bold("Sub-agents"))
			names := sortedKeys(perms.SubAgents)
			for _, n := range names {
				printPermRow(n, perms.SubAgents[n])
			}
			fmt.Println()
		}

		// Validation warnings
		problems := cfg.Validate()
		if len(problems) > 0 {
			fmt.Printf("  %s %s\n", ui.Yellow("Warnings:"), strings.Join(problems, "; "))
			fmt.Println()
		}

		return nil
	},
}

func printPermRow(name string, level agent.PermissionLevel) {
	var indicator string
	switch level {
	case agent.PermOn:
		indicator = ui.Green("on")
	case agent.PermAsk:
		indicator = ui.Yellow("ask")
	case agent.PermOff:
		indicator = ui.Red("off")
	default:
		indicator = ui.Dim(string(level))
	}
	fmt.Printf("    %-20s %s\n", name, indicator)
}

func sortedKeys(m map[string]agent.PermissionLevel) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
