package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	initCmd.Flags().String("name", "", "workspace name (skip interactive prompt)")
	initCmd.Flags().Bool("skip-key", false, "skip the OpenRouter API key prompt")
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a toc workspace in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.Exists() {
			return fmt.Errorf("workspace already initialized in this directory")
		}

		name, _ := cmd.Flags().GetString("name")

		if name == "" {
			ui.Header("Initialize workspace")

			var err error
			name, err = ui.Prompt("Workspace name", "")
			if err != nil {
				return err
			}
		}

		if name == "" {
			return fmt.Errorf("workspace name cannot be empty")
		}

		if err := config.Init(name); err != nil {
			return err
		}

		auditLog("workspace.init", map[string]interface{}{"name": name})

		fmt.Println()
		ui.Success("Initialized workspace %s", ui.Bold(name))

		// Optionally prompt for OpenRouter API key if not already set
		skipKey, _ := cmd.Flags().GetBool("skip-key")
		if !skipKey && os.Getenv("OPENROUTER_API_KEY") == "" {
			fmt.Println()
			ui.Info("toc-native agents use OpenRouter for LLM access.")
			ui.Info("Get a key at: %s", ui.Cyan("https://openrouter.ai/keys"))
			fmt.Println()
			setKey, err := ui.Confirm("Set an OpenRouter API key now?", false)
			if err == nil && setKey {
				key, err := ui.Prompt("OpenRouter API key", "")
				if err == nil && strings.TrimSpace(key) != "" {
					secrets, loadErr := config.LoadSecrets()
					if loadErr == nil {
						secrets.OpenRouterKey = strings.TrimSpace(key)
						if saveErr := config.SaveSecrets(secrets); saveErr == nil {
							ui.Success("API key stored in %s", ui.Dim(".toc/secrets.yaml"))
						} else {
							ui.Warn("Failed to store API key: %s", saveErr)
						}
					}
				}
			}
		}

		fmt.Println()
		ui.Info("Next steps:")
		ui.Info("  %s       Create a new agent from scratch", ui.Bold("toc agent create"))
		ui.Info("  %s     Browse the registry for templates", ui.Bold("toc registry search"))
		ui.Info("  %s  Install an agent or skill template", ui.Bold("toc registry install <name>"))
		fmt.Println()
		return nil
	},
}
