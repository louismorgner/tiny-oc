package runtime

import (
	"encoding/json"
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

	currentChars := estimateMessageChars(state.Messages)

	// Phase 1: Progressive tool result aging.
	// Before doing a full compaction (which loses context), try shrinking
	// old tool outputs first. This is cheaper and preserves more signal.
	// We age tool results that are older than keepRecent messages, reducing
	// them to a smaller budget (1/4 of original tool budget).
	if currentChars > triggerChars*3/4 {
		aged := ageToolResults(state.Messages, keepRecent)
		if aged > 0 {
			currentChars = estimateMessageChars(state.Messages)
		}
	}

	if currentChars <= triggerChars {
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
			// Preserve tool call details — the paths, commands, and patterns
			// are more useful than just the tool names for context reconstruction.
			var callSummaries []string
			for _, call := range msg.ToolCalls {
				callSummaries = append(callSummaries, summarizeToolCall(call))
			}
			calls := strings.Join(callSummaries, "; ")
			if msg.Content != "" {
				return fmt.Sprintf("Assistant: %s | tools: %s", truncateInline(msg.Content, 120), calls)
			}
			return "Assistant tools: " + calls
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
		// For tool results, include the first line (often a summary or file path)
		// plus a size hint so the model knows how much was there.
		firstLine := msg.Content
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		sizeHint := ""
		if len(msg.Content) > 200 {
			sizeHint = fmt.Sprintf(" [%d bytes total]", len(msg.Content))
		}
		return fmt.Sprintf("Tool %s: %s%s", name, truncateInline(firstLine, 160), sizeHint)
	case "system":
		return ""
	}

	if msg.Content != "" {
		return truncateInline(msg.Content, 200)
	}
	return ""
}

// summarizeToolCall extracts the key parameter (path, command, pattern)
// from a tool call so compaction summaries retain actionable context.
// Knowing "Read(main.go)" is far more useful than just "Read".
func summarizeToolCall(call ToolCall) string {
	name := call.Function.Name
	if name == "" {
		return "unknown"
	}
	key := extractToolCallKey(name, call.Function.Arguments)
	if key != "" {
		return name + "(" + key + ")"
	}
	return name
}

// extractToolCallKey pulls the most identifying parameter from a tool call's
// JSON arguments. This is a best-effort extraction — if parsing fails, we
// return empty and fall back to just the tool name.
func extractToolCallKey(toolName, argsJSON string) string {
	if argsJSON == "" {
		return ""
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	// Each tool has a primary identifying parameter.
	var key string
	switch toolName {
	case "Read", "Write", "Edit":
		key, _ = args["file_path"].(string)
	case "Bash":
		key, _ = args["command"].(string)
	case "Grep":
		key, _ = args["pattern"].(string)
	case "Glob":
		key, _ = args["pattern"].(string)
	case "Skill":
		key, _ = args["skill"].(string)
	case "SubAgent":
		key, _ = args["action"].(string)
	}
	return truncateInline(key, 80)
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

// ageToolResults shrinks tool result messages that are older than keepRecent
// positions in the message list. It replaces large tool outputs with
// middle-truncated versions at 1/4 of the tool's normal budget.
//
// This is the first line of defense against context bloat: before doing a
// full compaction that summarizes and drops messages, we can reclaim
// significant space by trimming the large outputs that have already been
// consumed by the model in earlier turns.
//
// Returns the number of messages that were shrunk.
func ageToolResults(messages []Message, keepRecent int) int {
	if len(messages) <= keepRecent {
		return 0
	}

	cutoff := len(messages) - keepRecent
	aged := 0
	for i := 0; i < cutoff; i++ {
		msg := &messages[i]
		if msg.Role != "tool" || msg.Content == "" {
			continue
		}
		// Age to 1/4 of the tool's normal budget.
		agedBudget := toolOutputBudget(msg.Name) / 4
		if agedBudget < 512 {
			agedBudget = 512
		}
		if len(msg.Content) <= agedBudget {
			continue
		}
		msg.Content = truncateMiddle(msg.Content, agedBudget)
		aged++
	}
	return aged
}
