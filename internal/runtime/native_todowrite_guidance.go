package runtime

import "strings"

const (
	todoWriteUseForMultiStep     = "Use the TodoWrite tool only when the task has multiple meaningful steps and tracking them will help you finish the work."
	todoWriteSkipShortSessions   = "Skip TodoWrite for single-answer requests, obvious one-step fixes, or short sessions where the overhead is higher than the value."
	todoWriteRewriteOnlyOnChange = "TodoWrite replaces the entire list on each call, so only rewrite it when the plan or statuses actually changed."
	todoWriteOneInProgress       = `Prefer only one item with status "in_progress" at a time and mark items complete immediately.`
)

var todoWriteRuntimeNotes = strings.Join([]string{
	todoWriteUseForMultiStep,
	todoWriteSkipShortSessions,
	todoWriteRewriteOnlyOnChange,
	todoWriteOneInProgress,
}, "\n")
