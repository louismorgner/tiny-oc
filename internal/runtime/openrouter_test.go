package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tiny-oc/toc/internal/session"
)

func TestNewNativeLLMClientFromEnv_PrefersTOCNativeBaseURL(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("TOC_NATIVE_BASE_URL", "http://localhost:8000")
	t.Setenv("OPENROUTER_BASE_URL", "http://localhost:9000")

	client, err := newNativeLLMClientFromEnv(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if client.baseURL != "http://localhost:8000" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}

func TestMergeStreamChunk_AccumulatesReasoning(t *testing.T) {
	resp := &chatResponse{}

	// First chunk: reasoning in delta
	chunk1 := &chatStreamChunk{
		ID:    "resp-1",
		Model: "anthropic/claude-sonnet-4",
	}
	chunk1.Choices = append(chunk1.Choices, struct {
		Index        int             `json:"index"`
		Delta        chatStreamDelta `json:"delta"`
		Reasoning    string          `json:"reasoning,omitempty"`
		FinishReason string          `json:"finish_reason"`
	}{
		Index: 0,
		Delta: chatStreamDelta{Role: "assistant", Reasoning: "Let me think"},
	})
	if _, err := mergeStreamChunk(resp, chunk1); err != nil {
		t.Fatal(err)
	}

	// Second chunk: more reasoning in delta
	chunk2 := &chatStreamChunk{}
	chunk2.Choices = append(chunk2.Choices, struct {
		Index        int             `json:"index"`
		Delta        chatStreamDelta `json:"delta"`
		Reasoning    string          `json:"reasoning,omitempty"`
		FinishReason string          `json:"finish_reason"`
	}{
		Index: 0,
		Delta: chatStreamDelta{Reasoning: " about this"},
	})
	if _, err := mergeStreamChunk(resp, chunk2); err != nil {
		t.Fatal(err)
	}

	// Third chunk: reasoning in top-level chunk field (provider variant)
	chunk3 := &chatStreamChunk{}
	chunk3.Choices = append(chunk3.Choices, struct {
		Index        int             `json:"index"`
		Delta        chatStreamDelta `json:"delta"`
		Reasoning    string          `json:"reasoning,omitempty"`
		FinishReason string          `json:"finish_reason"`
	}{
		Index:     0,
		Reasoning: " carefully",
	})
	if _, err := mergeStreamChunk(resp, chunk3); err != nil {
		t.Fatal(err)
	}

	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	want := "Let me think about this carefully"
	if got := resp.Choices[0].Reasoning; got != want {
		t.Fatalf("accumulated reasoning = %q, want %q", got, want)
	}
}

func TestMergeStreamChunk_ReasoningNotStreamedAsText(t *testing.T) {
	resp := &chatResponse{}

	chunk := &chatStreamChunk{}
	chunk.Choices = append(chunk.Choices, struct {
		Index        int             `json:"index"`
		Delta        chatStreamDelta `json:"delta"`
		Reasoning    string          `json:"reasoning,omitempty"`
		FinishReason string          `json:"finish_reason"`
	}{
		Index: 0,
		Delta: chatStreamDelta{
			Role:      "assistant",
			Reasoning: "internal reasoning",
			Content:   json.RawMessage(`"visible text"`),
		},
	})

	text, err := mergeStreamChunk(resp, chunk)
	if err != nil {
		t.Fatal(err)
	}
	if text != "visible text" {
		t.Fatalf("streamed text = %q, want %q", text, "visible text")
	}
	if resp.Choices[0].Reasoning != "internal reasoning" {
		t.Fatalf("reasoning = %q, want %q", resp.Choices[0].Reasoning, "internal reasoning")
	}
}

func TestChatRequest_ReasoningConfigSerialization(t *testing.T) {
	tests := []struct {
		name string
		rc   *reasoningConfig
		want string
	}{
		{
			name: "budget_tokens only",
			rc:   &reasoningConfig{MaxTokens: 8000},
			want: `{"max_tokens":8000}`,
		},
		{
			name: "effort only",
			rc:   &reasoningConfig{Effort: "high"},
			want: `{"effort":"high"}`,
		},
		{
			name: "nil reasoning omitted",
			rc:   nil,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := chatRequest{
				Model:     "test-model",
				Messages:  []Message{{Role: "user", Content: "hi"}},
				Stream:    true,
				Reasoning: tt.rc,
			}
			data, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			if tt.rc == nil {
				if _, exists := raw["reasoning"]; exists {
					t.Fatal("expected reasoning to be omitted when nil")
				}
			} else {
				got := string(raw["reasoning"])
				if got != tt.want {
					t.Fatalf("reasoning JSON = %s, want %s", got, tt.want)
				}
			}
		})
	}
}

func TestAppendEvent_ThinkingStep(t *testing.T) {
	sess := &session.Session{ID: "sess-thinking-event", MetadataDir: t.TempDir()}

	if err := AppendEvent(sess, Event{
		Timestamp: time.Now().UTC(),
		Step: Step{
			Type:    "thinking",
			Content: "Let me reason about this problem",
		},
	}); err != nil {
		t.Fatal(err)
	}

	parsed, err := LoadEventLog(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(parsed.Events))
	}
	if parsed.Events[0].Step.Type != "thinking" {
		t.Fatalf("step type = %q, want %q", parsed.Events[0].Step.Type, "thinking")
	}
	if parsed.Events[0].Step.Content != "Let me reason about this problem" {
		t.Fatalf("step content = %q", parsed.Events[0].Step.Content)
	}
}

