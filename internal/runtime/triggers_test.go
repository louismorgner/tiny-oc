package runtime

import "testing"

func TestEvaluateTriggers_MatchesChangedFilesAndTools(t *testing.T) {
	changes := &WorkingSet{
		FilesWritten: []string{"writing/post.md"},
		FilesEdited:  []string{"voice.md"},
		ToolsUsed:    []string{"Write", "Edit"},
	}
	triggers := []TurnTrigger{
		{
			Name: "voice-review",
			When: TriggerCondition{
				FileWritten: "writing/*.md",
				FileEdited:  "voice.md",
				ToolUsed:    "Write",
			},
			Prompt: "Review the draft.",
		},
	}

	got := evaluateTriggers(changes, triggers, map[string]bool{})
	if got == nil || got.Name != "voice-review" {
		t.Fatalf("evaluateTriggers() = %#v, want voice-review", got)
	}
}

func TestEvaluateTriggers_SkipsFiredAndUnmatched(t *testing.T) {
	changes := &WorkingSet{
		FilesWritten: []string{"notes/todo.txt"},
		ToolsUsed:    []string{"Write"},
	}
	triggers := []TurnTrigger{
		{
			Name:   "already-fired",
			When:   TriggerCondition{ToolUsed: "Write"},
			Prompt: "Should not run again.",
		},
		{
			Name:   "no-match",
			When:   TriggerCondition{FileWritten: "writing/*.md"},
			Prompt: "Should not match.",
		},
	}

	if got := evaluateTriggers(changes, triggers, map[string]bool{"already-fired": true}); got != nil {
		t.Fatalf("evaluateTriggers() = %#v, want nil", got)
	}
}
