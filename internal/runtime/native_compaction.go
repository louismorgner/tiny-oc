package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/runtimeinfo"
	"github.com/tiny-oc/toc/internal/session"
)

const (
	nativeCompactionSummaryPrefix = "[toc-summary]"
	// nativeCompactionContinuationPrefix marks structured continuation artifacts.
	nativeCompactionContinuationPrefix = "[toc-continuation]"

	defaultCompactionKeepRecentMessages = 12
	defaultCompactionMaxSummaryChars    = 6000
)

// pruneProtectedTools lists tool names whose outputs should survive pruning
// because they contain high-signal results (errors, diffs, coordination).
var pruneProtectedTools = map[string]bool{
	"Edit":     true, // diffs and patch-like output
	"Write":    true, // confirmations are already small
	"SubAgent": true, // coordination metadata
}

// maybeManageContext is the new entry point for context management. It uses
// token-budget-aware decisions when a model profile is available, falling
// back to the legacy char-threshold path otherwise.
func maybeManageContext(state *State, sess *session.Session, cfg *SessionConfig, profile runtimeinfo.NativeModelProfile, client *openRouterClient) (bool, error) {
	if state == nil {
		return false, nil
	}

	keepRecent := defaultCompactionKeepRecentMessages
	maxSummaryChars := defaultCompactionMaxSummaryChars
	if cfg != nil {
		if cfg.RuntimeConfig.CompactionKeepRecent > 0 {
			keepRecent = cfg.RuntimeConfig.CompactionKeepRecent
		}
		if cfg.RuntimeConfig.CompactionMaxSummaryChars > 0 {
			maxSummaryChars = cfg.RuntimeConfig.CompactionMaxSummaryChars
		}
	}

	return manageContextWithBudget(state, sess, keepRecent, maxSummaryChars, profile, client)
}

// manageContextWithBudget implements the token-budget-aware context pipeline:
//   1. Estimate current input tokens
//   2. Consult the budgeter for a decision
//   3. Prune → compact → fail based on the decision
func manageContextWithBudget(state *State, sess *session.Session, keepRecent, maxSummaryChars int, profile runtimeinfo.NativeModelProfile, client *openRouterClient) (bool, error) {
	budgeter := NewContextBudgeter(profile)
	currentTokens := estimateMessagesTokens(state.Messages)
	decision := budgeter.Evaluate(currentTokens)

	switch decision {
	case BudgetContinue:
		return false, nil

	case BudgetPrune:
		pruned := pruneStaleToolOutputs(state.Messages, keepRecent)
		if pruned > 0 {
			emitContextEvent(sess, fmt.Sprintf("Pruned %d stale tool outputs (token budget: %d/%d).", pruned, estimateMessagesTokens(state.Messages), budgeter.InputBudget()))
		}
		return pruned > 0, nil

	case BudgetCompact:
		// First try pruning
		pruned := pruneStaleToolOutputs(state.Messages, keepRecent)

		// Re-evaluate after pruning
		currentTokens = estimateMessagesTokens(state.Messages)
		postPruneDecision := budgeter.Evaluate(currentTokens)
		if postPruneDecision == BudgetContinue || postPruneDecision == BudgetPrune {
			if pruned > 0 {
				emitContextEvent(sess, fmt.Sprintf("Pruned %d stale tool outputs, compaction avoided (token budget: %d/%d).", pruned, currentTokens, budgeter.InputBudget()))
			}
			return pruned > 0, nil
		}

		// Full compaction needed
		compacted, compactedCount, _ := compactMessagesStructured(state, keepRecent, maxSummaryChars, client)
		if compactedCount == 0 {
			return pruned > 0, nil
		}

		state.Messages = compacted
		state.CompactionCount++
		state.CompactedMessages += compactedCount
		state.LastCompactedAt = time.Now().UTC()

		emitContextEvent(sess, fmt.Sprintf("Compacted %d messages into structured continuation (token budget: %d/%d).", compactedCount, estimateMessagesTokens(state.Messages), budgeter.InputBudget()))
		return true, nil

	case BudgetFail:
		// Emergency: try aggressive pruning + compaction before giving up
		pruneStaleToolOutputs(state.Messages, keepRecent)
		compacted, compactedCount, _ := compactMessagesStructured(state, keepRecent, maxSummaryChars, client)
		if compactedCount > 0 {
			state.Messages = compacted
			state.CompactionCount++
			state.CompactedMessages += compactedCount
			state.LastCompactedAt = time.Now().UTC()

			currentTokens = estimateMessagesTokens(state.Messages)
			if budgeter.Evaluate(currentTokens) != BudgetFail {
				emitContextEvent(sess, fmt.Sprintf("Emergency compaction: %d messages compacted (token budget: %d/%d).", compactedCount, currentTokens, budgeter.InputBudget()))
				return true, nil
			}
		}
		return false, fmt.Errorf("context exceeds model budget (%d tokens estimated, %d available) after emergency compaction", estimateMessagesTokens(state.Messages), budgeter.InputBudget())
	}

	return false, nil
}

