package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/audit"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/runtime"
	"github.com/tiny-oc/toc/internal/ui"
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

		// Step 3: Load integration definition
		def, err := integration.LoadFromRegistry(integrationName)
		if err != nil {
			return fmt.Errorf("unknown integration '%s': %w", integrationName, err)
		}

		if err := integration.ValidatePermissionsAgainstDefinition(integrationPerms, def); err != nil {
			return fmt.Errorf("invalid permissions in manifest: %w", err)
		}

		// Step 4: Load credentials
		cred, err := integration.LoadCredentialFromWorkspace(ctx.Workspace, integrationName)
		if err != nil {
			return fmt.Errorf("failed to load credentials for '%s': %w", integrationName, err)
		}

		// Slack: set up conversation resolver for permission checks and invocation.
		var resolver *integration.SlackChannelResolver
		if integrationName == "slack" {
			resolver = integration.NewSlackChannelResolver(cred.AccessToken)
		}

		// Step 5: Resolve the target and check permissions.
		target, err := integration.DeterminePermissionTarget(integrationName, actionName, params, resolver)
		if err != nil {
			return err
		}
		decision := integration.EvaluatePermission(def, integrationPerms, actionName, target)
		switch decision.Level {
		case agent.PermOff:
			return fmt.Errorf("permission denied: agent '%s' cannot perform '%s' on '%s'", ctx.Agent, actionName, target.Display())
		case agent.PermAsk:
			approved, alwaysAllow, err := requestInvocationApproval(ctx, integrationName, actionName, params, target)
			if err != nil {
				return err
			}
			if !approved {
				return fmt.Errorf("permission denied: user declined")
			}
			if alwaysAllow {
				if err := appendSessionPermissionOverride(ctx, integrationName, decision.Subject, target); err != nil {
					ui.Warn("Could not persist session-scoped approval override: %s", err)
				}
			}
		}

		// Use the canonical conversation ID for Slack once permission evaluation succeeded.
		if integrationName == "slack" && target.ID != "" {
			if _, ok := params["channel"]; ok {
				params["channel"] = target.ID
			}
		}

		// Step 6: Check rate limit
		actionDef, err := def.GetAction(actionName)
		if err != nil {
			return err
		}
		if !invokeRateLimiter.Allow(ctx.SessionID, integrationName+"."+actionName, actionDef.RateLimit) {
			return fmt.Errorf("rate limit exceeded for %s.%s — try again later", integrationName, actionName)
		}

		// Step 7: Make the call
		invokeReq := &integration.InvokeRequest{
			SessionID:   ctx.SessionID,
			Integration: integrationName,
			Action:      actionName,
			Params:      params,
			Credential:  cred,
			Definition:  def,
			Workspace:   ctx.Workspace,
		}

		// Slack: re-use the permission-time resolver so channel IDs stay consistent.
		if integrationName == "slack" {
			invokeReq.ChannelResolver = resolver
		}

		resp, err := integration.Invoke(invokeReq)
		if err != nil {
			return fmt.Errorf("invocation failed: %w", err)
		}

		// Step 8: Log to audit
		_ = audit.LogFromWorkspace(ctx.Workspace, "runtime.invoke", map[string]interface{}{
			"agent":       ctx.Agent,
			"session_id":  ctx.SessionID,
			"integration": integrationName,
			"action":      actionName,
			"target":      target.Display(),
			"status_code": resp.StatusCode,
		})

		// Step 9: Output response as JSON
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
	manifest, err := runtime.LoadPermissionManifestInWorkspace(ctx.Workspace, ctx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("permission manifest not found (session %s): %w", ctx.SessionID, err)
	}
	return manifest, nil
}

// determineTarget extracts the scope target from params based on the integration type.
func requestInvocationApproval(ctx *runtime.Context, integrationName, actionName string, params map[string]string, target integration.PermissionTarget) (bool, bool, error) {
	if ui.IsTTY(os.Stdin) && ui.IsTTY(os.Stdout) {
		choice, err := ui.Select(
			fmt.Sprintf("[%s] Agent %q wants to run %s on %s", integrationName, ctx.Agent, actionName, target.Display()),
			[]ui.SelectOption{
				{Label: "Allow once", Value: "allow"},
				{Label: "Always allow this target for this session", Value: "allow_always"},
				{Label: "Deny", Value: "deny"},
			},
			0,
		)
		if err != nil {
			return false, false, err
		}
		return choice == "allow" || choice == "allow_always", choice == "allow_always", nil
	}

	approvalID, err := runtime.WritePendingApproval(ctx.Workspace, ctx.SessionID, runtime.PendingApprovalRequest{
		SessionID:   ctx.SessionID,
		Agent:       ctx.Agent,
		Integration: integrationName,
		Action:      actionName,
		Target:      target.Display(),
		Params:      params,
	})
	if err != nil {
		return false, false, err
	}

	resp, err := runtime.WaitForPendingApproval(ctx.Workspace, ctx.SessionID, approvalID, 5*time.Minute)
	if err != nil {
		return false, false, fmt.Errorf("permission denied: %w", err)
	}

	switch resp.Decision {
	case "allow":
		return true, false, nil
	case "allow_always":
		return true, true, nil
	default:
		return false, false, nil
	}
}

func appendSessionPermissionOverride(ctx *runtime.Context, integrationName, subject string, target integration.PermissionTarget) error {
	manifest, err := runtime.LoadPermissionManifestInWorkspace(ctx.Workspace, ctx.SessionID)
	if err != nil {
		return err
	}
	override := agent.IntegrationPermissionGrant{
		Mode:       agent.PermOn,
		Capability: subject + ":" + target.ExactPermissionScope(),
	}
	manifest.Integrations[integrationName] = append(manifest.Integrations[integrationName], override)
	path := filepath.Join(ctx.Workspace, ".toc", "sessions", ctx.SessionID, "permissions.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
