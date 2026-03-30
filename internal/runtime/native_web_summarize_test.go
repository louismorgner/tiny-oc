package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveSmallModel(t *testing.T) {
	t.Run("nil config returns empty", func(t *testing.T) {
		if got := resolveSmallModel(nil); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("empty SmallModel returns empty", func(t *testing.T) {
		cfg := &SessionConfig{SmallModel: ""}
		if got := resolveSmallModel(cfg); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("whitespace SmallModel returns empty", func(t *testing.T) {
		cfg := &SessionConfig{SmallModel: "   "}
		if got := resolveSmallModel(cfg); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("configured model is returned", func(t *testing.T) {
		cfg := &SessionConfig{SmallModel: "openai/gpt-4.1-mini"}
		if got := resolveSmallModel(cfg); got != "openai/gpt-4.1-mini" {
			t.Fatalf("expected openai/gpt-4.1-mini, got %q", got)
		}
	})
}

func TestSummarizeWebContentFallback(t *testing.T) {
	t.Run("nil LLMClient returns empty", func(t *testing.T) {
		ctx := nativeToolContext{
			Config:    &SessionConfig{SmallModel: "test-model"},
			LLMClient: nil,
		}
		summary, err := summarizeWebContent(ctx, "https://example.com", "", "some content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary != "" {
			t.Fatalf("expected empty summary, got %q", summary)
		}
	})

	t.Run("empty SmallModel returns empty", func(t *testing.T) {
		ctx := nativeToolContext{
			Config:    &SessionConfig{SmallModel: ""},
			LLMClient: &openRouterClient{},
		}
		summary, err := summarizeWebContent(ctx, "https://example.com", "", "some content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary != "" {
			t.Fatalf("expected empty summary, got %q", summary)
		}
	})
}

func newTestLLMClient(t *testing.T, handler http.HandlerFunc) (*openRouterClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := &openRouterClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
	}
	return client, server
}

func TestSummarizeWebContentSuccess(t *testing.T) {
	client, server := newTestLLMClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp := chatResponse{
			Choices: []struct {
				Message      Message `json:"message"`
				Reasoning    string  `json:"reasoning,omitempty"`
				FinishReason string  `json:"finish_reason"`
			}{
				{Message: Message{Role: "assistant", Content: "This is the summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	rawContent := "URL: https://example.com\nStatus: 200 OK\nContent-Type: text/html\nTitle: Example\n\nHere is the full page content..."

	ctx := nativeToolContext{
		Config:    &SessionConfig{SmallModel: "test-model"},
		LLMClient: client,
	}
	summary, err := summarizeWebContent(ctx, "https://example.com", "", rawContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}

	// Metadata lines from the raw content should be preserved.
	if !strings.Contains(summary, "Status: 200 OK") {
		t.Fatalf("expected preserved status metadata, got %q", summary)
	}
	if !strings.Contains(summary, "Content-Type: text/html") {
		t.Fatalf("expected preserved content-type metadata, got %q", summary)
	}
	if !strings.Contains(summary, "[Summarized by test-model]") {
		t.Fatalf("expected model tag, got %q", summary)
	}
	if !strings.Contains(summary, "This is the summary.") {
		t.Fatalf("expected summary body, got %q", summary)
	}
}

func TestSummarizeWebContentWithQuery(t *testing.T) {
	var capturedReq chatRequest
	client, server := newTestLLMClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		resp := chatResponse{
			Choices: []struct {
				Message      Message `json:"message"`
				Reasoning    string  `json:"reasoning,omitempty"`
				FinishReason string  `json:"finish_reason"`
			}{
				{Message: Message{Role: "assistant", Content: "Focused summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	ctx := nativeToolContext{
		Config:    &SessionConfig{SmallModel: "test-model"},
		LLMClient: client,
	}
	_, err := summarizeWebContent(ctx, "https://example.com", "how to install", "URL: https://example.com\n\nPage content here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The user prompt should include the query.
	userMsg := capturedReq.Messages[len(capturedReq.Messages)-1].Content
	if !strings.Contains(userMsg, "Query: how to install") {
		t.Fatalf("expected query in user prompt, got %q", userMsg)
	}
}

func TestSummarizeWebContentChatError(t *testing.T) {
	client, server := newTestLLMClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", 500)
	})
	defer server.Close()

	ctx := nativeToolContext{
		Config:    &SessionConfig{SmallModel: "test-model"},
		LLMClient: client,
	}
	summary, err := summarizeWebContent(ctx, "https://example.com", "", "URL: https://example.com\n\ncontent")
	if err == nil {
		t.Fatal("expected error from Chat failure")
	}
	if summary != "" {
		t.Fatalf("expected empty summary on error, got %q", summary)
	}
}

func TestSummarizeWebContentTruncatesLargeInput(t *testing.T) {
	var capturedReq chatRequest
	client, server := newTestLLMClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		resp := chatResponse{
			Choices: []struct {
				Message      Message `json:"message"`
				Reasoning    string  `json:"reasoning,omitempty"`
				FinishReason string  `json:"finish_reason"`
			}{
				{Message: Message{Role: "assistant", Content: "Summary of large page."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	// Build content larger than maxSummarizeInputChars.
	bigContent := "URL: https://example.com\n\n" + strings.Repeat("x", maxSummarizeInputChars+1000)

	ctx := nativeToolContext{
		Config:    &SessionConfig{SmallModel: "test-model"},
		LLMClient: client,
	}
	_, err := summarizeWebContent(ctx, "https://example.com", "", bigContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The user prompt sent to the model should contain the truncation marker.
	userMsg := capturedReq.Messages[len(capturedReq.Messages)-1].Content
	if !strings.Contains(userMsg, "[Content truncated for summarization]") {
		t.Fatal("expected truncation marker in user prompt sent to model")
	}
	// The content in the prompt should be capped.
	contentStart := strings.Index(userMsg, "---\n") + 4
	contentEnd := strings.Index(userMsg[contentStart:], "\n---")
	if contentEnd > maxSummarizeInputChars+100 {
		t.Fatalf("content sent to model is too large: %d chars", contentEnd)
	}
}
