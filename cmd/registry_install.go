package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/registry"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	registryCmd.AddCommand(registryInstallCmd)
}

var registryInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: "Install a skill, agent, or workspace from the registry",
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

		entry, found := registry.Find(index, name)
		if !found {
			return fmt.Errorf("'%s' not found in registry — run 'toc registry search' to see what's available", name)
		}

		ui.Info("Installing %s %s — %s", entry.Type, ui.Bold(entry.Name), ui.Dim(entry.Description))

		fmt.Println()
		switch entry.Type {
		case "workspace":
			result, err := registry.InstallWorkspace(entry, index)
			if err != nil {
				return err
			}

			auditLog("registry.install", map[string]interface{}{
				"name":   entry.Name,
				"type":   entry.Type,
				"agents": entry.Agents,
			})

			ui.Success("Installed workspace %s", ui.Bold(entry.Name))
			if len(result.InstalledAgents) > 0 {
				ui.Info("Agents installed: %s", ui.Bold(strings.Join(result.InstalledAgents, ", ")))
			}
			if len(result.SkippedAgents) > 0 {
				ui.Info("Agents skipped (already exist): %s", ui.Dim(strings.Join(result.SkippedAgents, ", ")))
			}
			if len(result.InstalledSkills) > 0 {
				ui.Info("Skills installed: %s", ui.Dim(strings.Join(result.InstalledSkills, ", ")))
			}
			fmt.Println()
			ui.Info("Spawn any agent with: %s", ui.Bold("toc agent spawn <agent-name>"))

		default:
			if err := registry.Install(entry); err != nil {
				return err
			}

			auditLog("registry.install", map[string]interface{}{
				"name": entry.Name,
				"type": entry.Type,
			})

			switch entry.Type {
			case "agent":
				ui.Success("Installed agent %s", ui.Bold(entry.Name))
				if len(entry.Skills) > 0 {
					ui.Info("Skills installed: %s", ui.Dim(strings.Join(entry.Skills, ", ")))
				}
				ui.Info("Spawn with: %s", ui.Bold(fmt.Sprintf("toc agent spawn %s", entry.Name)))
				ui.Info("Tip: you can also use %s", ui.Bold(fmt.Sprintf("toc agent add %s", entry.Name)))
			case "skill":
				ui.Success("Installed skill %s", ui.Bold(entry.Name))
				ui.Info("Assign to an agent with: %s", ui.Bold("toc agent skills <agent-name>"))
				ui.Info("Tip: you can also use %s", ui.Bold(fmt.Sprintf("toc skill add %s", entry.Name)))
			}
		}
		fmt.Println()
		return nil
	},
}
