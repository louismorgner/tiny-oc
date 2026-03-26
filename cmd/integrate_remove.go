package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	integrateRemoveCmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")
	integrateCmd.AddCommand(integrateRemoveCmd)
}

var integrateRemoveCmd = &cobra.Command{
	Use:   "remove <integration>",
	Short: "Remove an integration and its credentials",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]

		if !integration.CredentialExists(name) {
			return fmt.Errorf("integration '%s' is not configured", name)
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			confirm, err := ui.Confirm(fmt.Sprintf("Remove integration '%s' and delete its credentials?", name), false)
			if err != nil || !confirm {
				return nil
			}
		}

		if err := integration.RemoveCredential(name); err != nil {
			return fmt.Errorf("failed to remove credentials: %w", err)
		}

		auditLog("integrate.remove", map[string]interface{}{
			"integration": name,
		})

		ui.Success("Integration '%s' removed", name)
		return nil
	},
}
