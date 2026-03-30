package runtime

import "testing"

func TestEvaluateBehaviors_MatchesChangedFilesAndTools(t *testing.T) {
	changes := &WorkingSet{
		FilesWritten: []string{"writing/post.md"},
		FilesEdited:  []string{"voice.md"},
		ToolsUsed:    []string{"Write", "Edit"},
	}
	behaviors := []Behavior{
		{
			Name: "voice-review",
			On:   "turn_complete",
			When: BehaviorCondition{
				FileWritten: "writing/*.md",
				FileEdited:  "voice.md",
				ToolUsed:    "Write",
			},
			Prompt: "Review the draft.",
		},
	}

	got := evaluateBehaviors(changes, behaviors, map[string]bool{})
	if got == nil || got.Name != "voice-review" {
		t.Fatalf("evaluateBehaviors() = %#v, want voice-review", got)
	}
}

func TestEvaluateBehaviors_SkipsFiredAndUnmatched(t *testing.T) {
	changes := &WorkingSet{
		FilesWritten: []string{"notes/todo.txt"},
		ToolsUsed:    []string{"Write"},
	}
	behaviors := []Behavior{
		{
			Name:   "already-fired",
			On:     "turn_complete",
			When:   BehaviorCondition{ToolUsed: "Write"},
			Prompt: "Should not run again.",
		},
		{
			Name:   "no-match",
			On:     "turn_complete",
			When:   BehaviorCondition{FileWritten: "writing/*.md"},
			Prompt: "Should not match.",
		},
	}

	if got := evaluateBehaviors(changes, behaviors, map[string]bool{"already-fired": true}); got != nil {
		t.Fatalf("evaluateBehaviors() = %#v, want nil", got)
	}
}

func TestEvaluateBehaviors_SkipsNonTurnComplete(t *testing.T) {
	changes := &WorkingSet{
		ToolsUsed: []string{"Write"},
	}
	behaviors := []Behavior{
		{
			Name:   "on-stop",
			On:     "session_end",
			When:   BehaviorCondition{ToolUsed: "Write"},
			Prompt: "Should be skipped during turn evaluation.",
		},
	}

	if got := evaluateBehaviors(changes, behaviors, map[string]bool{}); got != nil {
		t.Fatalf("evaluateBehaviors() = %#v, want nil (wrong lifecycle event)", got)
	}
}

func TestEvaluateBehaviors_ReturnsFirstMatchInDeclarationOrder(t *testing.T) {
	changes := &WorkingSet{
		FilesWritten: []string{"writing/post.md"},
	}
	behaviors := []Behavior{
		{
			Name:   "first",
			On:     "turn_complete",
			When:   BehaviorCondition{FileWritten: "writing/*.md"},
			Prompt: "First match.",
		},
		{
			Name:   "second",
			On:     "turn_complete",
			When:   BehaviorCondition{FileWritten: "writing/*.md"},
			Prompt: "Second match.",
		},
	}

	got := evaluateBehaviors(changes, behaviors, map[string]bool{})
	if got == nil || got.Name != "first" {
		t.Fatalf("evaluateBehaviors() = %#v, want first", got)
	}
}