func TestIsRetryableStreamError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "502 provider_unavailable",
			err:       &streamError{Code: 502, Message: "Overloaded", ErrorType: "provider_unavailable"},
			retryable: true,
		},
		{
			name:      "503 service unavailable",
			err:       &streamError{Code: 503, Message: "Service Unavailable"},
			retryable: true,
		},
		{
			name:      "529 site overloaded",
			err:       &streamError{Code: 529, Message: "Site Overloaded"},
			retryable: true,
		},
		{
			name:      "provider_overloaded error type",
			err:       &streamError{Code: 0, Message: "try again", ErrorType: "provider_overloaded"},
			retryable: true,
		},
		{
			name:      "rate_limited error type",
			err:       &streamError{Code: 0, Message: "slow down", ErrorType: "rate_limited"},
			retryable: true,
		},
		{
			name:      "message contains overloaded",
			err:       &streamError{Code: 0, Message: "Server is Overloaded, please retry"},
			retryable: true,
		},
		{
			name:      "400 bad request not retryable",
			err:       &streamError{Code: 400, Message: "Bad Request"},
			retryable: false,
		},
		{
			name:      "401 unauthorized not retryable",
			err:       &streamError{Code: 401, Message: "Unauthorized"},
			retryable: false,
		},
		{
			name:      "unknown error not retryable",
			err:       &streamError{Code: 0, Message: "something unexpected"},
			retryable: false,
		},
		{
			name:      "non-stream error not retryable",
			err:       fmt.Errorf("some other error"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableStreamError(tt.err)
			if got != tt.retryable {
				t.Errorf("isRetryableStreamError(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

// TestChatStream_RetriesTransientStreamError verifies that a transient error
// embedded in an SSE chunk (HTTP 200 body) triggers a retry.
func TestChatStream_RetriesTransientStreamError(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")

		if n == 1 {
			// First call: return a transient error in the stream.
			errPayload := map[string]interface{}{
				"error": map[string]interface{}{
					"code":    502,
					"message": "Overloaded",
					"metadata": map[string]interface{}{
						"error_type": "provider_unavailable",
					},
				},
			}
			data, _ := json.Marshal(errPayload)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}

		// Second call: return a successful response.
		chunk1 := map[string]interface{}{
			"id":    "resp-1",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{"index": 0, "delta": map[string]interface{}{"role": "assistant", "content": "Hello"}},
			},
		}
		chunk2 := map[string]interface{}{
			"id":    "resp-1",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{"index": 0, "delta": map[string]interface{}{"content": " world"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		}
		for _, c := range []map[string]interface{}{chunk1, chunk2} {
			data, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	client := &openRouterClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
	}

	var streamed string
	resp, err := client.ChatStream(context.Background(), chatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(text string) error {
		streamed += text
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected 2 HTTP calls (1 failed + 1 success), got %d", got)
	}
	if streamed != "Hello world" {
		t.Fatalf("streamed text = %q, want %q", streamed, "Hello world")
	}
	if resp.Choices[0].Message.Content != "Hello world" {
		t.Fatalf("assembled content = %q", resp.Choices[0].Message.Content)
	}
}

// TestChatStream_NoRetryOnNonTransientStreamError verifies that a non-transient
// error in the stream fails immediately without retrying.
func TestChatStream_NoRetryOnNonTransientStreamError(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")

		errPayload := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    400,
				"message": "Invalid request: messages is required",
			},
		}
		data, _ := json.Marshal(errPayload)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	client := &openRouterClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
	}

	_, err := client.ChatStream(context.Background(), chatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 HTTP call (no retries), got %d", got)
	}
}

// TestChatStream_RetriesHTTP502ThenSucceeds verifies HTTP-level 502 retries.
func TestChatStream_RetriesHTTP502ThenSucceeds(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `{"error":{"message":"Bad Gateway"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := map[string]interface{}{
			"id":    "resp-1",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{"index": 0, "delta": map[string]interface{}{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := &openRouterClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
	}

	resp, err := client.ChatStream(context.Background(), chatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("content = %q", resp.Choices[0].Message.Content)
	}
}

// TestChatStream_ExhaustsRetriesOnPersistentStreamError verifies that after
// maxRetryAttempts, the error is surfaced with the "session is saved" message.
func TestChatStream_ExhaustsRetriesOnPersistentStreamError(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		errPayload := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    502,
				"message": "Overloaded",
				"metadata": map[string]interface{}{
					"error_type": "provider_unavailable",
				},
			},
		}
		data, _ := json.Marshal(errPayload)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	client := &openRouterClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
	}

	_, err := client.ChatStream(context.Background(), chatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, nil)

	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := callCount.Load(); got != int32(maxRetryAttempts) {
		t.Fatalf("expected %d calls, got %d", maxRetryAttempts, got)
	}
}
