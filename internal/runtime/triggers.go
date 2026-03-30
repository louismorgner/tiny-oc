package runtime

import (
	"strings"

	"github.com/tiny-oc/toc/internal/agent"
	tocsync "github.com/tiny-oc/toc/internal/sync"
)

// TriggerCondition defines the matching criteria for a post-turn trigger.
// When multiple fields are set, all must match (AND semantics).
//
// Conditions are evaluated against a scoped working set (pendingTriggerChanges)
// that accumulates tool activity since the last trigger fired, not the full
// session working set. The scoped set resets each time a trigger fires or when
// the session loop returns to the caller.
//
// Because the working set deduplicates tool names, ToolUsed means "this tool
// was invoked at least once since the last trigger evaluation" — not "this
// tool was used in the most recent model turn."
type TriggerCondition struct {
	FileWritten string `json:"file_written,omitempty"`
	FileEdited  string `json:"file_edited,omitempty"`
	ToolUsed    string `json:"tool_used,omitempty"`
}

type TurnTrigger struct {
	Name   string           `json:"name"`
	When   TriggerCondition `json:"when"`
	Prompt string           `json:"prompt,omitempty"`
}

func resolveTurnTriggers(cfg []agent.TriggerConfig) []TurnTrigger {
	if len(cfg) == 0 {
		return nil
	}

	triggers := make([]TurnTrigger, 0, len(cfg))
	for _, trigger := range cfg {
		triggers = append(triggers, TurnTrigger{
			Name: trigger.Name,
			When: TriggerCondition{
				FileWritten: trigger.When.FileWritten,
				FileEdited:  trigger.When.FileEdited,
				ToolUsed:    trigger.When.ToolUsed,
			},
			Prompt: trigger.Prompt,
		})
	}
	return triggers
}

func evaluateTriggers(changes *WorkingSet, triggers []TurnTrigger, fired map[string]bool) *TurnTrigger {
	if changes == nil || len(triggers) == 0 {
		return nil
	}

	for i := range triggers {
		trigger := &triggers[i]
		if fired[trigger.Name] || !trigger.matches(changes) {
			continue
		}
		return trigger
	}
	return nil
}

func (t TurnTrigger) matches(changes *WorkingSet) bool {
	if changes == nil {
		return false
	}

	matched := false
	if pattern := strings.TrimSpace(t.When.FileWritten); pattern != "" {
		matched = true
		if !matchesAnyPath(changes.FilesWritten, pattern) {
			return false
		}
	}
	if pattern := strings.TrimSpace(t.When.FileEdited); pattern != "" {
		matched = true
		if !matchesAnyPath(changes.FilesEdited, pattern) {
			return false
		}
	}
	if toolName := strings.TrimSpace(t.When.ToolUsed); toolName != "" {
		matched = true
		if !containsString(changes.ToolsUsed, toolName) {
			return false
		}
	}
	return matched
}

func matchesAnyPath(paths []string, pattern string) bool {
	for _, path := range paths {
		if tocsync.MatchesAny(path, []string{pattern}) {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
