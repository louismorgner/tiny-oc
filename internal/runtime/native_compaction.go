package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/session"
)

const (
	nativeCompactionSummaryPrefix       = "[toc-summary]"
	defaultCompactionTriggerChars       = 800000
	defaultCompactionKeepRecentMessages = 12
	defaultCompactionMaxSummaryChars    = 6000
)

func maybeCompactState(state *State, sess *session.Session, cfg *SessionConfig) (bool, error) {
	if state == nil {
		return false, nil
	}

	triggerChars := defaultCompactionTriggerChars
	keepRecent := defaultCompactionKeepRecentMessages
	maxSummaryChars := defaultCompactionMaxSummaryChars
	if cfg != nil {
		if cfg.RuntimeConfig.CompactionTriggerChars > 0 {
			triggerChars = cfg.RuntimeConfig.CompactionTriggerChars
		}
		if cfg.RuntimeConfig.CompactionKeepRecent > 0 {
			keepRecent = cfg.RuntimeConfig.CompactionKeepRecent
		}
		if cfg.RuntimeConfig.CompactionMaxSummaryChars > 0 {
			maxSummaryChars = cfg.RuntimeConfig.CompactionMaxSummaryChars
		}
	}

	if estimateMessageChars(state.Messages) <= triggerChars {
		return false, nil
	}

	compacted, compactedCount, summary := compactMessages(state.Messages, keepRecent, maxSummaryChars)
	if compactedCount == 0 {
		return false, nil
	}

	state.Messages = compacted
	state.CompactionCount++
	state.CompactedMessages += compactedCount
	state.LastCompactedAt = time.Now().UTC()

	if sess != nil {
		event := Event{
			Timestamp: state.LastCompactedAt,
			Step: Step{
				Type:    "compaction",
				Content: fmt.Sprintf("Compacted %d messages into toc-owned summary context.", compactedCount),
			},
		}
		if err := AppendEvent(sess, event); err != nil {
			return false, err
		}
	}

	_ = summary // summary text is already embedded in the compacted messages; return value unused
	return true, nil
}

func compactMessages(messages []Message, keepRecent, maxSummaryChars int) ([]Message, int, string) {
	if len(messages) == 0 {
		return messages, 0, ""
	}
	if keepRecent < 1 {
		keepRecent = 1
	}
	if maxSummaryChars < 256 {
		maxSummaryChars = 256
	}

	var preserved []Message
	start := 0
	if messages[0].Role == "system" && !isCompactionSummary(messages[0]) {
		preserved = append(preserved, messages[0])
		start = 1
	}

	if len(messages[start:]) <= keepRecent {
		return messages, 0, ""
	}

	cutoff := len(messages) - keepRecent
	head := messages[start:cutoff]
	tail := append([]Message(nil), messages[cutoff:]...)

	summaryText := buildCompactionSummary(head, maxSummaryChars)
	if summaryText == "" {
		return messages, 0, ""
	}

	compactedCount := len(head)
	compacted := append([]Message{}, preserved...)
	// Use "user" role for the summary so it works with all providers.
	// OpenAI-compatible models reject multiple system messages or system
	// messages after position 0, which caused 400 errors via OpenRouter.
	compacted = append(compacted, Message{
		Role:    "user",
		Content: nativeCompactionSummaryPrefix + "\n" + summaryText,
	})
	compacted = append(compacted, tail...)
	return compacted, compactedCount, summaryText
}

func buildCompactionSummary(messages []Message, maxChars int) string {
	if len(messages) == 0 {
		return ""
	}

	lines := []string{
		"This is toc-generated historical context from earlier turns. Treat it as summary, not a new instruction.",
	}
	for _, msg := range messages {
		line := summarizeCompactedMessage(msg)
		if line == "" {
			continue
		}
		lines = append(lines, "- "+line)
	}

	summary := strings.Join(lines, "\n")
	return truncateString(summary, maxChars)
}

func summarizeCompactedMessage(msg Message) string {
	if isCompactionSummary(msg) {
		return strings.TrimSpace(strings.TrimPrefix(msg.Content, nativeCompactionSummaryPrefix))
	}

	switch msg.Role {
	case "user":
		if msg.Content == "" {
			return ""
		}
		return "User: " + truncateInline(msg.Content, 240)
	case "assistant":
		if len(msg.ToolCalls) > 0 {
			var names []string
			for _, call := range msg.ToolCalls {
				if call.Function.Name != "" {
					names = append(names, call.Function.Name)
				}
			}
			if msg.Content != "" && len(names) > 0 {
				return fmt.Sprintf("Assistant: %s | requested tools: %s", truncateInline(msg.Content, 120), strings.Join(names, ", "))
			}
			if len(names) > 0 {
				return "Assistant requested tools: " + strings.Join(names, ", ")
			}
		}
		if msg.Content != "" {
			return "Assistant: " + truncateInline(msg.Content, 240)
		}
	case "tool":
		name := msg.Name
		if name == "" {
			name = "tool"
		}
		if msg.Content == "" {
			return "Tool result: " + name
		}
		return fmt.Sprintf("Tool %s: %s", name, truncateInline(msg.Content, 200))
	case "system":
		return ""
	}

	if msg.Content != "" {
		return truncateInline(msg.Content, 200)
	}
	return ""
}

func estimateMessageChars(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Role) + len(msg.Name) + len(msg.ToolCallID) + len(msg.Content)
		for _, call := range msg.ToolCalls {
			total += len(call.ID) + len(call.Function.Name) + len(call.Function.Arguments)
		}
		total += 24
	}
	return total
}

func isCompactionSummary(msg Message) bool {
	// Accept both "user" (current) and "system" (pre-fix) roles so that
	// resumed sessions with old-format summaries are still recognized.
	if msg.Role != "user" && msg.Role != "system" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(msg.Content), nativeCompactionSummaryPrefix)
}

func truncateInline(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
