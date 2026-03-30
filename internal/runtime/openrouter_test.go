package runtime

import (
	"encoding/json"
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
