package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a toc workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.Exists() {
			return fmt.Errorf("workspace already initialized in this directory")
		}

		ui.Header("Initialize workspace")

		name, err := ui.Prompt("Workspace name", "")
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("workspace name cannot be empty")
		}

		if err := config.Init(name); err != nil {
			return err
		}

		_ = audit.Log("workspace.init", map[string]interface{}{"name": name})

		fmt.Println()
		ui.Success("Initialized workspace %s", ui.Bold(name))
		fmt.Println()
		ui.Info("Next steps:")
		ui.Info("  %s       Create a new agent from scratch", ui.Bold("toc agent create"))
		ui.Info("  %s     Browse the registry for templates", ui.Bold("toc registry search"))
		ui.Info("  %s  Install an agent or skill template", ui.Bold("toc registry install <name>"))
		fmt.Println()
		return nil
	},
}
