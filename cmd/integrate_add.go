package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	integrateAddCmd.Flags().Bool("manual", false, "Manually paste the authorization code instead of using a local callback server")
	integrateAddCmd.Flags().String("token", "", "Access token (skip interactive prompt)")
	integrateAddCmd.Flags().String("client-id", "", "OAuth2 client ID (skip interactive prompt)")
	integrateAddCmd.Flags().String("client-secret", "", "OAuth2 client secret (skip interactive prompt)")
	integrateAddCmd.Flags().BoolP("force", "f", false, "Replace existing credentials without confirmation")
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
		force, _ := cmd.Flags().GetBool("force")
		if integration.CredentialExists(name) && !force {
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
			token, _ := cmd.Flags().GetString("token")
			if token == "" {
				// Print setup instructions if available
				if def.Auth.SetupInstructions != "" {
					ui.Header("Setup instructions")
					for _, line := range strings.Split(strings.TrimSpace(def.Auth.SetupInstructions), "\n") {
						fmt.Println("  " + line)
					}
					fmt.Println()
				}
				if len(def.Auth.RequiredScopes) > 0 {
					ui.Info("Required scopes: %s", ui.Bold(strings.Join(def.Auth.RequiredScopes, ", ")))
					fmt.Println()
				}

				var err error
				token, err = ui.Prompt("Paste your access token", "")
				if err != nil {
					return err
				}
			}
			if token == "" {
				return fmt.Errorf("token cannot be empty")
			}
			cred = &integration.Credential{AccessToken: token}

		case "oauth2":
			manual, _ := cmd.Flags().GetBool("manual")
			flagClientID, _ := cmd.Flags().GetString("client-id")
			flagClientSecret, _ := cmd.Flags().GetString("client-secret")
			flagToken, _ := cmd.Flags().GetString("token")
			var err error
			cred, err = runOAuth2Flow(name, def, manual, flagClientID, flagClientSecret, flagToken)
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

		// Run inline auth verification for Slack
		if name == "slack" {
			fmt.Println()
			ui.Info("Verifying credentials with auth.test...")
			if err := runInlineAuthTest(cred); err != nil {
				ui.Warn("Credential verification failed: %s", err)
				ui.Info("You can retry with: %s", ui.Bold("toc integrate test slack"))
			}
		} else {
			ui.Info("Test with: %s", ui.Bold(fmt.Sprintf("toc integrate test %s", name)))
		}

		fmt.Println()
		return nil
	},
}

