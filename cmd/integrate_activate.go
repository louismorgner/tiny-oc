package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tiny-oc/toc/internal/agent"
	"github.com/tiny-oc/toc/internal/integration"
	"github.com/tiny-oc/toc/internal/ui"
)

// promptActivateOnAgents runs after a successful integration add. It offers the
// user a TUI flow to activate the integration on one or more agents by writing
// permission grants into their oc-agent.yaml files.
func promptActivateOnAgents(name string, def *integration.Definition) {
	agents, err := agent.List()
	if err != nil {
		ui.Warn("Could not list agents: %s", err)
		return
	}
	if len(agents) == 0 {
		return
	}

	fmt.Println()
	ui.Header("Activate on agents")
	ui.Info("Grant your agents permission to use %s.", ui.Bold(name))
	fmt.Println()

	// Step 1: Select agents
	agentOpts := make([]ui.SelectOption, len(agents))
	for i, a := range agents {
		desc := a.Description
		if desc == "" {
			desc = a.Runtime + "/" + a.Model
		}
		agentOpts[i] = ui.SelectOption{
			Label: fmt.Sprintf("%s  %s", a.Name, ui.Dim(desc)),
			Value: a.Name,
		}
	}

	selectedAgents, err := ui.MultiSelect("Select agents to activate "+ui.Bold(name)+" on", agentOpts, nil)
	if err != nil || len(selectedAgents) == 0 {
		fmt.Println()
		ui.Info("Skipped. You can activate later by editing oc-agent.yaml or running:")
		ui.Command(fmt.Sprintf("toc integrate activate %s", name))
		fmt.Println()
		return
	}

	// Step 2: Select capabilities/permissions
	grants := selectCapabilities(name, def)
	if len(grants) == 0 {
		fmt.Println()
		ui.Info("No capabilities selected, skipping activation.")
		fmt.Println()
		return
	}

	// Step 3: Choose permission mode
	mode, err := ui.Select("Permission mode", []ui.SelectOption{
		{Label: "on   " + ui.Dim("— agent can use without asking"), Value: "on"},
		{Label: "ask  " + ui.Dim("— agent must ask before each use"), Value: "ask"},
	}, 0)
	if err != nil {
		return
	}

	// Build the permission grants
	permGrants := make(agent.IntegrationPermissions, len(grants))
	for i, cap := range grants {
		permGrants[i] = agent.IntegrationPermissionGrant{
			Mode:       agent.PermissionLevel(mode),
			Capability: cap,
		}
	}

	// Step 4: Show summary and confirm
	fmt.Println()
	ui.Header("Summary")
	for _, agentName := range selectedAgents {
		fmt.Printf("  %s %s\n", ui.Bold(agentName), ui.Dim("→"))
		for _, g := range permGrants {
			fmt.Printf("    %s %s\n", ui.Green("•"), g)
		}
	}
	fmt.Println()

	confirm, err := ui.Confirm("Apply these permissions?", true)
	if err != nil || !confirm {
		ui.Info("Skipped. Edit oc-agent.yaml manually to configure permissions.")
		fmt.Println()
		return
	}

	// Step 5: Apply to each agent
	var applied, failed []string
	for _, agentName := range selectedAgents {
		cfg, err := agent.Load(agentName)
		if err != nil {
			ui.Error("Failed to load %s: %s", agentName, err)
			failed = append(failed, agentName)
			continue
		}

		if cfg.Perms == nil {
			cfg.Perms = &agent.Permissions{}
		}
		if cfg.Perms.Integrations == nil {
			cfg.Perms.Integrations = make(map[string]agent.IntegrationPermissions)
		}

		// Merge new grants with existing ones (avoid duplicates)
		existing := cfg.Perms.Integrations[name]
		merged := mergeGrants(existing, permGrants)
		cfg.Perms.Integrations[name] = merged

		if err := agent.Save(cfg); err != nil {
			ui.Error("Failed to save %s: %s", agentName, err)
			failed = append(failed, agentName)
			continue
		}
		applied = append(applied, agentName)
	}

	fmt.Println()
	if len(applied) > 0 {
		ui.Success("Activated %s on %s", ui.Bold(name), ui.Bold(strings.Join(applied, ", ")))
	}
	if len(failed) > 0 {
		ui.Warn("Failed for: %s", strings.Join(failed, ", "))
	}
	fmt.Println()
}

// selectCapabilities builds the capability selection UI. If the integration has
// named capabilities, offers those. Otherwise falls back to action-level grants.
func selectCapabilities(name string, def *integration.Definition) []string {
	if len(def.Capabilities) > 0 {
		return selectFromCapabilities(def)
	}
	return selectFromActions(def)
}

func selectFromCapabilities(def *integration.Definition) []string {
	// Sort capability names for stable ordering
	capNames := make([]string, 0, len(def.Capabilities))
	for name := range def.Capabilities {
		capNames = append(capNames, name)
	}
	sort.Strings(capNames)

	opts := make([]ui.SelectOption, len(capNames))
	for i, name := range capNames {
		cap := def.Capabilities[name]
		opts[i] = ui.SelectOption{
			Label: fmt.Sprintf("%s  %s", name, ui.Dim(cap.Description)),
			Value: name,
		}
	}

	// Pre-select all capabilities by default
	preselected := make([]string, len(capNames))
	copy(preselected, capNames)

	selected, err := ui.MultiSelect("Select capabilities to grant", opts, preselected)
	if err != nil {
		return nil
	}

	// Convert to capability:scope format (default to wildcard scope)
	grants := make([]string, len(selected))
	for i, cap := range selected {
		grants[i] = cap + ":*"
	}
	return grants
}

func selectFromActions(def *integration.Definition) []string {
	actionNames := def.ActionNames()
	opts := make([]ui.SelectOption, len(actionNames))
	for i, name := range actionNames {
		action := def.Actions[name]
		opts[i] = ui.SelectOption{
			Label: fmt.Sprintf("%s  %s", name, ui.Dim(action.Description)),
			Value: name,
		}
	}

	// Pre-select all actions by default
	preselected := make([]string, len(actionNames))
	copy(preselected, actionNames)

	selected, err := ui.MultiSelect("Select actions to grant", opts, preselected)
	if err != nil {
		return nil
	}

	// Use action:* format for action-based grants
	grants := make([]string, len(selected))
	for i, action := range selected {
		grants[i] = action + ":*"
	}
	return grants
}

// mergeGrants combines existing and new grants, deduplicating by capability.
// New grants take precedence over existing ones for the same capability, so
// re-activating with a different mode correctly updates the permission.
func mergeGrants(existing, newGrants agent.IntegrationPermissions) agent.IntegrationPermissions {
	seen := make(map[string]bool)
	var merged agent.IntegrationPermissions

	// New grants first so they take precedence
	for _, g := range newGrants {
		if !seen[g.Capability] {
			seen[g.Capability] = true
			merged = append(merged, g)
		}
	}

	// Keep existing grants that aren't overridden
	for _, g := range existing {
		if !seen[g.Capability] {
			seen[g.Capability] = true
			merged = append(merged, g)
		}
	}

	return merged
}
