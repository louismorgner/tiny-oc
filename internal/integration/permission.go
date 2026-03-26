package integration

import (
	"fmt"
	"strings"

	"github.com/tiny-oc/toc/internal/agent"
)

// Permission represents a parsed resource.action:scope permission entry.
type Permission struct {
	Resource string   // e.g. "issues", "*"
	Action   string   // e.g. "read", "*"
	Scopes   []string // e.g. ["*"], ["louismorgner/tiny-oc", "other/repo"]
}

// ParsePermission parses a permission string in resource.action:scope format.
// Examples:
//   - "issues.read:*"
//   - "issues.write:louismorgner/tiny-oc"
//   - "pulls.read:*"
//   - "issues.*:*"
//   - "send_message:*"        (no dot — action only, resource is empty)
//   - "send_message:dm"
//   - "read_messages:#eng,#ops"
func ParsePermission(s string) (*Permission, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty permission string")
	}

	// Split on colon to get resource.action and scope
	colonIdx := strings.LastIndex(s, ":")
	if colonIdx < 0 {
		return nil, fmt.Errorf("invalid permission format '%s': missing ':' separator (expected resource.action:scope)", s)
	}

	resourceAction := s[:colonIdx]
	scopeStr := s[colonIdx+1:]

	if resourceAction == "" {
		return nil, fmt.Errorf("invalid permission format '%s': empty resource.action", s)
	}
	if scopeStr == "" {
		return nil, fmt.Errorf("invalid permission format '%s': empty scope", s)
	}

	// Parse scopes (comma-separated)
	scopes := strings.Split(scopeStr, ",")
	for i := range scopes {
		scopes[i] = strings.TrimSpace(scopes[i])
		if scopes[i] == "" {
			return nil, fmt.Errorf("invalid permission format '%s': empty scope in list", s)
		}
	}

	// Split resource.action on dot
	var resource, action string
	dotIdx := strings.LastIndex(resourceAction, ".")
	if dotIdx < 0 {
		// No dot — treat as action-only (e.g. "send_message:*")
		resource = ""
		action = resourceAction
	} else {
		resource = resourceAction[:dotIdx]
		action = resourceAction[dotIdx+1:]
	}

	if action == "" {
		return nil, fmt.Errorf("invalid permission format '%s': empty action", s)
	}

	return &Permission{
		Resource: resource,
		Action:   action,
		Scopes:   scopes,
	}, nil
}

// ParsePermissions parses a list of permission strings.
func ParsePermissions(perms []string) ([]*Permission, error) {
	result := make([]*Permission, 0, len(perms))
	for _, s := range perms {
		p, err := ParsePermission(s)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, nil
}

// CheckPermission checks if a specific action with a target scope is allowed
// by the given permission list. Returns true if allowed, false if denied.
// Default deny — no matching permission means no access.
func CheckPermission(permissions []*Permission, actionName string, target string) bool {
	// Split the action name into resource and action parts
	var targetResource, targetAction string
	if dotIdx := strings.LastIndex(actionName, "."); dotIdx >= 0 {
		targetResource = actionName[:dotIdx]
		targetAction = actionName[dotIdx+1:]
	} else {
		targetResource = ""
		targetAction = actionName
	}

	for _, perm := range permissions {
		if matchesAction(perm, targetResource, targetAction) && matchesScope(perm, target) {
			return true
		}
	}
	return false
}

// matchesAction checks if a permission entry matches the requested resource.action.
func matchesAction(perm *Permission, resource, action string) bool {
	// Resource matching must be explicit:
	// - perm.Resource == "*" matches any resource (including empty)
	// - perm.Resource == "" only matches actions with no resource component
	// - perm.Resource == "foo" only matches resource "foo"
	if perm.Resource == "*" {
		// wildcard resource — matches anything
	} else if perm.Resource != resource {
		return false
	}

	// Check action match
	if perm.Action != "*" && perm.Action != action {
		return false
	}

	return true
}

// matchesScope checks if any of the permission's scopes cover the target.
func matchesScope(perm *Permission, target string) bool {
	for _, scope := range perm.Scopes {
		if scope == "*" {
			return true
		}
		if scope == target {
			return true
		}
	}
	return false
}

// ValidatePermissionsAgainstDefinition checks that all permissions reference
// valid actions and scopes from the integration definition.
func ValidatePermissionsAgainstDefinition(perms []string, def *Definition) error {
	parsed, err := ParsePermissions(perms)
	if err != nil {
		return err
	}

	for _, p := range parsed {
		// Build the full action name for lookup
		actionName := p.Action
		if p.Resource != "" {
			actionName = p.Resource + "." + p.Action
		}

		// Wildcard action — check that at least one action exists with the resource prefix
		if p.Action == "*" {
			if p.Resource == "" || p.Resource == "*" {
				continue // full wildcard — covers everything
			}
			found := false
			for name := range def.Actions {
				if strings.HasPrefix(name, p.Resource+".") || name == p.Resource {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("no actions matching resource '%s' in integration '%s'", p.Resource, def.Name)
			}
			continue
		}

		// Check exact action exists
		if _, ok := def.Actions[actionName]; !ok {
			// Also try without resource prefix for action-only permissions
			if p.Resource == "" {
				if _, ok := def.Actions[p.Action]; !ok {
					return fmt.Errorf("unknown action '%s' in integration '%s'", actionName, def.Name)
				}
			} else {
				return fmt.Errorf("unknown action '%s' in integration '%s'", actionName, def.Name)
			}
		}
	}

	return nil
}

// PermissionManifest is the resolved permission set written at spawn time.
type PermissionManifest struct {
	SessionID    string                           `json:"session_id"`
	Agent        string                           `json:"agent"`
	Filesystem   agent.FilesystemPermissions      `json:"filesystem,omitempty"`
	Integrations map[string][]string              `json:"integrations"`
	SubAgents    map[string]agent.PermissionLevel `json:"sub_agents,omitempty"`
}
