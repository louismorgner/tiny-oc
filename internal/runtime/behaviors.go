package runtime

import (
	"slices"
	"strings"

	"github.com/tiny-oc/toc/internal/agent"
	tocsync "github.com/tiny-oc/toc/internal/sync"
)

// BehaviorCondition defines the matching criteria for a behavior.
// When multiple fields are set, all must match (AND semantics).
//
// Conditions are evaluated against a scoped working set (pendingBehaviorChanges)
// that accumulates tool activity since the last behavior fired, not the full
// session working set. The scoped set resets each time a behavior fires or when
// the session loop returns to the caller.
//
// Because the working set deduplicates tool names, ToolUsed means "this tool
// was invoked at least once since the last behavior evaluation" — not "this
// tool was used in the most recent model turn."
type BehaviorCondition struct {
	FileWritten string `json:"file_written,omitempty"`
	FileEdited  string `json:"file_edited,omitempty"`
	ToolUsed    string `json:"tool_used,omitempty"`
}

// Behavior is a resolved, runtime-ready behavior definition. The On field
// describes the lifecycle event that activates evaluation (e.g. "turn_complete").
type Behavior struct {
	Name   string            `json:"name"`
	On     string            `json:"on,omitempty"`
	When   BehaviorCondition `json:"when"`
	Prompt string            `json:"prompt,omitempty"`
}

func resolveBehaviors(cfg []agent.BehaviorConfig) []Behavior {
	if len(cfg) == 0 {
		return nil
	}

	behaviors := make([]Behavior, 0, len(cfg))
	for _, b := range cfg {
		on := strings.TrimSpace(b.On)
		if on == "" {
			on = "turn_complete"
		}
		behaviors = append(behaviors, Behavior{
			Name: b.Name,
			On:   on,
			When: BehaviorCondition{
				FileWritten: b.When.FileWritten,
				FileEdited:  b.When.FileEdited,
				ToolUsed:    b.When.ToolUsed,
			},
			Prompt: b.Prompt,
		})
	}
	return behaviors
}

// evaluateBehaviors walks behaviors in declaration order and returns the first
// matching turn_complete behavior that has not already fired in this prompt run.
func evaluateBehaviors(changes *WorkingSet, behaviors []Behavior, fired map[string]bool) *Behavior {
	if changes == nil || len(behaviors) == 0 {
		return nil
	}

	for i := range behaviors {
		b := &behaviors[i]
		if b.On != "turn_complete" {
			continue
		}
		if fired[b.Name] || !b.matches(changes) {
			continue
		}
		return b
	}
	return nil
}

func (b Behavior) matches(changes *WorkingSet) bool {
	if changes == nil {
		return false
	}

	matched := false
	if pattern := strings.TrimSpace(b.When.FileWritten); pattern != "" {
		matched = true
		if !matchesAnyPath(changes.FilesWritten, pattern) {
			return false
		}
	}
	if pattern := strings.TrimSpace(b.When.FileEdited); pattern != "" {
		matched = true
		if !matchesAnyPath(changes.FilesEdited, pattern) {
			return false
		}
	}
	if toolName := strings.TrimSpace(b.When.ToolUsed); toolName != "" {
		matched = true
		if !slices.Contains(changes.ToolsUsed, toolName) {
			return false
		}
	}
	return matched
}

func matchesAnyPath(paths []string, pattern string) bool {
	for _, path := range paths {
		if tocsync.MatchOne(path, pattern) {
			return true
		}
	}
	return false
}