// pruneStaleToolOutputs replaces old, low-value tool outputs with short stubs.
// Unlike ageToolResults (which shrinks to 1/4 budget), this completely replaces
// the content with a marker. Protected tools (errors, diffs) are preserved.
func pruneStaleToolOutputs(messages []Message, keepRecent int) int {
	if len(messages) <= keepRecent {
		return 0
	}

	cutoff := len(messages) - keepRecent
	pruned := 0
	for i := 0; i < cutoff; i++ {
		msg := &messages[i]
		if msg.Role != "tool" || msg.Content == "" {
			continue
		}

		// Skip protected tools
		if pruneProtectedTools[msg.Name] {
			continue
		}

		// Protect error results — they contain diagnostic signal
		if looksLikeError(msg.Content) {
			continue
		}

		// Already pruned or very small — skip
		if len(msg.Content) <= 512 {
			continue
		}

		// Age to 1/4 budget. For very small budgets, replace with a clean
		// prune marker instead of a mostly-marker truncation.
		agedBudget := toolOutputBudget(msg.Name) / 4
		if agedBudget < 512 {
			agedBudget = 512
		}
		if len(msg.Content) > agedBudget {
			if agedBudget <= 512 {
				msg.Content = pruneMarker
			} else {
				msg.Content = truncateMiddle(msg.Content, agedBudget)
			}
			pruned++
		}
	}
	return pruned
}

// looksLikeError returns true if the content likely contains an error message
// worth preserving through pruning.
func looksLikeError(content string) bool {
	// Check the first 500 bytes for error indicators
	check := content
	if len(check) > 500 {
		check = check[:500]
	}
	lower := strings.ToLower(check)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "exit code")
}

// compactMessagesStructured performs full compaction, generating a structured
// continuation artifact. When a client is available, uses the LLM to generate
// the continuation; falls back to heuristic extraction otherwise.
func compactMessagesStructured(state *State, keepRecent, maxSummaryChars int, client *openRouterClient) ([]Message, int, string) {
	messages := state.Messages
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
	if messages[0].Role == "system" && !isCompactionSummary(messages[0]) && !isContinuationArtifact(messages[0]) {
		preserved = append(preserved, messages[0])
		start = 1
	}

	if len(messages[start:]) <= keepRecent {
		return messages, 0, ""
	}

	cutoff := len(messages) - keepRecent
	head := messages[start:cutoff]
	tail := append([]Message(nil), messages[cutoff:]...)

	// Try LLM-generated continuation first; fall back to heuristic extraction.
	var continuation ContinuationArtifact
	if client != nil && state.Model != "" {
		llmCont, err := generateContinuationViaLLM(client, state.Model, head, state.Continuation)
		if err == nil && llmCont != nil {
			continuation = *llmCont
		} else {
			continuation = buildContinuationFromMessages(head, state)
		}
	} else {
		continuation = buildContinuationFromMessages(head, state)
	}
	state.Continuation = &continuation

	// Render continuation as injection text
	continuationText := renderContinuation(continuation, maxSummaryChars)
	if continuationText == "" {
		return messages, 0, ""
	}

	compactedCount := len(head)
	compacted := append([]Message{}, preserved...)
	compacted = append(compacted, Message{
		Role:    "user",
		Content: nativeCompactionContinuationPrefix + "\n" + continuationText,
	})
	compacted = append(compacted, tail...)
	return compacted, compactedCount, continuationText
}