func runOAuth2Flow(name string, def *integration.Definition, manual bool, flagClientID, flagClientSecret, flagToken string) (*integration.Credential, error) {
	clientID := flagClientID
	clientSecret := flagClientSecret

	// Show setup instructions as a numbered wizard for OAuth2 integrations
	if (clientID == "" || clientSecret == "") && def.Auth.SetupInstructions != "" {
		showSetupWizard(def)
	} else if clientID == "" || clientSecret == "" {
		ui.Info("This integration uses OAuth2. You'll need your app's Client ID and Client Secret.")
		if def.Auth.SetupURL != "" {
			ui.Info("Create an app at: %s", ui.Cyan(def.Auth.SetupURL))
		}
		fmt.Println()
	}

	if clientID == "" {
		var err error
		clientID, err = ui.Prompt("Enter Client ID", "")
		if err != nil {
			return nil, err
		}
	}
	if clientID == "" {
		return nil, fmt.Errorf("client ID cannot be empty")
	}

	// Validate credential format for Slack
	if name == "slack" {
		if err := integration.ValidateSlackClientID(clientID); err != nil {
			return nil, err
		}
	}

	if clientSecret == "" {
		var err error
		clientSecret, err = ui.Prompt("Enter Client Secret", "")
		if err != nil {
			return nil, err
		}
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("client secret cannot be empty")
	}

	// Validate credential format for Slack
	if name == "slack" {
		if err := integration.ValidateSlackClientSecret(clientSecret); err != nil {
			return nil, err
		}
	}

	// For non-slack integrations, fall back to PAT until we add provider-specific configs
	if name != "slack" {
		token := flagToken
		if token == "" {
			ui.Info("Generic OAuth2 flow — enter a personal access token instead.")
			var err error
			token, err = ui.Prompt("Enter access token", "")
			if err != nil {
				return nil, err
			}
		}
		if token == "" {
			return nil, fmt.Errorf("token cannot be empty")
		}
		return &integration.Credential{AccessToken: token}, nil
	}

	oauth2Cfg := integration.SlackOAuth2Config(clientID, clientSecret, def.Auth.RequiredScopes, def.Auth.UserScopes)

	authURL := oauth2Cfg.AuthorizationURL()
	fmt.Println()

	// Store client credentials for future token refresh
	clientCfg := &integration.OAuth2ClientConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	var code string
	var codeErr error
	if manual {
		code, codeErr = runManualOAuth2Flow(authURL)
	} else {
		code, codeErr = runHostedOAuth2Flow(authURL, oauth2Cfg)
	}
	if codeErr != nil {
		return nil, fmt.Errorf("OAuth2 authorization failed: %w", codeErr)
	}

	ui.Info("Authorization code received, exchanging for tokens...")

	cred, err := oauth2Cfg.ExchangeCode(code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	if err := integration.StoreOAuth2ClientConfig(name, clientCfg); err != nil {
		ui.Warn("Could not store client info for token refresh: %s", err)
	}

	return cred, nil
}

// showSetupWizard displays the integration's setup instructions as a numbered
// wizard with pauses between steps so the user can complete each one.
func showSetupWizard(def *integration.Definition) {
	ui.Header("Setup wizard")

	lines := strings.Split(strings.TrimSpace(def.Auth.SetupInstructions), "\n")

	// Show required scopes up front
	scopes := def.Auth.UserScopes
	if len(scopes) == 0 {
		scopes = def.Auth.RequiredScopes
	}
	if len(scopes) > 0 {
		ui.Info("Required scopes: %s", ui.Bold(strings.Join(scopes, ", ")))
		fmt.Println()
	}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Highlight the redirect URL step
		if strings.Contains(line, "Redirect URL") || strings.Contains(line, "redirect_uri") || strings.Contains(line, SlackOAuthCallbackURL()) {
			fmt.Println("  " + ui.Yellow(line))
			fmt.Println()
			ui.Warn("OAuth will fail if the redirect URL is not added exactly as shown above.")
			fmt.Println()
		} else {
			fmt.Println("  " + line)
			fmt.Println()
		}

		// Pause between steps (except the last one, which is usually "run toc integrate add")
		if i < len(lines)-1 {
			ui.WaitForEnter("Press Enter when ready for the next step...")
			fmt.Println()
		}
	}
}

// SlackOAuthCallbackURL returns the callback URL for display purposes.
func SlackOAuthCallbackURL() string {
	return integration.SlackOAuthCallbackURL
}

// runHostedOAuth2Flow opens the browser and starts a localhost callback server.
// The hosted HTTPS worker receives Slack's redirect and bounces the user back
// to localhost where the code is captured automatically.
func runHostedOAuth2Flow(authURL string, oauth2Cfg *integration.OAuth2Config) (string, error) {
	ui.Info("Opening browser for authorization...")
	ui.Info("If the browser doesn't open, visit: %s", ui.Cyan(authURL))
	fmt.Println()

	openBrowser(authURL)

	ui.Info("Waiting for authorization callback on localhost:%d...", oauth2Cfg.RedirectPort)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return oauth2Cfg.RunCallbackServer(ctx)
}

// runManualOAuth2Flow displays the auth URL and prompts the user to paste the code.
// Fallback for environments where the browser can't be opened automatically.
func runManualOAuth2Flow(authURL string) (string, error) {
	ui.Info("Open this URL in your browser to authorize:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	ui.Info("After authorizing, copy the code shown on the page and paste it here:")
	fmt.Println()

	raw, err := ui.Prompt("Paste authorization code", "")
	if err != nil {
		return "", err
	}

	return integration.ParseCodeFromURL(raw)
}

// runInlineAuthTest calls Slack's auth.test endpoint immediately after OAuth
// to verify the token works, so the user doesn't have to run a separate command.
func runInlineAuthTest(cred *integration.Credential) error {
	req, err := http.NewRequest("GET", "https://slack.com/api/auth.test", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("User-Agent", "toc/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("invalid response from Slack")
	}

	if err := integration.CheckSlackResponse(resp.StatusCode, data); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "invalid_auth") {
			return fmt.Errorf("%s — the token may have been revoked or is malformed", errStr)
		}
		if strings.Contains(errStr, "missing_scope") {
			return fmt.Errorf("%s — re-add the integration with the correct scopes", errStr)
		}
		return err
	}

	user, _ := data["user"].(string)
	team, _ := data["team"].(string)
	ui.Success("Authenticated as %s in workspace %s", ui.Bold(user), ui.Bold(team))
	return nil
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
