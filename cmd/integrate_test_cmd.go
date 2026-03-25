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
		testURL, authHeader := getTestEndpoint(name, def, cred)

		req, err := http.NewRequest("GET", testURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create test request: %w", err)
		}
		req.Header.Set("Authorization", authHeader)
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

func getTestEndpoint(name string, def *integration.Definition, cred *integration.Credential) (string, string) {
	// Use well-known test endpoints per integration
	switch name {
	case "github":
		authHeader := strings.ReplaceAll("Bearer {{token}}", "{{token}}", cred.AccessToken)
		return "https://api.github.com/user", authHeader
	default:
		// Find the first GET action and use that
		for _, action := range def.Actions {
			if action.Method == "GET" {
				authHeader := strings.ReplaceAll(action.AuthHeader, "{{token}}", cred.AccessToken)
				return action.Endpoint, authHeader
			}
		}
		// Last resort
		authHeader := strings.ReplaceAll("Bearer {{token}}", "{{token}}", cred.AccessToken)
		return def.Auth.SetupURL, authHeader
	}
}
