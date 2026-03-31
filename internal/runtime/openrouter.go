package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/config"
)

const (
	maxRetryAttempts = 3
	retryBaseDelay   = 1 * time.Second

	// HTTP status code 529 is used by some providers (e.g. Cloudflare) for
	// "Site Overloaded" — treat it as retryable alongside standard 5xx codes.
	httpStatusSiteOverloaded = 529
)

// isRetryableStatusCode returns true for HTTP status codes that indicate
// a transient server-side error worth retrying.
func isRetryableStatusCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		httpStatusSiteOverloaded:
		return true
	}
	return false
}

// streamError is a sentinel error indicating the SSE stream delivered an
// error payload instead of (or mid-way through) content. It carries enough
// detail from the OpenRouter error envelope to decide whether to retry.
type streamError struct {
	Code      int
	Message   string
	ErrorType string // from metadata.error_type
}

func (e *streamError) Error() string {
	if e.ErrorType != "" {
		return fmt.Sprintf("openrouter stream error (code=%d, type=%s): %s", e.Code, e.ErrorType, e.Message)
	}
	return fmt.Sprintf("openrouter stream error (code=%d): %s", e.Code, e.Message)
}

// isRetryableStreamError returns true when an in-stream error represents a
// transient provider-side failure that is safe to retry with the same request.
func isRetryableStreamError(err error) bool {
	var se *streamError
	if !errors.As(err, &se) {
		return false
	}
	// Retryable by HTTP-style status code embedded in the error envelope.
	if se.Code > 0 && isRetryableStatusCode(se.Code) {
		return true
	}
	// Retryable by error_type metadata.
	switch se.ErrorType {
	case "provider_unavailable", "provider_overloaded", "rate_limited":
		return true
	}
	// Retryable by message content (some providers only set message).
	lower := strings.ToLower(se.Message)
	for _, keyword := range []string{"overloaded", "temporarily unavailable", "capacity"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// isRetryableNetworkError returns true for transient network-level errors
// (connection refused, DNS failures, timeouts).
func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	return false
}

// retryDelay computes backoff delay for the given attempt (0-indexed).
// For 429 responses, it respects the Retry-After header if present.
func retryDelay(attempt int, resp *http.Response) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				return time.Duration(secs) * time.Second
			}
		}
	}
	// Exponential backoff: 1s, 2s, 4s
	return retryBaseDelay * (1 << attempt)
}

// openRouterAPIError formats a user-friendly error for an OpenRouter HTTP error.
func openRouterAPIError(statusCode int, serverMsg string, exhaustedRetries bool) error {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return fmt.Errorf("OpenRouter authentication failed (HTTP %d). Check your API key with: toc config set-key openrouter", statusCode)
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("OpenRouter rate limit exceeded (HTTP 429). Wait a moment and retry.")
	case statusCode >= 500 && statusCode < 600:
		detail := fmt.Sprintf("HTTP %d %s", statusCode, http.StatusText(statusCode))
		if serverMsg != "" {
			detail += ": " + serverMsg
		}
		if exhaustedRetries {
			return fmt.Errorf("OpenRouter server error after %d attempts (%s). Your session is saved and can be resumed.", maxRetryAttempts, detail)
		}
		return fmt.Errorf("OpenRouter server error (%s)", detail)
	default:
		if serverMsg != "" {
			return fmt.Errorf("OpenRouter request failed (HTTP %d %s): %s", statusCode, http.StatusText(statusCode), serverMsg)
		}
		return fmt.Errorf("OpenRouter request failed (HTTP %d %s)", statusCode, http.StatusText(statusCode))
	}
}

// extractOpenRouterErrorMessage tries to pull a useful error message from an
// OpenRouter error response body. Falls back to the truncated raw body so that
// debugging info is never silently swallowed.
func extractOpenRouterErrorMessage(body []byte) string {
	var parsed chatResponse
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	// Fallback: return truncated raw body for debugging.
	raw := strings.TrimSpace(string(body))
	if len(raw) > 512 {
		raw = raw[:512] + "..."
	}
	if raw != "" {
		return raw
	}
	return "(empty response body)"
}

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

func resolveNativeBaseURLFromEnv() string {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("TOC_NATIVE_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL")), "/")
	}
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}
	return baseURL
}

type openRouterClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	title      string
	referer    string
}

type cacheControl struct {
	Type string `json:"type"`
}

// reasoningConfig enables extended thinking/reasoning for models that support it.
// Use MaxTokens for Anthropic models (token budget) or Effort for other models.
type reasoningConfig struct {
	MaxTokens int    `json:"max_tokens,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

type chatRequest struct {
	Model        string              `json:"model"`
	Messages     []Message           `json:"messages"`
	Tools        []toolDefinition    `json:"tools,omitempty"`
	Stream       bool                `json:"stream"`
	Provider     *providerPreference `json:"provider,omitempty"`
	CacheControl *cacheControl       `json:"cache_control,omitempty"`
	Reasoning    *reasoningConfig    `json:"reasoning,omitempty"`
}

type providerPreference struct {
	RequireParameters bool `json:"require_parameters,omitempty"`
}

type toolDefinition struct {
	Type     string         `json:"type"`
	Function toolDescriptor `json:"function"`
}

type toolDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type promptTokensDetails struct {
	CachedTokens     int64 `json:"cached_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
}

// openRouterError captures the error envelope that OpenRouter embeds in both
// non-streaming JSON responses and SSE stream chunks.
type openRouterError struct {
	Code     int    `json:"code,omitempty"`
	Message  string `json:"message"`
	Metadata struct {
		ErrorType string `json:"error_type,omitempty"`
	} `json:"metadata,omitempty"`
}

type chatResponse struct {
	ID      string `json:"id,omitempty"`
	Model   string `json:"model,omitempty"`
	Choices []struct {
		Message      Message `json:"message"`
		Reasoning    string  `json:"reasoning,omitempty"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int64                `json:"prompt_tokens"`
		CompletionTokens    int64                `json:"completion_tokens"`
		TotalTokens         int64                `json:"total_tokens"`
		PromptTokensDetails *promptTokensDetails `json:"prompt_tokens_details,omitempty"`
	} `json:"usage"`
	Error *openRouterError `json:"error,omitempty"`
}

type chatStreamChunk struct {
	ID      string `json:"id,omitempty"`
	Model   string `json:"model,omitempty"`
	Choices []struct {
		Index        int             `json:"index"`
		Delta        chatStreamDelta `json:"delta"`
		Reasoning    string          `json:"reasoning,omitempty"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int64                `json:"prompt_tokens"`
		CompletionTokens    int64                `json:"completion_tokens"`
		TotalTokens         int64                `json:"total_tokens"`
		PromptTokensDetails *promptTokensDetails `json:"prompt_tokens_details,omitempty"`
	} `json:"usage"`
	Error *openRouterError `json:"error,omitempty"`
}

type chatStreamDelta struct {
	Role      string                `json:"role,omitempty"`
	Content   json.RawMessage       `json:"content,omitempty"`
	Reasoning string                `json:"reasoning,omitempty"`
	ToolCalls []streamToolCallDelta `json:"tool_calls,omitempty"`
}

type streamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

func newNativeLLMClientFromEnv(workspaceRoot string) (*openRouterClient, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if apiKey == "" {
		// Fall back to stored key in workspace secrets.
		// Use the workspace root explicitly — CWD is a session temp dir.
		apiKey = config.OpenRouterKeyFrom(workspaceRoot)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OpenRouter API key not found.\n\n" +
			"  Set it with:  toc config set-key openrouter\n" +
			"  Or export:    export OPENROUTER_API_KEY=sk-or-...\n\n" +
			"  Get a key at: https://openrouter.ai/keys")
	}

	return &openRouterClient{
		baseURL: resolveNativeBaseURLFromEnv(),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
		title:   strings.TrimSpace(os.Getenv("OPENROUTER_TITLE")),
		referer: strings.TrimSpace(os.Getenv("OPENROUTER_HTTP_REFERER")),
	}, nil
}