// buildContinuationFromMessages extracts structured context from compacted
// messages. This replaces the old freeform bullet-point summary.
func buildContinuationFromMessages(messages []Message, state *State) ContinuationArtifact {
	c := ContinuationArtifact{
		GeneratedAt: time.Now().UTC(),
	}

	// Inherit from previous continuation if one exists
	if state.Continuation != nil {
		c.Constraints = append(c.Constraints, state.Continuation.Constraints...)
		c.Decisions = append(c.Decisions, state.Continuation.Decisions...)
	}

	// Extract working files from working set if available
	if state.WorkingSet != nil {
		for _, f := range state.WorkingSet.FilesEdited {
			c.WorkingFiles = appendUnique(c.WorkingFiles, f, 30)
		}
		for _, f := range state.WorkingSet.FilesWritten {
			c.WorkingFiles = appendUnique(c.WorkingFiles, f, 30)
		}
	}

	// Extract goal from first user message
	for _, msg := range messages {
		if msg.Role == "user" && !isCompactionSummary(msg) && !isContinuationArtifact(msg) {
			if msg.Content != "" {
				c.Goal = truncateInline(msg.Content, 300)
				break
			}
		}
	}

	// Summarize completed tool work
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, call := range msg.ToolCalls {
				summary := summarizeToolCall(call)
				c.CompletedWork = appendCapped(c.CompletedWork, summary, 30)
			}
		}
	}

	// Extract discoveries from assistant text
	for _, msg := range messages {
		if msg.Role == "assistant" && msg.Content != "" && len(msg.ToolCalls) == 0 {
			// Assistant messages without tool calls are usually explanations/findings
			c.Discoveries = appendCapped(c.Discoveries, truncateInline(msg.Content, 200), 10)
		}
	}

	// Extract tool errors as open loops
	for _, msg := range messages {
		if msg.Role == "tool" && looksLikeError(msg.Content) {
			errSummary := fmt.Sprintf("%s error: %s", msg.Name, truncateInline(msg.Content, 150))
			c.OpenLoops = appendCapped(c.OpenLoops, errSummary, 10)
		}
	}

	// Derive next steps from open loops and recent tool calls
	if len(c.OpenLoops) > 0 {
		for _, loop := range c.OpenLoops {
			c.NextSteps = appendCapped(c.NextSteps, "Resolve: "+truncateInline(loop, 150), 5)
		}
	}

	// Carry forward remaining work from prior continuation
	if state.Continuation != nil {
		for _, w := range state.Continuation.RemainingWork {
			c.RemainingWork = appendCapped(c.RemainingWork, w, 10)
		}
		for _, s := range state.Continuation.NextSteps {
			c.NextSteps = appendCapped(c.NextSteps, s, 10)
		}
	}

	// Fold in any existing continuation summary content
	for _, msg := range messages {
		if isCompactionSummary(msg) || isContinuationArtifact(msg) {
			// Inherit prior continuation's data if it was structured
			content := msg.Content
			content = strings.TrimPrefix(content, nativeCompactionContinuationPrefix)
			content = strings.TrimPrefix(content, nativeCompactionSummaryPrefix)
			content = strings.TrimSpace(content)
			if content != "" {
				// Try to parse as structured continuation
				var prior ContinuationArtifact
				if err := json.Unmarshal([]byte(extractJSONBlock(content)), &prior); err == nil {
					mergeContinuation(&c, &prior)
				}
			}
		}
	}

	return c
}

// mergeContinuation folds prior continuation fields into current, avoiding duplicates.
func mergeContinuation(dst, src *ContinuationArtifact) {
	if src.Goal != "" && dst.Goal == "" {
		dst.Goal = src.Goal
	}
	for _, v := range src.Constraints {
		dst.Constraints = appendUnique(dst.Constraints, v, 20)
	}
	for _, v := range src.Decisions {
		dst.Decisions = appendUnique(dst.Decisions, v, 20)
	}
	for _, v := range src.WorkingFiles {
		dst.WorkingFiles = appendUnique(dst.WorkingFiles, v, 30)
	}
}

// renderContinuation produces the text that gets injected into the message
// history as a continuation artifact.
func renderContinuation(c ContinuationArtifact, maxChars int) string {
	var sections []string
	sections = append(sections, "This is a toc-generated structured continuation from compacted history. Use it to maintain context, not as a new instruction.")

	if c.Goal != "" {
		sections = append(sections, "\n## Goal\n"+c.Goal)
	}
	if len(c.Constraints) > 0 {
		sections = append(sections, "\n## Constraints\n"+bulletList(c.Constraints))
	}
	if len(c.Decisions) > 0 {
		sections = append(sections, "\n## Decisions\n"+bulletList(c.Decisions))
	}
	if len(c.Discoveries) > 0 {
		sections = append(sections, "\n## Discoveries\n"+bulletList(c.Discoveries))
	}
	if len(c.WorkingFiles) > 0 {
		sections = append(sections, "\n## Working Files\n"+bulletList(c.WorkingFiles))
	}
	if len(c.CompletedWork) > 0 {
		sections = append(sections, "\n## Completed Work\n"+bulletList(c.CompletedWork))
	}
	if len(c.RemainingWork) > 0 {
		sections = append(sections, "\n## Remaining Work\n"+bulletList(c.RemainingWork))
	}
	if len(c.OpenLoops) > 0 {
		sections = append(sections, "\n## Open Loops\n"+bulletList(c.OpenLoops))
	}
	if len(c.NextSteps) > 0 {
		sections = append(sections, "\n## Next Steps\n"+bulletList(c.NextSteps))
	}

	text := strings.Join(sections, "\n")
	return truncateString(text, maxChars)
}

