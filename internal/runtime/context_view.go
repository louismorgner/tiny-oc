package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ContinuationArtifact is the structured output of compaction, replacing
// the old freeform bullet-point summary. It captures actionable state so
// the model can resume effectively after context is compacted.
type ContinuationArtifact struct {
	Goal          string   `json:"goal"`
	Constraints   []string `json:"constraints,omitempty"`
	Decisions     []string `json:"decisions,omitempty"`
	Discoveries   []string `json:"discoveries,omitempty"`
	WorkingFiles  []string `json:"working_files,omitempty"`
	CompletedWork []string `json:"completed_work,omitempty"`
	RemainingWork []string `json:"remaining_work,omitempty"`
	OpenLoops     []string `json:"open_loops,omitempty"`
	NextSteps     []string `json:"next_steps,omitempty"`
	GeneratedAt   time.Time `json:"generated_at,omitempty"`
}

// WorkingSet tracks files and commands the agent has interacted with during
// the session. It is updated incrementally as tool calls complete and fed
// into compaction and diagnostics.
type WorkingSet struct {
	FilesRead    []string `json:"files_read,omitempty"`
	FilesEdited  []string `json:"files_edited,omitempty"`
	FilesWritten []string `json:"files_written,omitempty"`
	RecentBash   []string `json:"recent_bash,omitempty"`
	SubAgents    []string `json:"sub_agents,omitempty"`
}

const maxWorkingSetEntries = 50

// UpdateFromToolCall updates the working set based on a completed tool call.
func (ws *WorkingSet) UpdateFromToolCall(toolName, argsJSON string) {
	if argsJSON == "" {
		return
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return
	}

	switch toolName {
	case "Read":
		if path, ok := args["file_path"].(string); ok && path != "" {
			ws.FilesRead = appendUnique(ws.FilesRead, path, maxWorkingSetEntries)
		}
	case "Edit":
		if path, ok := args["file_path"].(string); ok && path != "" {
			ws.FilesEdited = appendUnique(ws.FilesEdited, path, maxWorkingSetEntries)
		}
	case "Write":
		if path, ok := args["file_path"].(string); ok && path != "" {
			ws.FilesWritten = appendUnique(ws.FilesWritten, path, maxWorkingSetEntries)
		}
	case "Bash":
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			ws.RecentBash = appendCapped(ws.RecentBash, truncateInline(cmd, 120), maxWorkingSetEntries)
		}
	case "SubAgent":
		if action, ok := args["action"].(string); ok && action != "" {
			ws.SubAgents = appendUnique(ws.SubAgents, action, maxWorkingSetEntries)
		}
	}
}

// Summary returns a compact text representation for injection into context.
func (ws *WorkingSet) Summary() string {
	if ws == nil {
		return ""
	}
	var parts []string
	if len(ws.FilesEdited) > 0 {
		parts = append(parts, fmt.Sprintf("Edited: %s", strings.Join(ws.FilesEdited, ", ")))
	}
	if len(ws.FilesWritten) > 0 {
		parts = append(parts, fmt.Sprintf("Written: %s", strings.Join(ws.FilesWritten, ", ")))
	}
	if len(ws.FilesRead) > 0 {
		// Only show last few reads to keep it concise
		reads := ws.FilesRead
		if len(reads) > 10 {
			reads = reads[len(reads)-10:]
		}
		parts = append(parts, fmt.Sprintf("Read: %s", strings.Join(reads, ", ")))
	}
	if len(ws.RecentBash) > 0 {
		recent := ws.RecentBash
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		parts = append(parts, fmt.Sprintf("Recent commands: %s", strings.Join(recent, "; ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// ContextDiagnostics captures per-request context usage information for
// debugging and observability.
type ContextDiagnostics struct {
	EstimatedInputTokens int                `json:"estimated_input_tokens"`
	InputBudget          int                `json:"input_budget"`
	BudgetDecision       BudgetDecision     `json:"budget_decision"`
	TopContributors      []ContextContributor `json:"top_contributors,omitempty"`
	MessageCount         int                `json:"message_count"`
	PrunedCount          int                `json:"pruned_count,omitempty"`
	CompactionReason     string             `json:"compaction_reason,omitempty"`
	ContinuationAge      string             `json:"continuation_age,omitempty"`
}

// ContextContributor identifies a message or category consuming context.
type ContextContributor struct {
	Label          string `json:"label"`
	EstimatedTokens int   `json:"estimated_tokens"`
}

// BuildContextDiagnostics computes diagnostics for the current message state.
func BuildContextDiagnostics(messages []Message, budgeter *ContextBudgeter) ContextDiagnostics {
	totalTokens := estimateMessagesTokens(messages)
	decision := budgeter.Evaluate(totalTokens)

	diag := ContextDiagnostics{
		EstimatedInputTokens: totalTokens,
		InputBudget:          budgeter.InputBudget(),
		BudgetDecision:       decision,
		MessageCount:         len(messages),
	}

	// Compute top contributors by role/type
	contributors := map[string]int{}
	for _, msg := range messages {
		var label string
		switch msg.Role {
		case "system":
			label = "system"
		case "user":
			if isCompactionSummary(msg) {
				label = "continuation"
			} else {
				label = "user"
			}
		case "assistant":
			label = "assistant"
		case "tool":
			label = "tool:" + msg.Name
		default:
			label = msg.Role
		}
		tokens := estimateTokens(msg.Content) + perMessageOverhead
		for _, call := range msg.ToolCalls {
			tokens += estimateTokens(call.Function.Name) + estimateTokens(call.Function.Arguments) + perMessageOverhead
		}
		contributors[label] += tokens
	}

	for label, tokens := range contributors {
		diag.TopContributors = append(diag.TopContributors, ContextContributor{
			Label:           label,
			EstimatedTokens: tokens,
		})
	}

	return diag
}

// BuildContextView assembles the curated message slice sent to the model.
// This decouples what gets persisted (state.Messages) from what gets sent
// to the API, giving us a seam for working-set injection, continuation
// re-ranking, and other context shaping without mutating stored state.
func BuildContextView(state *State) []Message {
	if state == nil || len(state.Messages) == 0 {
		return nil
	}

	// Start with the current Messages (already compacted/pruned).
	view := make([]Message, 0, len(state.Messages)+1)

	for i, msg := range state.Messages {
		view = append(view, msg)

		// After the system prompt (or continuation artifact), inject a
		// working set summary if we have one. This gives the model
		// awareness of which files it has touched without consuming
		// much budget.
		if i == 0 && (msg.Role == "system" || isContinuationArtifact(msg) || isCompactionSummary(msg)) {
			if wsSummary := state.WorkingSet.Summary(); wsSummary != "" {
				view = append(view, Message{
					Role:    "user",
					Content: "[toc-working-set]\n" + wsSummary,
				})
			}
		}
	}

	return view
}

func appendUnique(slice []string, val string, maxLen int) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	slice = append(slice, val)
	if len(slice) > maxLen {
		slice = slice[len(slice)-maxLen:]
	}
	return slice
}

func appendCapped(slice []string, val string, maxLen int) []string {
	slice = append(slice, val)
	if len(slice) > maxLen {
		slice = slice[len(slice)-maxLen:]
	}
	return slice
}
