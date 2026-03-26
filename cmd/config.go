package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetKeyCmd)
	configCmd.AddCommand(configGetKeyCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage workspace configuration",
}

var configSetKeyCmd = &cobra.Command{
	Use:   "set-key <provider> [key]",
	Short: "Store an API key for a provider (e.g. openrouter)",
	Long:  "Store an API key securely in .toc/secrets.yaml. If the key is not provided as an argument, you will be prompted to enter it.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		provider := strings.ToLower(args[0])
		if provider != "openrouter" {
			return fmt.Errorf("unknown provider '%s' — supported providers: openrouter", provider)
		}

		var key string
		if len(args) >= 2 {
			key = strings.TrimSpace(args[1])
		} else {
			fmt.Println()
			ui.Info("Get your API key at: %s", ui.Cyan("https://openrouter.ai/keys"))
			fmt.Println()
			var err error
			key, err = ui.Prompt("OpenRouter API key", "")
			if err != nil {
				return err
			}
		}

		if key == "" {
			return fmt.Errorf("API key cannot be empty")
		}

		secrets, err := config.LoadSecrets()
		if err != nil {
			return err
		}

		secrets.OpenRouterKey = key
		if err := config.SaveSecrets(secrets); err != nil {
			return err
		}

		auditLog("config.set-key", map[string]interface{}{"provider": provider})

		fmt.Println()
		ui.Success("OpenRouter API key stored in %s", ui.Dim(".toc/secrets.yaml"))
		fmt.Println()
		return nil
	},
}

var configGetKeyCmd = &cobra.Command{
	Use:   "get-key <provider>",
	Short: "Check if an API key is configured for a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		provider := strings.ToLower(args[0])
		if provider != "openrouter" {
			return fmt.Errorf("unknown provider '%s' — supported providers: openrouter", provider)
		}

		// Load secrets from workspace root — handles both direct workspace
		// and session context (where CWD is a temp dir but TOC_WORKSPACE is set).
		secrets, err := config.LoadSecretsFrom(config.WorkspaceRoot())
		if err != nil {
			return err
		}

		fmt.Println()
		if secrets.OpenRouterKey != "" {
			// Show first 8 chars, mask the rest
			masked := secrets.OpenRouterKey[:min(8, len(secrets.OpenRouterKey))] + "..."
			ui.Success("OpenRouter API key is configured: %s", ui.Dim(masked))
		} else {
			ui.Warn("No OpenRouter API key configured.")
			ui.Info("Set one with: %s", ui.Bold("toc config set-key openrouter"))
		}
		fmt.Println()
		return nil
	},
}
