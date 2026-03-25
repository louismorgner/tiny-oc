package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	integrateCmd.AddCommand(integrateAddCmd)
}

var integrateAddCmd = &cobra.Command{
	Use:   "add <integration>",
	Short: "Add an integration (e.g. github, slack)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]

		// Load integration definition from registry
		def, err := integration.LoadFromRegistry(name)
		if err != nil {
			return fmt.Errorf("unknown integration '%s': %w", name, err)
		}

		// Check if already configured
		if integration.CredentialExists(name) {
			replace, err := ui.Confirm(fmt.Sprintf("Integration '%s' already configured. Replace credentials?", name), false)
			if err != nil || !replace {
				return nil
			}
		}

		fmt.Println()
		ui.Header(fmt.Sprintf("Add integration: %s", name))
		ui.Info("Description: %s", def.Description)
		ui.Info("Auth method: %s", def.Auth.Method)
		if def.Auth.SetupURL != "" {
			ui.Info("Setup URL: %s", ui.Cyan(def.Auth.SetupURL))
		}
		fmt.Println()

		var token string
		switch def.Auth.Method {
		case "token", "api_key":
			token, err = ui.Prompt("Enter access token (PAT)", "")
			if err != nil {
				return err
			}
			if token == "" {
				return fmt.Errorf("token cannot be empty")
			}
		case "oauth2":
			// For OAuth2, we still accept a PAT for now (v1 simplification)
			ui.Info("OAuth2 flow not yet implemented — enter a personal access token instead.")
			token, err = ui.Prompt("Enter access token", "")
			if err != nil {
				return err
			}
			if token == "" {
				return fmt.Errorf("token cannot be empty")
			}
		default:
			return fmt.Errorf("unsupported auth method: %s", def.Auth.Method)
		}

		cred := &integration.Credential{
			AccessToken: token,
		}

		if err := integration.StoreCredential(name, cred); err != nil {
			return fmt.Errorf("failed to store credentials: %w", err)
		}

		auditLog("integrate.add", map[string]interface{}{
			"integration": name,
			"auth_method": def.Auth.Method,
		})

		fmt.Println()
		ui.Success("Integration '%s' configured", name)
		ui.Info("Credentials encrypted at %s", ui.Dim(".toc/integrations/"+name+"/credentials.enc"))
		ui.Info("Test with: %s", ui.Bold(fmt.Sprintf("toc integrate test %s", name)))
		fmt.Println()
		return nil
	},
}
