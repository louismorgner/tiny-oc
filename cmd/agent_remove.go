package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/session"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	agentCmd.AddCommand(agentRemoveCmd)
}

var agentRemoveCmd = &cobra.Command{
	Use:               "remove <agent-name>",
	Short:             "Remove an agent and its configuration",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAgentNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]
		if !agent.Exists(name) {
			return fmt.Errorf("agent '%s' not found", name)
		}

		confirm, err := ui.Prompt(fmt.Sprintf("Remove agent %s? (y/N)", ui.Bold(name)), "n")
		if err != nil {
			return err
		}
		if confirm != "y" && confirm != "Y" {
			ui.Info("Cancelled.")
			return nil
		}

		if err := agent.Remove(name); err != nil {
			return err
		}

		if err := session.RemoveByAgent(name); err != nil {
			return err
		}

		auditLog("agent.remove", map[string]interface{}{"agent": name})

		fmt.Println()
		ui.Success("Removed agent %s and its sessions", ui.Bold(name))
		fmt.Println()
		return nil
	},
}
