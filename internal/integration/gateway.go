package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RateLimiter tracks per-session per-action call counts.
type RateLimiter struct {
	mu       sync.Mutex
	counters map[string]*rateBucket
}

type rateBucket struct {
	count    int
	resetAt  time.Time
}

// NewRateLimiter creates a new in-memory rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		counters: make(map[string]*rateBucket),
	}
}

// Allow checks if an action is within rate limits. Returns true if allowed.
func (rl *RateLimiter) Allow(sessionID, actionKey string, limit *RateLimit) bool {
	if limit == nil || limit.Max <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	key := sessionID + ":" + actionKey
	bucket, ok := rl.counters[key]
	now := time.Now()

	if !ok || now.After(bucket.resetAt) {
		rl.counters[key] = &rateBucket{
			count:   1,
			resetAt: now.Add(limit.Window),
		}
		return true
	}

	if bucket.count >= limit.Max {
		return false
	}

	bucket.count++
	return true
}

// InvokeRequest contains everything needed to make a gateway call.
type InvokeRequest struct {
	SessionID   string
	Integration string
	Action      string
	Params      map[string]string
	Credential  *Credential
	Definition  *Definition
}

// InvokeResponse contains the filtered response from the gateway.
type InvokeResponse struct {
	StatusCode int                    `json:"status_code"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Error      string                 `json:"error,omitempty"`
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

	// Parse response
	var rawResponse map[string]interface{}
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		// Non-JSON response
		return &InvokeResponse{
			StatusCode: resp.StatusCode,
			Data:       map[string]interface{}{"raw": string(body)},
		}, nil
	}

	// Filter through field whitelist
	filtered := filterResponse(rawResponse, actionDef.Returns)

	return &InvokeResponse{
		StatusCode: resp.StatusCode,
		Data:       filtered,
	}, nil
}

func buildHTTPRequest(action *Action, cred *Credential, params map[string]string) (*http.Request, error) {
	endpoint := action.Endpoint

	// Replace template variables in endpoint
	for k, v := range params {
		endpoint = strings.ReplaceAll(endpoint, "{{"+k+"}}", v)
	}

	var bodyReader io.Reader
	switch action.BodyFormat {
	case "json":
		bodyMap := make(map[string]interface{})
		for k, v := range params {
			bodyMap[k] = v
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
			q.Set(k, v)
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
