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

// SlackChannelResolver translates channel names (e.g. #general) to Slack channel IDs.
// It caches the mapping per session to avoid repeated API calls.
type SlackChannelResolver struct {
	mu      sync.Mutex
	cache   map[string]string // name -> ID
	token   string
	ready   bool
	baseURL string // override for testing; defaults to https://slack.com
}

// NewSlackChannelResolver creates a new resolver with the given access token.
func NewSlackChannelResolver(token string) *SlackChannelResolver {
	return &SlackChannelResolver{
		cache: make(map[string]string),
		token: token,
	}
}

// Resolve translates a channel reference to a Slack channel ID.
// If the input is already a channel ID (starts with C, D, or G), it is returned as-is.
// Channel names starting with # have the prefix stripped before lookup.
func (r *SlackChannelResolver) Resolve(channel string) (string, error) {
	// Already a channel ID
	if isSlackChannelID(channel) {
		return channel, nil
	}

	// Strip # prefix
	name := strings.TrimPrefix(channel, "#")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check cache
	if id, ok := r.cache[name]; ok {
		return id, nil
	}

	// Populate cache if not done yet
	if !r.ready {
		if err := r.populateCache(); err != nil {
			// Fallback: if the #-stripped name looks like a raw channel ID, use it directly
			if isSlackChannelID(name) {
				return name, nil
			}
			return "", fmt.Errorf("failed to resolve channel '%s': %w (hint: you can pass a raw channel ID like C01234ABCDE if name resolution is unavailable)", channel, err)
		}
		r.ready = true
	}

	id, ok := r.cache[name]
	if !ok {
		// Fallback: if the #-stripped name looks like a raw channel ID, use it directly
		if isSlackChannelID(name) {
			return name, nil
		}
		return "", fmt.Errorf("channel '%s' not found — use list_channels to see available channels", channel)
	}
	return id, nil
}

func (r *SlackChannelResolver) populateCache() error {
	client := &http.Client{Timeout: 15 * time.Second}

	base := r.baseURL
	if base == "" {
		base = "https://slack.com"
	}

	cursor := ""
	for {
		url := base + "/api/conversations.list?types=public_channel,private_channel&limit=200"
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
				ID   string `json:"id"`
				Name string `json:"name"`
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
			r.cache[ch.Name] = ch.ID
		}

		cursor = result.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}

	return nil
}

// isSlackChannelID checks if a string looks like a Slack channel/DM/group ID.
func isSlackChannelID(s string) bool {
	if len(s) < 2 {
		return false
	}
	prefix := s[0]
	return (prefix == 'C' || prefix == 'D' || prefix == 'G') && isAlphanumeric(s[1:])
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
// Slack Client IDs are numeric strings (e.g. "1234567890.9876543210").
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
// Slack Client Secrets are 32-character hex strings.
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

// CheckSlackResponse inspects a Slack API response and returns a clean error
// if the response indicates failure (ok: false).
func CheckSlackResponse(statusCode int, data interface{}) error {
	return CheckSlackResponseForAction(statusCode, data, "")
}

// CheckSlackResponseForAction inspects a Slack API response and returns
// a structured InvokeError that identifies the failure layer and suggests a fix.
func CheckSlackResponseForAction(statusCode int, data interface{}, action string) error {
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
		return ClassifySlackError(errMsg, action)
	}

	return nil
}
