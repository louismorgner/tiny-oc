package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentListCmd)
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		agents, err := agent.List()
		if err != nil {
			return err
		}

		if len(agents) == 0 {
			ui.Warn("No agents configured. Run %s to create one.", ui.Bold("toc agent create"))
			return nil
		}

		fmt.Println()
		fmt.Printf("  %-20s %-10s %s\n", ui.Bold("NAME"), ui.Bold("MODEL"), ui.Bold("DESCRIPTION"))
		fmt.Printf("  %-20s %-10s %s\n", ui.Dim("────────────────────"), ui.Dim("──────────"), ui.Dim("───────────────────────────────"))
		for _, a := range agents {
			fmt.Printf("  %-20s %-10s %s\n", ui.Cyan(a.Name), a.Model, ui.Dim(a.Description))
		}
		fmt.Println()
		return nil
	},
}
