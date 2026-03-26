package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type slackConversation struct {
	ID   string
	Name string
	Kind string
}

// SlackChannelResolver translates Slack conversation references to IDs and metadata.
// It caches the mapping per session to avoid repeated API calls.
type SlackChannelResolver struct {
	mu     sync.Mutex
	byName map[string]*slackConversation
	byID   map[string]*slackConversation
	token  string
	ready  bool
}

func NewSlackChannelResolver(token string) *SlackChannelResolver {
	return &SlackChannelResolver{
		byName: make(map[string]*slackConversation),
		byID:   make(map[string]*slackConversation),
		token:  token,
	}
}

// Resolve returns only the Slack conversation ID.
func (r *SlackChannelResolver) Resolve(channel string) (string, error) {
	conversation, err := r.ResolveConversation(channel)
	if err != nil {
		return "", err
	}
	return conversation.ID, nil
}

// ResolveTarget returns the canonical permission target for a Slack conversation reference.
func (r *SlackChannelResolver) ResolveTarget(channel string) (PermissionTarget, error) {
	conversation, err := r.ResolveConversation(channel)
	if err != nil {
		return PermissionTarget{}, err
	}
	target := PermissionTarget{
		Raw:      channel,
		ID:       conversation.ID,
		Kind:     conversation.Kind,
		Resolved: true,
		Exact:    "id/" + conversation.ID,
	}
	if conversation.Name != "" {
		target.Name = conversation.Name
	}
	return target, nil
}

// ResolveConversation resolves a Slack conversation by name or ID.
func (r *SlackChannelResolver) ResolveConversation(channel string) (*slackConversation, error) {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return nil, fmt.Errorf("channel cannot be empty")
	}

	name := strings.TrimPrefix(channel, "#")

	r.mu.Lock()
	defer r.mu.Unlock()

	if conversation, ok := r.byID[channel]; ok {
		return conversation, nil
	}
	if conversation, ok := r.byName[name]; ok {
		return conversation, nil
	}
	if isSlackConversationID(channel) {
		// Raw C-prefixed IDs do not encode public vs private, so when we have no
		// cached metadata we preserve only the weaker "channel" classification.
		// That allows channels/* and id/... grants, but not public/* or private/*.
		return &slackConversation{
			ID:   channel,
			Kind: inferSlackConversationKind(channel),
		}, nil
	}

	if !r.ready {
		if err := r.populateCache(); err != nil {
			return nil, fmt.Errorf("failed to resolve channel '%s': %w", channel, err)
		}
		r.ready = true
	}

	if conversation, ok := r.byID[channel]; ok {
		return conversation, nil
	}
	if conversation, ok := r.byName[name]; ok {
		return conversation, nil
	}

	return nil, fmt.Errorf("channel '%s' not found — use discover:* to list visible Slack conversations", channel)
}

func (r *SlackChannelResolver) populateCache() error {
	client := &http.Client{Timeout: 15 * time.Second}

	cursor := ""
	for {
		url := "https://slack.com/api/conversations.list?types=public_channel,private_channel,mpim,im&limit=200"
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+r.token)

		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		var result struct {
			OK       bool `json:"ok"`
			Channels []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IsPrivate bool   `json:"is_private"`
				IsIM      bool   `json:"is_im"`
				IsMPIM    bool   `json:"is_mpim"`
			} `json:"channels"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
			Error string `json:"error"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("failed to parse channel list: %w", err)
		}
		if !result.OK {
			return fmt.Errorf("Slack API error: %s", result.Error)
		}

		for _, ch := range result.Channels {
			conversation := &slackConversation{
				ID:   ch.ID,
				Name: ch.Name,
				Kind: slackConversationKind(ch.IsPrivate, ch.IsIM, ch.IsMPIM),
			}
			r.byID[ch.ID] = conversation
			if ch.Name != "" {
				r.byName[ch.Name] = conversation
			}
		}

		cursor = result.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}

	return nil
}

func slackConversationKind(isPrivate, isIM, isMPIM bool) string {
	switch {
	case isIM:
		return "dm"
	case isMPIM:
		return "mpim"
	case isPrivate:
		return "private"
	default:
		return "public"
	}
}

func inferSlackConversationKind(id string) string {
	switch {
	case strings.HasPrefix(id, "C"):
		// C IDs identify channels but do not reliably distinguish public from private.
		return "channel"
	case strings.HasPrefix(id, "D"):
		return "dm"
	case strings.HasPrefix(id, "G"):
		return "mpim"
	default:
		return ""
	}
}

func isSlackConversationID(s string) bool {
	if len(s) < 2 {
		return false
	}
	prefix := s[0]
	return (prefix == 'C' || prefix == 'D' || prefix == 'G') && isAlphanumeric(s[1:])
}

// isSlackChannelID is kept as a compatibility alias for existing tests/callers.
func isSlackChannelID(s string) bool {
	return isSlackConversationID(s)
}

func isAlphanumeric(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}

// ValidateSlackClientID checks that a Slack Client ID looks valid.
func ValidateSlackClientID(id string) error {
	if strings.HasPrefix(id, "xoxb-") || strings.HasPrefix(id, "xoxp-") {
		return fmt.Errorf("this looks like a Slack token, not a Client ID — find the Client ID under Basic Information in your Slack app settings")
	}
	if len(id) < 8 {
		return fmt.Errorf("Slack Client ID should be numeric (e.g. 1234567890.9876543210) — find it under Basic Information in your Slack app settings")
	}
	hasDigit := false
	for _, c := range id {
		if c >= '0' && c <= '9' {
			hasDigit = true
		} else if c != '.' {
			return fmt.Errorf("Slack Client ID should be numeric (e.g. 1234567890.9876543210) — find it under Basic Information in your Slack app settings")
		}
	}
	if !hasDigit {
		return fmt.Errorf("Slack Client ID should be numeric (e.g. 1234567890.9876543210) — find it under Basic Information in your Slack app settings")
	}
	return nil
}

// ValidateSlackClientSecret checks that a Slack Client Secret looks valid.
func ValidateSlackClientSecret(secret string) error {
	if strings.HasPrefix(secret, "xoxb-") || strings.HasPrefix(secret, "xoxp-") {
		return fmt.Errorf("this looks like a Slack token, not a Client Secret — find the Client Secret under Basic Information in your Slack app settings")
	}
	if len(secret) < 20 || len(secret) > 64 {
		return fmt.Errorf("Slack Client Secret should be ~32 character hex string — find it under Basic Information in your Slack app settings")
	}
	for _, c := range secret {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("Slack Client Secret should be a hex string — find it under Basic Information in your Slack app settings")
		}
	}
	return nil
}

// CheckSlackResponse inspects a Slack API response and returns a clean error if ok is false.
func CheckSlackResponse(statusCode int, data interface{}) error {
	m, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	okField, exists := m["ok"]
	if !exists {
		return nil
	}

	if okBool, isBool := okField.(bool); isBool && !okBool {
		errMsg := "unknown_error"
		if e, ok := m["error"].(string); ok {
			errMsg = e
		}
		return fmt.Errorf("slack API error: %s", errMsg)
	}

	return nil
}
