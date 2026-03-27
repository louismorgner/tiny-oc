package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	integrateCmd.AddCommand(integrateActivateCmd)
}

var integrateActivateCmd = &cobra.Command{
	Use:   "activate <integration>",
	Short: "Activate an integration on your agents",
	Long:  "Grant agents permission to use an integration by updating their oc-agent.yaml files.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]

		// Verify integration exists and has credentials
		if !integration.CredentialExists(name) {
			return fmt.Errorf("integration '%s' is not configured — run %s first", name, ui.Bold("toc integrate add "+name))
		}

		def, err := integration.LoadFromRegistry(name)
		if err != nil {
			return fmt.Errorf("unknown integration '%s': %w", name, err)
		}

		promptActivateOnAgents(name, def)
		return nil
	},
}
