package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	integrateCmd.AddCommand(integrateListCmd)
}

var integrateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured integrations",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		names, err := integration.ListConfiguredIntegrations()
		if err != nil {
			return err
		}

		if len(names) == 0 {
			ui.Info("No integrations configured.")
			ui.Info("Add one with: %s", ui.Bold("toc integrate add <name>"))
			return nil
		}

		fmt.Println()
		ui.Header("Configured integrations")
		fmt.Println()
		for _, name := range names {
			def, err := integration.LoadFromRegistry(name)
			if err != nil {
				fmt.Printf("  %s %s %s\n", ui.Green("✓"), ui.Bold(name), ui.Dim("(definition not found)"))
				continue
			}
			fmt.Printf("  %s %s — %s\n", ui.Green("✓"), ui.Bold(name), ui.Dim(def.Description))
		}
		fmt.Println()
		return nil
	},
}
