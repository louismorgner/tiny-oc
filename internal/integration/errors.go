package integration

import "fmt"

// InvokeErrorKind distinguishes the layer at which an invoke call failed.
type InvokeErrorKind int

const (
	ErrPermissionDenied InvokeErrorKind = iota
	ErrActionNotFound
	ErrMissingOAuthScope
	ErrCredentialError
	ErrAPIError
)

// InvokeError is a structured error returned by the invoke pipeline.
// It identifies which layer failed and provides an actionable fix message.
type InvokeError struct {
	Kind        InvokeErrorKind
	Integration string
	Action      string
	Agent       string
	Message     string
	Fix         string
}

func (e *InvokeError) Error() string {
	if e.Fix != "" {
		return fmt.Sprintf("%s\n  Fix: %s", e.Message, e.Fix)
	}
	return e.Message
}

// NewPermissionDeniedError creates an error for when the agent lacks permission.
func NewPermissionDeniedError(agent, integration, action string) *InvokeError {
	return &InvokeError{
		Kind:        ErrPermissionDenied,
		Integration: integration,
		Action:      action,
		Agent:       agent,
		Message:     fmt.Sprintf("Your agent '%s' does not have permission for %s.%s", agent, integration, action),
		Fix:         fmt.Sprintf("Add it to oc-agent.yaml under permissions.integrations.%s", integration),
	}
}

// NewNoIntegrationPermError creates an error for when the agent has no permissions for the integration at all.
func NewNoIntegrationPermError(agent, integration string) *InvokeError {
	return &InvokeError{
		Kind:        ErrPermissionDenied,
		Integration: integration,
		Agent:       agent,
		Message:     fmt.Sprintf("Your agent '%s' has no permissions for the %s integration", agent, integration),
		Fix:         fmt.Sprintf("Add it to oc-agent.yaml under permissions.integrations.%s", integration),
	}
}

// NewActionNotFoundError creates an error for when the action doesn't exist on the integration.
func NewActionNotFoundError(integration, action string, available []string) *InvokeError {
	msg := fmt.Sprintf("The %s integration does not have a '%s' action", integration, action)
	fix := fmt.Sprintf("Run toc runtime invoke %s --help to see available actions", integration)
	if len(available) > 0 {
		msg += fmt.Sprintf(". Available actions: %s", joinActions(available))
	}
	return &InvokeError{
		Kind:        ErrActionNotFound,
		Integration: integration,
		Action:      action,
		Message:     msg,
		Fix:         fix,
	}
}

// NewMissingScopeError creates an error for when the OAuth token is missing a required scope.
func NewMissingScopeError(integration, scope string) *InvokeError {
	return &InvokeError{
		Kind:        ErrMissingOAuthScope,
		Integration: integration,
		Message:     fmt.Sprintf("The %s OAuth token is missing scope '%s'", integration, scope),
		Fix:         fmt.Sprintf("Re-run toc integrate add %s to update scopes", integration),
	}
}

// NewCredentialError creates an error for credential loading failures.
func NewCredentialError(integration string, underlying error) *InvokeError {
	return &InvokeError{
		Kind:        ErrCredentialError,
		Integration: integration,
		Message:     fmt.Sprintf("Failed to load credentials for '%s': %v", integration, underlying),
		Fix:         fmt.Sprintf("Run toc integrate add %s to configure credentials", integration),
	}
}

func joinActions(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	result := actions[0]
	for _, a := range actions[1:] {
		result += ", " + a
	}
	return result
}

// slackScopeMap maps Slack API error strings to the OAuth scope needed.
var slackScopeMap = map[string]string{
	"missing_scope":       "",
	"channel_not_found":   "channels:read",
	"not_in_channel":      "channels:read",
	"not_authed":          "",
	"invalid_auth":        "",
	"account_inactive":    "",
	"token_revoked":       "",
	"no_permission":       "",
	"ekm_access_denied":   "",
	"is_archived":         "",
	"channel_not_joined":  "channels:read",
}

// Slack action to required scope mapping.
var slackActionScopeMap = map[string]string{
	"send_message":    "chat:write",
	"read_messages":   "channels:history",
	"search_messages": "search:read",
	"list_channels":   "channels:read",
	"react":           "reactions:write",
}

// ClassifySlackError takes a Slack API error string and the action that was
// attempted, and returns a structured InvokeError with actionable guidance.
func ClassifySlackError(slackErr string, action string) *InvokeError {
	switch slackErr {
	case "missing_scope":
		scope := slackActionScopeMap[action]
		if scope == "" {
			scope = "(unknown)"
		}
		return NewMissingScopeError("slack", scope)

	case "not_authed", "invalid_auth", "token_revoked", "account_inactive":
		return &InvokeError{
			Kind:        ErrCredentialError,
			Integration: "slack",
			Action:      action,
			Message:     fmt.Sprintf("Slack authentication failed: %s", slackErr),
			Fix:         "Re-run toc integrate add slack to refresh your credentials",
		}

	case "channel_not_found", "channel_not_joined", "not_in_channel", "is_archived":
		return &InvokeError{
			Kind:        ErrAPIError,
			Integration: "slack",
			Action:      action,
			Message:     fmt.Sprintf("Slack API error: %s", slackErr),
			Fix:         "Verify the channel exists and your account has access. Use toc runtime invoke slack list_channels to see available channels",
		}

	case "no_permission", "ekm_access_denied":
		scope := slackActionScopeMap[action]
		if scope != "" {
			return NewMissingScopeError("slack", scope)
		}
		return &InvokeError{
			Kind:        ErrMissingOAuthScope,
			Integration: "slack",
			Action:      action,
			Message:     fmt.Sprintf("Slack permission denied: %s", slackErr),
			Fix:         "Re-run toc integrate add slack to update scopes",
		}

	default:
		return &InvokeError{
			Kind:        ErrAPIError,
			Integration: "slack",
			Action:      action,
			Message:     fmt.Sprintf("Slack API error: %s", slackErr),
		}
	}
}