func bulletList(items []string) string {
	var lines []string
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}

func isContinuationArtifact(msg Message) bool {
	if msg.Role != "user" && msg.Role != "system" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(msg.Content), nativeCompactionContinuationPrefix)
}

// extractJSONBlock tries to find a JSON object in a string (for parsing
// prior structured continuations). Note: brace-counting can be fooled by
// braces inside JSON string values; this is acceptable here because the
// continuation schema only contains simple strings without embedded braces.
func extractJSONBlock(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	// Find the matching closing brace
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func emitContextEvent(sess *session.Session, content string) {
	if sess == nil {
		return
	}
	_ = AppendEvent(sess, Event{
		Timestamp: time.Now().UTC(),
		Step: Step{
			Type:    "compaction",
			Content: content,
		},
	})
}

// continuationPrompt is the system instruction sent to the LLM to generate
// a structured continuation artifact during compaction.
const continuationPrompt = `You are a session-state summarizer. Given conversation messages being compacted, produce a JSON continuation artifact.

Output ONLY valid JSON matching this schema (no markdown, no explanation):
{
  "goal": "the user's primary objective",
  "constraints": ["any constraints or requirements mentioned"],
  "decisions": ["key decisions made during the conversation"],
  "discoveries": ["important findings or observations"],
  "working_files": ["files that were read, edited, or created"],
  "completed_work": ["work that has been finished"],
  "remaining_work": ["work still to be done"],
  "open_loops": ["unresolved errors, blockers, or questions"],
  "next_steps": ["what should happen next"]
}

Omit empty arrays. Be concise — each entry should be one sentence max.`

// generateContinuationViaLLM calls the model to synthesize a structured
// continuation from the messages being compacted. Falls back to nil on error
// so the caller can use heuristic extraction.
func generateContinuationViaLLM(client *openRouterClient, model string, messages []Message, existing *ContinuationArtifact) (*ContinuationArtifact, error) {
	if client == nil || model == "" {
		return nil, fmt.Errorf("no client or model")
	}

	// Build a compact representation of the messages to summarize.
	// We don't send raw messages — just a summary to keep the request small.
	var summaryLines []string
	for _, msg := range messages {
		line := summarizeCompactedMessage(msg)
		if line != "" {
			summaryLines = append(summaryLines, line)
		}
	}
	if len(summaryLines) == 0 {
		return nil, fmt.Errorf("no content to summarize")
	}

	contextText := strings.Join(summaryLines, "\n")
	if existing != nil {
		contextText += "\n\nPrior continuation context:\n" + renderContinuation(*existing, 2000)
	}

	req := chatRequest{
		Model: model,
		Messages: []Message{
			{Role: "system", Content: continuationPrompt},
			{Role: "user", Content: contextText},
		},
	}

	resp, err := client.Chat(context.Background(), req)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	jsonStr := extractJSONBlock(content)
	if jsonStr == "" {
		jsonStr = content // try parsing the whole thing
	}

	var artifact ContinuationArtifact
	if err := json.Unmarshal([]byte(jsonStr), &artifact); err != nil {
		return nil, fmt.Errorf("failed to parse continuation JSON: %w", err)
	}
	artifact.GeneratedAt = time.Now().UTC()
	return &artifact, nil
}

// pruneMarker is the stub content left behind when a tool result is pruned.
const pruneMarker = "[tool result pruned — re-run tool if needed]"

// --- Legacy helpers still used by the budget-aware path ---

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

func extractToolCallKey(toolName, argsJSON string) string {
	if argsJSON == "" {
		return ""
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
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
	if msg.Role != "user" && msg.Role != "system" {
		return false
	}
	trimmed := strings.TrimSpace(msg.Content)
	return strings.HasPrefix(trimmed, nativeCompactionSummaryPrefix) ||
		strings.HasPrefix(trimmed, nativeCompactionContinuationPrefix)
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
// positions in the message list.
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