func (c *openRouterClient) Chat(ctx context.Context, req chatRequest) (*chatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	endpoint := c.baseURL + "/chat/completions"
	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if attempt > 0 {
			delay := retryDelay(attempt-1, nil)
			fmt.Fprintf(os.Stderr, "OpenRouter error (model=%s, endpoint=%s): %v; retrying in %s (attempt %d/%d)...\n",
				req.Model, endpoint, lastErr, delay, attempt+1, maxRetryAttempts)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		if c.title != "" {
			httpReq.Header.Set("X-OpenRouter-Title", c.title)
		}
		if c.referer != "" {
			httpReq.Header.Set("HTTP-Referer", c.referer)
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if isRetryableNetworkError(err) {
				lastErr = fmt.Errorf("network error: %w", err)
				continue
			}
			return nil, err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		if resp.StatusCode >= 400 {
			serverMsg := extractOpenRouterErrorMessage(data)
			if isRetryableStatusCode(resp.StatusCode) {
				lastErr = openRouterAPIError(resp.StatusCode, serverMsg, false)
				continue
			}
			return nil, openRouterAPIError(resp.StatusCode, serverMsg, false)
		}

		var parsed chatResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("failed to decode OpenRouter response: %w", err)
		}
		if len(parsed.Choices) == 0 {
			return nil, fmt.Errorf("openrouter returned no choices")
		}
		return &parsed, nil
	}

	// All retries exhausted — produce a descriptive final error.
	if lastErr != nil {
		if isRetryableNetworkError(lastErr) {
			return nil, fmt.Errorf("OpenRouter unreachable after %d attempts (model=%s): %w. Your session is saved and can be resumed.", maxRetryAttempts, req.Model, lastErr)
		}
		return nil, fmt.Errorf("OpenRouter error after %d attempts (model=%s): %w. Your session is saved and can be resumed.", maxRetryAttempts, req.Model, lastErr)
	}
	return nil, fmt.Errorf("OpenRouter request failed after %d attempts (model=%s)", maxRetryAttempts, req.Model)
}

func (c *openRouterClient) ChatStream(ctx context.Context, req chatRequest, onText func(string) error) (*chatResponse, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	endpoint := c.baseURL + "/chat/completions"
	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if attempt > 0 {
			delay := retryDelay(attempt-1, nil)
			fmt.Fprintf(os.Stderr, "OpenRouter error (model=%s, endpoint=%s): %v; retrying in %s (attempt %d/%d)...\n",
				req.Model, endpoint, lastErr, delay, attempt+1, maxRetryAttempts)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		assembled, streamErr := c.doStreamRequest(ctx, endpoint, body, onText)
		if streamErr == nil {
			return assembled, nil
		}

		// Determine whether this error is retryable.
		if isRetryableStreamError(streamErr) || isRetryableNetworkError(streamErr) {
			lastErr = streamErr
			continue
		}
		// Check for wrapped HTTP-level retryable errors.
		var apiErr *httpAPIError
		if errors.As(streamErr, &apiErr) && isRetryableStatusCode(apiErr.StatusCode) {
			lastErr = streamErr
			continue
		}

		// Non-retryable error — fail immediately.
		return nil, streamErr
	}

	// All retries exhausted.
	if lastErr != nil {
		return nil, fmt.Errorf("OpenRouter error after %d attempts (model=%s): %w. Your session is saved and can be resumed.", maxRetryAttempts, req.Model, lastErr)
	}
	return nil, fmt.Errorf("OpenRouter request failed after %d attempts (model=%s)", maxRetryAttempts, req.Model)
}

// httpAPIError wraps an HTTP-level error with the status code preserved, so
// the retry loop can inspect it.
type httpAPIError struct {
	StatusCode int
	Err        error
}

func (e *httpAPIError) Error() string { return e.Err.Error() }
func (e *httpAPIError) Unwrap() error { return e.Err }

