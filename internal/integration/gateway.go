package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// rateBucketJSON is the on-disk representation of a rate limit bucket.
type rateBucketJSON struct {
	Count   int       `json:"count"`
	ResetAt time.Time `json:"reset_at"`
}

// RateLimiter tracks per-session per-action call counts, persisted to disk
// so that state survives across separate process invocations.
type RateLimiter struct {
	mu   sync.Mutex
	path string // path to rate_limits.json
}

// NewRateLimiter creates a file-backed rate limiter.
// path should be e.g. .toc/sessions/<id>/rate_limits.json.
func NewRateLimiter(path string) *RateLimiter {
	return &RateLimiter{path: path}
}

// Allow checks if an action is within rate limits. Returns true if allowed.
// Loads state from disk, checks/increments, and writes back atomically.
func (rl *RateLimiter) Allow(sessionID, actionKey string, limit *RateLimit) bool {
	if limit == nil || limit.Max <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	counters := rl.load()

	key := sessionID + ":" + actionKey
	bucket, ok := counters[key]
	now := time.Now()

	if !ok || now.After(bucket.ResetAt) {
		counters[key] = &rateBucketJSON{
			Count:   1,
			ResetAt: now.Add(limit.Window),
		}
		rl.save(counters)
		return true
	}

	if bucket.Count >= limit.Max {
		return false
	}

	bucket.Count++
	rl.save(counters)
	return true
}

func (rl *RateLimiter) load() map[string]*rateBucketJSON {
	counters := make(map[string]*rateBucketJSON)
	if rl.path == "" {
		return counters
	}
	data, err := os.ReadFile(rl.path)
	if err != nil {
		return counters
	}
	_ = json.Unmarshal(data, &counters)
	return counters
}

func (rl *RateLimiter) save(counters map[string]*rateBucketJSON) {
	if rl.path == "" {
		return
	}
	data, err := json.Marshal(counters)
	if err != nil {
		return
	}
	_ = os.WriteFile(rl.path, data, 0600)
}

// InvokeRequest contains everything needed to make a gateway call.
type InvokeRequest struct {
	SessionID   string
	Integration string
	Action      string
	Params      map[string]string
	Credential  *Credential
	Definition  *Definition

	// ChannelResolver is set for Slack integrations to translate channel names to IDs.
	ChannelResolver *SlackChannelResolver
	// Workspace is set when token refresh is possible, to allow credential updates.
	Workspace string
}

