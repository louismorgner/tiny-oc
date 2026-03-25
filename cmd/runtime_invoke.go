package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/runtime"
)

// invokeRateLimiter is initialized per-invocation with the session path.
var invokeRateLimiter *integration.RateLimiter

func init() {
	runtimeCmd.AddCommand(runtimeInvokeCmd)
}

var runtimeInvokeCmd = &cobra.Command{
	Use:   "invoke <integration> <action> [--param value...]",
	Short: "Invoke an integration action through the gateway",
	Long: `Invoke an external API action through the toc gateway.

The gateway enforces permissions from the session manifest, loads encrypted
credentials, makes the HTTP call, and returns a filtered response.

Examples:
  toc runtime invoke github issues.read --repo louismorgner/tiny-oc
  toc runtime invoke github issues.write --repo louismorgner/tiny-oc --title "Bug report"
  toc runtime invoke github pulls.read --repo louismorgner/tiny-oc --state open`,
	Args:               cobra.MinimumNArgs(2),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		// Initialize file-backed rate limiter for this session
		rateLimitPath := filepath.Join(ctx.Workspace, ".toc", "sessions", ctx.SessionID, "rate_limits.json")
		invokeRateLimiter = integration.NewRateLimiter(rateLimitPath)

		integrationName := args[0]
		actionName := args[1]

		// Parse remaining args as --key value pairs
		params := parseInvokeParams(args[2:])

		// Step 1: Load permission manifest
		manifest, err := loadPermissionManifest(ctx)
		if err != nil {
			return fmt.Errorf("failed to load permission manifest: %w", err)
		}

		// Step 2: Check permissions
		integrationPerms, ok := manifest.Integrations[integrationName]
		if !ok {
			return fmt.Errorf("agent '%s' has no permissions for integration '%s'", ctx.Agent, integrationName)
		}

		parsedPerms, err := integration.ParsePermissions(integrationPerms)
		if err != nil {
			return fmt.Errorf("invalid permissions in manifest: %w", err)
		}

		// Determine the target scope from params
		target := determineTarget(integrationName, actionName, params)
		if !integration.CheckPermission(parsedPerms, actionName, target) {
			return fmt.Errorf("permission denied: agent '%s' cannot perform '%s' on '%s'", ctx.Agent, actionName, target)
		}

		// Step 3: Load integration definition
		def, err := integration.LoadFromRegistry(integrationName)
		if err != nil {
			return fmt.Errorf("unknown integration '%s': %w", integrationName, err)
		}

		// Step 4: Check rate limit
		actionDef, err := def.GetAction(actionName)
		if err != nil {
			return err
		}
		if !invokeRateLimiter.Allow(ctx.SessionID, integrationName+"."+actionName, actionDef.RateLimit) {
			return fmt.Errorf("rate limit exceeded for %s.%s — try again later", integrationName, actionName)
		}

		// Step 5: Load credentials
		cred, err := integration.LoadCredentialFromWorkspace(ctx.Workspace, integrationName)
		if err != nil {
			return fmt.Errorf("failed to load credentials for '%s': %w", integrationName, err)
		}

		// Step 6: Make the call
		invokeReq := &integration.InvokeRequest{
			SessionID:   ctx.SessionID,
			Integration: integrationName,
			Action:      actionName,
			Params:      params,
			Credential:  cred,
			Definition:  def,
			Workspace:   ctx.Workspace,
		}

		// Slack: set up channel resolver for transparent name-to-ID translation
		if integrationName == "slack" {
			invokeReq.ChannelResolver = integration.NewSlackChannelResolver(cred.AccessToken)
		}

		resp, err := integration.Invoke(invokeReq)
		if err != nil {
			return fmt.Errorf("invocation failed: %w", err)
		}

		// Step 7: Log to audit
		_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.invoke", map[string]interface{}{
			"agent":       ctx.Agent,
			"session_id":  ctx.SessionID,
			"integration": integrationName,
			"action":      actionName,
			"target":      target,
			"status_code": resp.StatusCode,
		})

		// Step 8: Output response as JSON
		output, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(output))
		return nil
	},
}

func parseInvokeParams(args []string) map[string]string {
	params := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		key := strings.TrimPrefix(arg, "--")

		// Handle --key=value format
		if idx := strings.Index(key, "="); idx >= 0 {
			params[key[:idx]] = key[idx+1:]
			continue
		}

		// Handle --key value format
		if i+1 < len(args) {
			params[key] = args[i+1]
			i++
		}
	}
	return params
}

func loadPermissionManifest(ctx *runtime.Context) (*integration.PermissionManifest, error) {
	path := filepath.Join(ctx.Workspace, ".toc", "sessions", ctx.SessionID, "permissions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("permission manifest not found (session %s): %w", ctx.SessionID, err)
	}

	var manifest integration.PermissionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid permission manifest: %w", err)
	}

	return &manifest, nil
}

// determineTarget extracts the scope target from params based on the integration type.
func determineTarget(integrationName, actionName string, params map[string]string) string {
	switch integrationName {
	case "github":
		if repo, ok := params["repo"]; ok {
			return repo
		}
	case "slack":
		if ch, ok := params["channel"]; ok {
			return ch
		}
	case "linear":
		if team, ok := params["team"]; ok {
			return "team/" + team
		}
	}

	// Fall back to "*" for actions without a clear target
	return "*"
}
