package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/config"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

func init() {
	integrateCmd.AddCommand(integrateTestCmd)
}

var integrateTestCmd = &cobra.Command{
	Use:   "test <integration>",
	Short: "Test integration credentials",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := config.EnsureInitialized(); err != nil {
			return err
		}

		name := args[0]

		if !integration.CredentialExists(name) {
			return fmt.Errorf("integration '%s' is not configured — run 'toc integrate add %s' first", name, name)
		}

		def, err := integration.LoadFromRegistry(name)
		if err != nil {
			return fmt.Errorf("unknown integration '%s': %w", name, err)
		}

		cred, err := integration.LoadCredential(name)
		if err != nil {
			return fmt.Errorf("failed to load credentials: %w", err)
		}

		ui.Info("Testing %s credentials...", ui.Bold(name))

		// Use the first GET action as the test, or fall back to a known test endpoint
		testURL, headerName, headerValue := getTestEndpoint(name, def, cred)

		req, err := http.NewRequest("GET", testURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create test request: %w", err)
		}
		req.Header.Set(headerName, headerValue)
		req.Header.Set("User-Agent", "toc/1.0")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			ui.Error("Connection failed: %s", err)
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			ui.Success("Credentials valid — %s responded with %d", name, resp.StatusCode)
		} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
			ui.Error("Authentication failed — %s responded with %d", name, resp.StatusCode)
			ui.Info("Check your token and try again: %s", ui.Bold(fmt.Sprintf("toc integrate add %s", name)))
		} else {
			ui.Warn("Unexpected response — %s responded with %d", name, resp.StatusCode)
		}

		return nil
	},
}

// getTestEndpoint returns the URL, header name, and header value for testing credentials.
func getTestEndpoint(name string, def *integration.Definition, cred *integration.Credential) (string, string, string) {
	switch name {
	case "github":
		return "https://api.github.com/user", "Authorization", "Bearer " + cred.AccessToken
	case "slack":
		return "https://slack.com/api/auth.test", "Authorization", "Bearer " + cred.AccessToken
	case "exa":
		return "https://api.exa.ai/search", "x-api-key", cred.AccessToken
	default:
		// Find the first GET action and use that
		for _, action := range def.Actions {
			if action.Method == "GET" {
				headerName, headerValue := parseAuthHeader(action.AuthHeader, cred.AccessToken)
				return action.Endpoint, headerName, headerValue
			}
		}
		return def.Auth.SetupURL, "Authorization", "Bearer " + cred.AccessToken
	}
}

// parseAuthHeader splits an auth_header template into header name and value.
// If the template contains ": ", the left side is the header name.
// Otherwise, "Authorization" is used as the header name.
func parseAuthHeader(template, token string) (string, string) {
	if idx := strings.Index(template, ": "); idx > 0 {
		name := template[:idx]
		value := strings.ReplaceAll(template[idx+2:], "{{token}}", token)
		return name, value
	}
	value := strings.ReplaceAll(template, "{{token}}", token)
	return "Authorization", value
}