// InvokeResponse contains the filtered response from the gateway.
type InvokeResponse struct {
	StatusCode int         `json:"status_code"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// Invoke executes an API call through the generic HTTP adapter.
func Invoke(req *InvokeRequest) (*InvokeResponse, error) {
	actionDef, err := req.Definition.GetAction(req.Action)
	if err != nil {
		return nil, err
	}

	// Validate required params
	for _, param := range actionDef.Params {
		if param.Required {
			if _, ok := req.Params[param.Name]; !ok {
				// Check for default
				if param.Default != "" {
					req.Params[param.Name] = param.Default
				} else {
					return nil, fmt.Errorf("missing required parameter: %s", param.Name)
				}
			}
		}
	}

	// Apply defaults for optional params
	for _, param := range actionDef.Params {
		if !param.Required && param.Default != "" {
			if _, ok := req.Params[param.Name]; !ok {
				req.Params[param.Name] = param.Default
			}
		}
	}

	// Slack: resolve channel names to IDs
	if req.Integration == "slack" && req.ChannelResolver != nil {
		if ch, ok := req.Params["channel"]; ok {
			resolved, err := req.ChannelResolver.Resolve(ch)
			if err != nil {
				return nil, err
			}
			req.Params["channel"] = resolved
		}
	}

	// OAuth2: check token expiry and refresh if needed
	if req.Credential.IsExpired(30*time.Second) && req.Credential.RefreshToken != "" && req.Workspace != "" {
		refreshed, err := refreshCredentialForIntegration(req.Integration, req.Credential, req.Workspace)
		if err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
		req.Credential = refreshed
	}

	// Build HTTP request
	httpReq, err := buildHTTPRequest(actionDef, req.Credential, req.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP request: %w", err)
	}

	// Execute
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response — could be a JSON object or a JSON array
	var rawResponse interface{}
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		// Non-JSON response
		return &InvokeResponse{
			StatusCode: resp.StatusCode,
			Data:       map[string]interface{}{"raw": string(body)},
		}, nil
	}

	// Slack: check for ok:false error responses before filtering
	if req.Integration == "slack" {
		if err := CheckSlackResponse(resp.StatusCode, rawResponse); err != nil {
			return &InvokeResponse{
				StatusCode: resp.StatusCode,
				Error:      err.Error(),
			}, nil
		}
	}

	// Filter through field whitelist
	filtered := filterAnyResponse(rawResponse, actionDef.Returns)

	return &InvokeResponse{
		StatusCode: resp.StatusCode,
		Data:       filtered,
	}, nil
}

// refreshCredentialForIntegration refreshes the OAuth2 token for the given integration.
func refreshCredentialForIntegration(integrationName string, cred *Credential, workspace string) (*Credential, error) {
	clientCfg, err := LoadOAuth2ClientConfigFromWorkspace(workspace, integrationName)
	if err != nil {
		return nil, fmt.Errorf("no OAuth2 client config stored for '%s': %w", integrationName, err)
	}

	// Determine token URL based on integration
	var tokenURL string
	switch integrationName {
	case "slack":
		tokenURL = "https://slack.com/api/oauth.v2.access"
	default:
		return nil, fmt.Errorf("token refresh not supported for integration '%s'", integrationName)
	}

	refreshed, err := RefreshAccessToken(tokenURL, clientCfg.ClientID, clientCfg.ClientSecret, cred.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Persist the refreshed credential — non-fatal if this fails since the
	// token still works for the current request.
	if err := StoreCredentialInWorkspace(workspace, integrationName, refreshed); err != nil {
		log.Printf("warning: failed to persist refreshed credential for '%s': %v", integrationName, err)
	}

	return refreshed, nil
}

func buildHTTPRequest(action *Action, cred *Credential, params map[string]string) (*http.Request, error) {
	endpoint := action.Endpoint

	// Replace template variables in endpoint and track which params were used
	templateParams := make(map[string]bool)
	for k, v := range params {
		placeholder := "{{" + k + "}}"
		if strings.Contains(endpoint, placeholder) {
			endpoint = strings.ReplaceAll(endpoint, placeholder, v)
			templateParams[k] = true
		}
	}

	var bodyReader io.Reader
	switch action.BodyFormat {
	case "json":
		bodyMap := make(map[string]interface{})
		for k, v := range params {
			if !templateParams[k] {
				bodyMap[k] = v
			}
		}
		data, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)

	case "query", "":
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, v := range params {
			if !templateParams[k] {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	req, err := http.NewRequest(action.Method, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}

	// Set auth header
	authHeader := strings.ReplaceAll(action.AuthHeader, "{{token}}", cred.AccessToken)
	if strings.HasPrefix(authHeader, "Bearer ") || strings.HasPrefix(authHeader, "token ") {
		req.Header.Set("Authorization", authHeader)
	} else {
		req.Header.Set("Authorization", authHeader)
	}

	if action.BodyFormat == "json" {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("User-Agent", "toc/1.0")

	return req, nil
}

// filterAnyResponse filters either a map or array response through the whitelist.
// If no whitelist is defined, returns the raw response unchanged.
func filterAnyResponse(raw interface{}, whitelist []string) interface{} {
	if len(whitelist) == 0 {
		return raw
	}

	switch v := raw.(type) {
	case map[string]interface{}:
		return filterResponse(v, whitelist)
	case []interface{}:
		return filterArrayResponse(v, whitelist)
	default:
		return raw
	}
}

// filterArrayResponse handles top-level JSON array responses with [].field notation.
// Returns a slice of maps containing only the whitelisted fields from each element.
func filterArrayResponse(arr []interface{}, whitelist []string) []interface{} {
	// Strip leading "[]." prefix from whitelist entries
	fields := make([]string, 0, len(whitelist))
	for _, w := range whitelist {
		if strings.HasPrefix(w, "[].") {
			fields = append(fields, w[3:])
		} else {
			fields = append(fields, w)
		}
	}

	result := make([]interface{}, 0, len(arr))
	for _, elem := range arr {
		m, ok := elem.(map[string]interface{})
		if !ok {
			continue
		}
		filtered := make(map[string]interface{})
		for _, field := range fields {
			val := extractField(m, field)
			if val != nil {
				setField(filtered, field, val)
			}
		}
		if len(filtered) > 0 {
			result = append(result, filtered)
		}
	}
	return result
}

// filterResponse returns only the whitelisted fields from the raw response.
// If no whitelist is defined, returns the full response.
func filterResponse(raw map[string]interface{}, whitelist []string) map[string]interface{} {
	if len(whitelist) == 0 {
		return raw
	}

	filtered := make(map[string]interface{})
	for _, field := range whitelist {
		val := extractField(raw, field)
		if val != nil {
			setField(filtered, field, val)
		}
	}
	return filtered
}

// extractField extracts a value from a nested map using dot notation.
// Supports array notation like "messages[].text".
func extractField(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		// Handle array accessor like "messages[]"
		if strings.HasSuffix(part, "[]") {
			arrayKey := strings.TrimSuffix(part, "[]")
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			arr, ok := m[arrayKey].([]interface{})
			if !ok {
				return nil
			}
			current = arr
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil
			}
			current = val
		case []interface{}:
			// Extract field from each element in the array
			var results []interface{}
			for _, elem := range v {
				if m, ok := elem.(map[string]interface{}); ok {
					if val, ok := m[part]; ok {
						results = append(results, val)
					}
				}
			}
			if len(results) == 0 {
				return nil
			}
			current = results
		default:
			return nil
		}
	}

	return current
}

// setField sets a value in a nested map structure.
func setField(data map[string]interface{}, path string, value interface{}) {
	// Flatten array notation for output
	cleanPath := strings.ReplaceAll(path, "[]", "")
	parts := strings.Split(cleanPath, ".")

	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[part] = next
		}
		current = next
	}
}