// doStreamRequest performs a single streaming request and reads the SSE body.
// It returns a *chatResponse on success, or an error that the caller can
// classify as retryable or fatal.
func (c *openRouterClient) doStreamRequest(ctx context.Context, endpoint string, body []byte, onText func(string) error) (*chatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if c.title != "" {
		httpReq.Header.Set("X-OpenRouter-Title", c.title)
	}
	if c.referer != "" {
		httpReq.Header.Set("HTTP-Referer", c.referer)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		serverMsg := extractOpenRouterErrorMessage(data)
		return nil, &httpAPIError{
			StatusCode: resp.StatusCode,
			Err:        openRouterAPIError(resp.StatusCode, serverMsg, false),
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	assembled := &chatResponse{}
	var eventData []string
	processEvent := func() error {
		if len(eventData) == 0 {
			return nil
		}
		payload := strings.TrimSpace(strings.Join(eventData, "\n"))
		eventData = nil
		if payload == "" {
			return nil
		}
		if payload == "[DONE]" {
			return io.EOF
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("failed to decode OpenRouter stream chunk: %w", err)
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return &streamError{
				Code:      chunk.Error.Code,
				Message:   chunk.Error.Message,
				ErrorType: chunk.Error.Metadata.ErrorType,
			}
		}
		text, err := mergeStreamChunk(assembled, &chunk)
		if err != nil {
			return err
		}
		if text != "" && onText != nil {
			if err := onText(text); err != nil {
				return err
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			err := processEvent()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := processEvent(); err != nil && err != io.EOF {
		return nil, err
	}
	if len(assembled.Choices) == 0 {
		return nil, fmt.Errorf("openrouter returned no choices")
	}
	return assembled, nil
}

func mergeStreamChunk(resp *chatResponse, chunk *chatStreamChunk) (string, error) {
	if resp == nil || chunk == nil {
		return "", nil
	}
	if resp.ID == "" {
		resp.ID = chunk.ID
	}
	if resp.Model == "" {
		resp.Model = chunk.Model
	}
	if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		resp.Usage.PromptTokens = chunk.Usage.PromptTokens
		resp.Usage.CompletionTokens = chunk.Usage.CompletionTokens
		resp.Usage.TotalTokens = chunk.Usage.TotalTokens
		if chunk.Usage.PromptTokensDetails != nil {
			resp.Usage.PromptTokensDetails = chunk.Usage.PromptTokensDetails
		}
	}

	var streamedText strings.Builder
	for _, choiceChunk := range chunk.Choices {
		for len(resp.Choices) <= choiceChunk.Index {
			resp.Choices = append(resp.Choices, struct {
				Message      Message `json:"message"`
				Reasoning    string  `json:"reasoning,omitempty"`
				FinishReason string  `json:"finish_reason"`
			}{})
		}
		choice := &resp.Choices[choiceChunk.Index]
		if choice.Message.Role == "" {
			choice.Message.Role = "assistant"
		}
		if choiceChunk.Delta.Role != "" {
			choice.Message.Role = choiceChunk.Delta.Role
		}
		// Accumulate reasoning text (never streamed to stdout).
		if choiceChunk.Delta.Reasoning != "" {
			choice.Reasoning += choiceChunk.Delta.Reasoning
		}
		// Some providers put reasoning in the top-level chunk field.
		if choiceChunk.Reasoning != "" {
			choice.Reasoning += choiceChunk.Reasoning
		}
		text, err := normalizeOpenRouterContent(choiceChunk.Delta.Content)
		if err != nil {
			return "", err
		}
		if text != "" {
			choice.Message.Content += text
			if choiceChunk.Index == 0 {
				streamedText.WriteString(text)
			}
		}
		mergeToolCallDeltas(choice, choiceChunk.Delta.ToolCalls)
		if choiceChunk.FinishReason != "" {
			choice.FinishReason = choiceChunk.FinishReason
		}
	}
	return streamedText.String(), nil
}

func mergeToolCallDeltas(choice *struct {
	Message      Message `json:"message"`
	Reasoning    string  `json:"reasoning,omitempty"`
	FinishReason string  `json:"finish_reason"`
}, deltas []streamToolCallDelta) {
	for _, delta := range deltas {
		for len(choice.Message.ToolCalls) <= delta.Index {
			choice.Message.ToolCalls = append(choice.Message.ToolCalls, ToolCall{})
		}
		call := &choice.Message.ToolCalls[delta.Index]
		if call.ID == "" {
			call.ID = delta.ID
		}
		if call.Type == "" {
			call.Type = delta.Type
		}
		if delta.Function.Name != "" {
			call.Function.Name += delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			call.Function.Arguments += delta.Function.Arguments
		}
	}
}

func extractMessageText(msg Message) string {
	return strings.TrimSpace(msg.Content)
}

func normalizeOpenRouterContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			if part.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(part.Text)
		}
		return b.String(), nil
	}

	return "", fmt.Errorf("unsupported OpenRouter message content format")
}
