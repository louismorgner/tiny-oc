package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
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

		confirm, err := ui.Confirm(fmt.Sprintf("Remove integration '%s' and delete its credentials?", name), false)
		if err != nil || !confirm {
			return nil
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
