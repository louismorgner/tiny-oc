package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		testReq := getTestRequest(name, def, cred)

		// For Slack, use the shared auth test helper
		if name == "slack" {
			data, err := callSlackAuthTest(cred.AccessToken)
			if err != nil {
				ui.Error("Authentication failed — %s", err)
				ui.Info("Re-add with: %s", ui.Bold("toc integrate add slack"))

				// Warn about bot vs user token for search
				if strings.HasPrefix(cred.AccessToken, "xoxb-") {
					ui.Warn("You're using a bot token (xoxb-). search:read requires a user token (xoxp-).")
				}

				return nil
			}

			user, _ := data["user"].(string)
			team, _ := data["team"].(string)
			ui.Success("Credentials valid — authenticated as %s in workspace %s", ui.Bold(user), ui.Bold(team))
			return nil
		}

		var bodyReader *bytes.Reader
		if testReq.body != nil {
			bodyReader = bytes.NewReader(testReq.body)
		}

		var req *http.Request
		if bodyReader != nil {
			req, err = http.NewRequest(testReq.method, testReq.url, bodyReader)
		} else {
			req, err = http.NewRequest(testReq.method, testReq.url, nil)
		}
		if err != nil {
			return fmt.Errorf("failed to create test request: %w", err)
		}
		req.Header.Set(testReq.headerName, testReq.headerValue)
		if testReq.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("User-Agent", "toc/1.0")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			ui.Error("Connection failed: %s", err)
			return nil
		}
		defer resp.Body.Close()

		// Non-Slack: check HTTP status code
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

// testRequest describes the HTTP request to send for credential testing.
type testRequest struct {
	method      string
	url         string
	headerName  string
	headerValue string
	body        []byte // nil for GET requests
}

// getTestRequest returns the full test request config for the given integration.
func getTestRequest(name string, def *integration.Definition, cred *integration.Credential) testRequest {
	switch name {
	case "github":
		return testRequest{
			method: "GET", url: "https://api.github.com/user",
			headerName: "Authorization", headerValue: "Bearer " + cred.AccessToken,
		}
	case "slack":
		return testRequest{
			method: "GET", url: "https://slack.com/api/auth.test",
			headerName: "Authorization", headerValue: "Bearer " + cred.AccessToken,
		}
	case "exa":
		return testRequest{
			method: "POST", url: "https://api.exa.ai/search",
			headerName: "x-api-key", headerValue: cred.AccessToken,
			body: []byte(`{"query":"test","numResults":1}`),
		}
	default:
		// Find the first GET action and use that
		for _, action := range def.Actions {
			if action.Method == "GET" {
				headerName, headerValue := parseAuthHeader(action.AuthHeader, cred.AccessToken)
				return testRequest{
					method: "GET", url: action.Endpoint,
					headerName: headerName, headerValue: headerValue,
				}
			}
		}
		return testRequest{
			method: "GET", url: def.Auth.SetupURL,
			headerName: "Authorization", headerValue: "Bearer " + cred.AccessToken,
		}
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
