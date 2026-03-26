package integration

import (
	"fmt"
	"strings"

	"github.com/tiny-oc/toc/internal/agent"
)

// Permission represents a parsed resource.action:scope permission entry.
type Permission struct {
	Resource string   // e.g. "issues", "*"
	Action   string   // e.g. "read", "*", "post"
	Scopes   []string // e.g. ["*"], ["#eng", "#ops"], ["channels/*"]
}

// PermissionTarget is the resolved target that permission checks operate on.
type PermissionTarget struct {
	Raw      string
	Exact    string // canonical exact selector, usually an immutable ID
	Name     string // human-readable name, without leading '#'
	ID       string // provider ID, if known
	Kind     string // e.g. public, private, dm, mpim
	Resolved bool
}

// PermissionDecision captures the result of permission evaluation.
type PermissionDecision struct {
	Level        agent.PermissionLevel
	Grant        agent.IntegrationPermissionGrant
	Subject      string
	MatchedScope string
}

func (t PermissionTarget) Display() string {
	switch {
	case t.Raw != "":
		return t.Raw
	case t.Exact != "":
		return t.Exact
	default:
		return "*"
	}
}

func (t PermissionTarget) ExactPermissionScope() string {
	switch {
	case t.ID != "":
		return "id/" + t.ID
	case t.Exact != "":
		return t.Exact
	case t.Raw != "":
		return t.Raw
	default:
		return "*"
	}
}

