package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

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

		var cred *integration.Credential

		switch def.Auth.Method {
		case "token", "api_key":
			// Print setup instructions if available
			if def.Auth.SetupInstructions != "" {
				ui.Header("Setup instructions")
				for _, line := range strings.Split(strings.TrimSpace(def.Auth.SetupInstructions), "\n") {
					fmt.Println("  " + line)
				}
				fmt.Println()
			} else if def.Auth.SetupURL != "" {
				ui.Info("Create or configure your app at: %s", ui.Cyan(def.Auth.SetupURL))
			}
			if len(def.Auth.RequiredScopes) > 0 {
				ui.Info("Required scopes: %s", ui.Bold(strings.Join(def.Auth.RequiredScopes, ", ")))
				fmt.Println()
			}

			token, err := ui.Prompt("Paste your access token", "")
			if err != nil {
				return err
			}
			if token == "" {
				return fmt.Errorf("token cannot be empty")
			}
			cred = &integration.Credential{AccessToken: token}

		case "oauth2":
			var err error
			cred, err = runOAuth2Flow(name, def)
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unsupported auth method: %s", def.Auth.Method)
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

func runOAuth2Flow(name string, def *integration.Definition) (*integration.Credential, error) {
	ui.Info("This integration uses OAuth2. You'll need your app's Client ID and Client Secret.")
	if def.Auth.SetupURL != "" {
		ui.Info("Create an app at: %s", ui.Cyan(def.Auth.SetupURL))
	}
	fmt.Println()

	clientID, err := ui.Prompt("Enter Client ID", "")
	if err != nil {
		return nil, err
	}
	if clientID == "" {
		return nil, fmt.Errorf("client ID cannot be empty")
	}

	clientSecret, err := ui.Prompt("Enter Client Secret", "")
	if err != nil {
		return nil, err
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("client secret cannot be empty")
	}

	// For non-slack integrations, fall back to PAT until we add provider-specific configs
	if name != "slack" {
		ui.Info("Generic OAuth2 flow — enter a personal access token instead.")
		token, err := ui.Prompt("Enter access token", "")
		if err != nil {
			return nil, err
		}
		if token == "" {
			return nil, fmt.Errorf("token cannot be empty")
		}
		return &integration.Credential{AccessToken: token}, nil
	}

	oauth2Cfg := integration.SlackOAuth2Config(clientID, clientSecret, def.Auth.RequiredScopes)

	authURL := oauth2Cfg.AuthorizationURL()
	fmt.Println()
	ui.Info("Opening browser for authorization...")
	ui.Info("If the browser doesn't open, visit: %s", ui.Cyan(authURL))
	fmt.Println()

	// Open browser
	openBrowser(authURL)

	// Start callback server with 5-minute timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ui.Info("Waiting for OAuth callback on localhost:%d...", oauth2Cfg.RedirectPort)

	code, err := oauth2Cfg.RunCallbackServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("OAuth2 authorization failed: %w", err)
	}

	ui.Info("Authorization code received, exchanging for tokens...")

	cred, err := oauth2Cfg.ExchangeCode(code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Store client credentials separately for future token refresh
	clientCfg := &integration.OAuth2ClientConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	if err := integration.StoreOAuth2ClientConfig(name, clientCfg); err != nil {
		ui.Warn("Could not store client info for token refresh: %s", err)
	}

	return cred, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		fmt.Printf("Unsupported platform — please open the URL manually.\n")
		return
	}
	_ = cmd.Start()
}
