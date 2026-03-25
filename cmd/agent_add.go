package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentAddCmd)
}

var agentAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an agent from the registry",
	Long:  "Add an agent template from the registry by name. To create from scratch, use: toc agent create",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]

		ui.Info("Fetching registry...")
		index, err := registry.FetchIndex()
		if err != nil {
			return err
		}

		entry, found := registry.FindAgent(index, name)
		if !found {
			// Check if it exists as a different type to give a better error.
			if other, ok := registry.Find(index, name); ok {
				return fmt.Errorf("'%s' is a %s, not an agent — use 'toc skill add %s' instead", name, other.Type, name)
			}
			return fmt.Errorf("'%s' not found in the registry — run 'toc registry search' to see what's available", name)
		}

		ui.Info("Installing agent %s — %s", ui.Bold(entry.Name), ui.Dim(entry.Description))

		if err := registry.Install(entry); err != nil {
			return err
		}

		_ = audit.Log("agent.add", map[string]interface{}{
			"agent":  entry.Name,
			"source": "registry",
		})

		fmt.Println()
		ui.Success("Installed agent %s from registry", ui.Bold(entry.Name))
		if len(entry.Skills) > 0 {
			ui.Info("Skills installed: %s", ui.Dim(fmt.Sprintf("%v", entry.Skills)))
		}
		ui.Info("Spawn with: %s", ui.Bold(fmt.Sprintf("toc agent spawn %s", entry.Name)))
		fmt.Println()
		return nil
	},
}