// ParsePermission parses a permission string in resource.action:scope format.
// Examples:
//   - "issues.read:*"
//   - "issues.write:louismorgner/tiny-oc"
//   - "send_message:*"
//   - "read_messages:#eng,#ops"
//   - "post:channels/*"
//   - "*"
func ParsePermission(s string) (*Permission, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty permission string")
	}
	if s == "*" {
		return &Permission{
			Resource: "",
			Action:   "*",
			Scopes:   []string{"*"},
		}, nil
	}

	colonIdx := strings.LastIndex(s, ":")
	if colonIdx < 0 {
		return nil, fmt.Errorf("invalid permission format '%s': missing ':' separator (expected resource.action:scope)", s)
	}

	resourceAction := strings.TrimSpace(s[:colonIdx])
	scopeStr := strings.TrimSpace(s[colonIdx+1:])
	if resourceAction == "" {
		return nil, fmt.Errorf("invalid permission format '%s': empty resource.action", s)
	}
	if scopeStr == "" {
		return nil, fmt.Errorf("invalid permission format '%s': empty scope", s)
	}

	scopes := strings.Split(scopeStr, ",")
	for i := range scopes {
		scopes[i] = strings.TrimSpace(scopes[i])
		if scopes[i] == "" {
			return nil, fmt.Errorf("invalid permission format '%s': empty scope in list", s)
		}
	}

	var resource, action string
	if dotIdx := strings.LastIndex(resourceAction, "."); dotIdx >= 0 {
		resource = resourceAction[:dotIdx]
		action = resourceAction[dotIdx+1:]
	} else {
		action = resourceAction
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

// ParsePermissions parses a list of raw permission strings.
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

// EvaluatePermission resolves the effective permission level for the given action and target.
func EvaluatePermission(def *Definition, grants agent.IntegrationPermissions, actionName string, target PermissionTarget) PermissionDecision {
	best := PermissionDecision{Level: agent.PermOff}
	bestScore := -1

	for _, grant := range grants {
		perm, err := ParsePermission(grant.Capability)
		if err != nil {
			continue
		}

		subject := permissionSubject(perm)
		actionScore := actionSpecificity(def, perm, actionName)
		if actionScore < 0 {
			continue
		}

		scopeScore, matchedScope := scopeSpecificity(def, perm, target)
		if scopeScore < 0 {
			continue
		}

		score := actionScore + scopeScore
		if score > bestScore {
			level := grant.Mode
			if level == "" {
				level = agent.PermOn
			}
			best = PermissionDecision{
				Level:        level,
				Grant:        grant,
				Subject:      subject,
				MatchedScope: matchedScope,
			}
			bestScore = score
		}
	}

	return best
}

// CheckPermission preserves the legacy bool API for existing callers/tests.
func CheckPermission(permissions []*Permission, actionName string, target string) bool {
	def := &Definition{Actions: map[string]Action{}}
	for _, perm := range permissions {
		subject := permissionSubject(perm)
		if subject != "*" {
			def.Actions[subject] = Action{}
		}
	}
	decision := EvaluatePermission(def, rawPermissionsToGrants(permissions), actionName, PermissionTarget{Raw: target, Exact: target})
	return decision.Level != agent.PermOff
}

// ValidatePermissionsAgainstDefinition checks that all permissions reference
// valid actions/capabilities and sane target syntax for the given integration.
func ValidatePermissionsAgainstDefinition(grants agent.IntegrationPermissions, def *Definition) error {
	for _, grant := range grants {
		perm, err := ParsePermission(grant.Capability)
		if err != nil {
			return err
		}

		if err := validatePermissionSubject(perm, def); err != nil {
			return err
		}
		if err := validatePermissionScopes(perm, def); err != nil {
			return err
		}
	}
	return nil
}

func rawPermissionsToGrants(perms []*Permission) agent.IntegrationPermissions {
	grants := make(agent.IntegrationPermissions, 0, len(perms))
	for _, perm := range perms {
		scope := strings.Join(perm.Scopes, ",")
		subject := permissionSubject(perm)
		capability := subject
		if scope != "*" || subject != "*" {
			capability = subject + ":" + scope
		}
		grants = append(grants, agent.IntegrationPermissionGrant{
			Mode:       agent.PermOn,
			Capability: capability,
		})
	}
	return grants
}

func permissionSubject(perm *Permission) string {
	if perm.Resource == "" {
		return perm.Action
	}
	return perm.Resource + "." + perm.Action
}

func actionSpecificity(def *Definition, perm *Permission, actionName string) int {
	if perm.Resource == "" {
		if perm.Action == "*" {
			return 1
		}
		if _, ok := def.Actions[perm.Action]; ok && perm.Action == actionName {
			return 50
		}
		if capability, ok := def.Capabilities[perm.Action]; ok {
			for _, action := range capability.Actions {
				if action == actionName {
					return 40
				}
			}
			return -1
		}
		if perm.Action == actionName {
			return 50
		}
		return -1
	}

	targetResource, targetAction := splitActionName(actionName)
	if !matchesAction(perm, targetResource, targetAction) {
		return -1
	}

	score := 10
	if perm.Resource != "*" {
		score += 20
	}
	if perm.Action != "*" {
		score += 10
	}
	return score
}

func splitActionName(actionName string) (string, string) {
	if dotIdx := strings.LastIndex(actionName, "."); dotIdx >= 0 {
		return actionName[:dotIdx], actionName[dotIdx+1:]
	}
	return "", actionName
}

func matchesAction(perm *Permission, resource, action string) bool {
	if perm.Resource == "*" {
	} else if perm.Resource != resource {
		return false
	}
	if perm.Action != "*" && perm.Action != action {
		return false
	}
	return true
}

func scopeSpecificity(def *Definition, perm *Permission, target PermissionTarget) (int, string) {
	bestScore := -1
	bestScope := ""
	for _, scope := range perm.Scopes {
		var score int
		if def != nil && def.Name == "slack" {
			score = slackScopeSpecificity(scope, target)
		} else {
			score = genericScopeSpecificity(scope, target)
		}
		if score > bestScore {
			bestScore = score
			bestScope = scope
		}
	}
	return bestScore, bestScope
}

func genericScopeSpecificity(scope string, target PermissionTarget) int {
	switch {
	case scope == "*":
		return 1
	case scope == target.Exact && target.Exact != "":
		return 50
	case scope == target.Raw:
		return 50
	default:
		return -1
	}
}

func slackScopeSpecificity(scope string, target PermissionTarget) int {
	scope = strings.TrimSpace(scope)
	switch {
	case scope == "*":
		return 1
	case strings.HasPrefix(scope, "id/"):
		if target.ID != "" && scope == "id/"+target.ID {
			return 120
		}
		return -1
	case isSlackClassScope(scope):
		return slackClassScopeSpecificity(scope, target.Kind)
	case strings.HasPrefix(scope, "#"):
		if target.Name != "" && scope == "#"+target.Name {
			return 100
		}
		return -1
	case isSlackConversationID(scope):
		if target.ID != "" && scope == target.ID {
			return 110
		}
		if target.Raw == scope {
			return 110
		}
		return -1
	case scope == "dm":
		if target.Kind == "dm" {
			return 60
		}
		return -1
	case scope == "channels":
		if target.Kind == "public" || target.Kind == "private" {
			return 40
		}
		return -1
	case target.Name != "" && scope == target.Name:
		return 95
	case scope == target.Raw:
		return 80
	default:
		return -1
	}
}

func slackClassScopeSpecificity(scope, kind string) int {
	switch scope {
	case "public/*":
		if kind == "public" {
			return 70
		}
	case "private/*":
		if kind == "private" {
			return 70
		}
	case "channels/*":
		if kind == "public" || kind == "private" || kind == "channel" {
			return 65
		}
	case "dm/*":
		if kind == "dm" {
			return 65
		}
	case "mpim/*":
		if kind == "mpim" {
			return 65
		}
	case "conversations/*":
		if kind != "" {
			return 20
		}
	}
	return -1
}

func validatePermissionSubject(perm *Permission, def *Definition) error {
	subject := permissionSubject(perm)
	if subject == "*" {
		return nil
	}

	if perm.Resource == "" {
		if _, ok := def.Actions[perm.Action]; ok {
			return nil
		}
		if _, ok := def.Capabilities[perm.Action]; ok {
			return nil
		}
		return fmt.Errorf("unknown action or capability '%s' in integration '%s'", subject, def.Name)
	}

	if perm.Action == "*" {
		for name := range def.Actions {
			if strings.HasPrefix(name, perm.Resource+".") {
				return nil
			}
		}
		return fmt.Errorf("no actions matching resource '%s' in integration '%s'", perm.Resource, def.Name)
	}

	if _, ok := def.Actions[subject]; ok {
		return nil
	}
	return fmt.Errorf("unknown action '%s' in integration '%s'", subject, def.Name)
}

func validatePermissionScopes(perm *Permission, def *Definition) error {
	if def.Name != "slack" {
		return nil
	}
	for _, scope := range perm.Scopes {
		if scope == "*" ||
			strings.HasPrefix(scope, "#") ||
			strings.HasPrefix(scope, "id/") ||
			isSlackClassScope(scope) ||
			isSlackConversationID(scope) ||
			scope == "dm" ||
			scope == "channels" {
			continue
		}
		// Preserve backward compatibility for literal exact targets like "general".
		if strings.TrimSpace(scope) != "" {
			continue
		}
		return fmt.Errorf("invalid slack scope '%s'", scope)
	}
	return nil
}

func isSlackClassScope(scope string) bool {
	switch scope {
	case "public/*", "private/*", "channels/*", "dm/*", "mpim/*", "conversations/*":
		return true
	default:
		return false
	}
}

// DeterminePermissionTarget resolves the permission target for a runtime invocation.
func DeterminePermissionTarget(integrationName, actionName string, params map[string]string, resolver *SlackChannelResolver) (PermissionTarget, error) {
	switch integrationName {
	case "github":
		if repo, ok := params["repo"]; ok {
			return PermissionTarget{Raw: repo, Exact: repo}, nil
		}
	case "slack":
		switch actionName {
		case "send_message", "read_messages", "react":
			channel, ok := params["channel"]
			if !ok {
				return PermissionTarget{Raw: "*", Exact: "*"}, nil
			}
			if resolver == nil {
				return PermissionTarget{Raw: channel, Exact: channel}, nil
			}
			return resolver.ResolveTarget(channel)
		case "search_messages":
			return PermissionTarget{Raw: "*", Exact: "*"}, nil
		case "list_channels":
			return slackListTargetFromParams(params), nil
		}
	case "linear":
		if team, ok := params["team"]; ok {
			target := "team/" + team
			return PermissionTarget{Raw: target, Exact: target}, nil
		}
	}

	return PermissionTarget{Raw: "*", Exact: "*"}, nil
}

func slackListTargetFromParams(params map[string]string) PermissionTarget {
	types := strings.TrimSpace(params["types"])
	if types == "" {
		types = "public_channel,private_channel,im,mpim"
	}

	parts := strings.Split(types, ",")
	normalized := make(map[string]bool, len(parts))
	for _, part := range parts {
		normalized[strings.TrimSpace(part)] = true
	}

	switch {
	case len(normalized) == 1 && normalized["public_channel"]:
		return PermissionTarget{Raw: "public/*", Exact: "public/*", Kind: "public"}
	case len(normalized) == 1 && normalized["private_channel"]:
		return PermissionTarget{Raw: "private/*", Exact: "private/*", Kind: "private"}
	case len(normalized) == 1 && normalized["im"]:
		return PermissionTarget{Raw: "dm/*", Exact: "dm/*", Kind: "dm"}
	case len(normalized) == 1 && normalized["mpim"]:
		return PermissionTarget{Raw: "mpim/*", Exact: "mpim/*", Kind: "mpim"}
	case normalized["public_channel"] || normalized["private_channel"] || normalized["im"] || normalized["mpim"]:
		return PermissionTarget{Raw: "conversations/*", Exact: "conversations/*", Kind: "conversation"}
	default:
		return PermissionTarget{Raw: "*", Exact: "*"}
	}
}

// PermissionManifest is the resolved permission set written at spawn time.
type PermissionManifest struct {
	SessionID    string                                  `json:"session_id"`
	Agent        string                                  `json:"agent"`
	Filesystem   agent.FilesystemPermissions             `json:"filesystem,omitempty"`
	Integrations map[string]agent.IntegrationPermissions `json:"integrations"`
	SubAgents    map[string]agent.PermissionLevel        `json:"sub_agents,omitempty"`
}
