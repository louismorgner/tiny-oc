package cmd

import (
	"encoding/json"
	"fmt"
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

Use --help at any level to discover available actions and parameters:
  toc runtime invoke slack --help              # list actions
  toc runtime invoke slack send_message --help # show params for an action

Channel resolution: channel names (e.g. #general) are automatically resolved
to Slack channel IDs. If resolution fails, inputs matching Slack's ID format
(starting with C, D, or G followed by alphanumeric characters) are used as-is.

Examples:
  toc runtime invoke github issues.read --repo louismorgner/tiny-oc
  toc runtime invoke github issues.write --repo louismorgner/tiny-oc --title "Bug report"
  toc runtime invoke github pulls.read --repo louismorgner/tiny-oc --state open`,
	Args:               cobra.ArbitraryArgs,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle no-args case
		if len(args) == 0 {
			return cmd.Help()
		}

		// Check for --help as first argument
		if args[0] == "--help" || args[0] == "-h" {
			return cmd.Help()
		}

		integrationName := args[0]

		// Handle: toc runtime invoke <integration> --help
		if len(args) == 1 || (len(args) >= 2 && (args[1] == "--help" || args[1] == "-h")) {
			return printIntegrationHelp(integrationName)
		}

		actionName := args[1]

		// Handle: toc runtime invoke <integration> <action> --help
		if hasHelpFlag(args[2:]) {
			return printActionHelp(integrationName, actionName)
		}

		ctx, err := runtime.FromEnv()
		if err != nil {
			return err
		}

		// Initialize file-backed rate limiter for this session
		rateLimitPath := filepath.Join(ctx.Workspace, ".toc", "sessions", ctx.SessionID, "rate_limits.json")
		invokeRateLimiter = integration.NewRateLimiter(rateLimitPath)

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
			return integration.NewNoIntegrationPermError(ctx.Agent, integrationName)
		}

		parsedPerms, err := integration.ParsePermissions(integrationPerms)
		if err != nil {
			return fmt.Errorf("invalid permissions in manifest: %w", err)
		}

		// Determine the target scope from params
		target := determineTarget(integrationName, actionName, params)
		if !integration.CheckPermission(parsedPerms, actionName, target) {
			return integration.NewPermissionDeniedError(ctx.Agent, integrationName, actionName)
		}

		// Step 3: Load integration definition
		def, err := integration.LoadFromRegistry(integrationName)
		if err != nil {
			return fmt.Errorf("unknown integration '%s': %w", integrationName, err)
		}

		// Step 4: Check rate limit — also validates the action exists
		actionDef, err := def.GetAction(actionName)
		if err != nil {
			available := def.ActionNames()
			return integration.NewActionNotFoundError(integrationName, actionName, available)
		}
		if !invokeRateLimiter.Allow(ctx.SessionID, integrationName+"."+actionName, actionDef.RateLimit) {
			return fmt.Errorf("rate limit exceeded for %s.%s — try again later", integrationName, actionName)
		}

		// Step 5: Load credentials
		cred, err := integration.LoadCredentialFromWorkspace(ctx.Workspace, integrationName)
		if err != nil {
			return integration.NewCredentialError(integrationName, err)
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

func hasHelpFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--help" || a == "-h" {
			return true
		}
		// Skip the value following a --key flag so we don't match
		// help strings inside parameter values (e.g. --text "--help").
		if strings.HasPrefix(a, "--") && !strings.Contains(a, "=") && i+1 < len(args) {
			i++ // skip value
		}
	}
	return false
}

// printIntegrationHelp shows available actions for an integration.
func printIntegrationHelp(name string) error {
	def, err := integration.LoadFromRegistry(name)
	if err != nil {
		return fmt.Errorf("unknown integration '%s': %w", name, err)
	}

	fmt.Printf("Integration: %s\n", def.Name)
	if def.Description != "" {
		fmt.Printf("  %s\n", def.Description)
	}
	fmt.Println()
	fmt.Println("Available actions:")

	names := def.ActionNames()
	for _, actionName := range names {
		action := def.Actions[actionName]
		fmt.Printf("  %-20s %s\n", actionName, action.Description)
	}

	fmt.Println()
	fmt.Println("Use toc runtime invoke", name, "<action> --help for details on a specific action.")
	return nil
}

// printActionHelp shows parameters for a specific action.
func printActionHelp(integrationName, actionName string) error {
	def, err := integration.LoadFromRegistry(integrationName)
	if err != nil {
		return fmt.Errorf("unknown integration '%s': %w", integrationName, err)
	}

	actionDef, err := def.GetAction(actionName)
	if err != nil {
		available := def.ActionNames()
		return integration.NewActionNotFoundError(integrationName, actionName, available)
	}

	fmt.Printf("Action: %s.%s\n", integrationName, actionName)
	if actionDef.Description != "" {
		fmt.Printf("  %s\n", actionDef.Description)
	}
	fmt.Println()

	// Show parameters
	hasRequired := false
	hasOptional := false
	for _, p := range actionDef.Params {
		if p.Required {
			hasRequired = true
		} else {
			hasOptional = true
		}
	}

	if hasRequired {
		fmt.Println("Required:")
		for _, p := range actionDef.Params {
			if p.Required {
				fmt.Printf("  --%s\n", p.Name)
			}
		}
	}

	if hasOptional {
		fmt.Println("Optional:")
		for _, p := range actionDef.Params {
			if !p.Required {
				defaultStr := ""
				if p.Default != "" {
					defaultStr = fmt.Sprintf(" (default: %s)", p.Default)
				}
				fmt.Printf("  --%s%s\n", p.Name, defaultStr)
			}
		}
	}

	if len(actionDef.Params) == 0 {
		fmt.Println("  No parameters.")
	}

	// Show channel resolution note for Slack channel params
	if integrationName == "slack" {
		for _, p := range actionDef.Params {
			if p.Name == "channel" {
				fmt.Println()
				fmt.Println("Channel resolution:")
				fmt.Println("  Channel names (e.g. #general) are resolved to IDs automatically.")
				fmt.Println("  If resolution fails, values matching Slack ID format (C/D/G + alphanumeric)")
				fmt.Println("  are used as raw channel IDs.")
				break
			}
		}
	}

	fmt.Println()
	fmt.Printf("Example:\n  toc runtime invoke %s %s", integrationName, actionName)
	for _, p := range actionDef.Params {
		if p.Required {
			fmt.Printf(" --%s <value>", p.Name)
		}
	}
	fmt.Println()

	return nil
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
