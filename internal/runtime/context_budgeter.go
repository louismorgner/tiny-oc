package runtime

import "github.com/tiny-oc/toc/internal/runtimeinfo"

// BudgetDecision is the action the runtime should take based on context usage.
type BudgetDecision string

const (
	BudgetContinue BudgetDecision = "continue" // context fits comfortably
	BudgetPrune    BudgetDecision = "prune"    // age/prune stale tool outputs
	BudgetCompact  BudgetDecision = "compact"  // full compaction needed
	BudgetFail     BudgetDecision = "fail"     // context is irrecoverably full
)

// ContextBudgeter makes token-budget-aware decisions about when to prune,
// compact, or fail, replacing the old character-threshold heuristic.
type ContextBudgeter struct {
	ContextWindow  int // total model context window in tokens
	MaxOutput      int // max output tokens reserved for model response
	ReservedBuffer int // overhead for tool definitions, system framing
}

// NewContextBudgeter creates a budgeter from a model profile. If the profile
// has zero values (custom/unknown model), conservative defaults are used.
func NewContextBudgeter(profile runtimeinfo.NativeModelProfile) *ContextBudgeter {
	ctx := profile.ContextWindow
	if ctx <= 0 {
		ctx = 128000
	}
	maxOut := profile.MaxOutputTokens
	if maxOut <= 0 {
		maxOut = 8192
	}
	reserved := profile.ReservedBuffer
	if reserved <= 0 {
		reserved = 4096
	}
	return &ContextBudgeter{
		ContextWindow:  ctx,
		MaxOutput:      maxOut,
		ReservedBuffer: reserved,
	}
}

// InputBudget returns the maximum number of input tokens available for
// message history (context window minus output reservation minus buffer).
func (b *ContextBudgeter) InputBudget() int {
	// Guard: MaxOutput must not exceed the context window (misconfigured profiles
	// would produce a negative budget and silently floor sessions at 1024).
	maxOut := b.MaxOutput
	if maxOut > b.ContextWindow {
		maxOut = b.ContextWindow / 2
	}
	budget := b.ContextWindow - maxOut - b.ReservedBuffer
	if budget < 1024 {
		return 1024
	}
	return budget
}

// PruneThreshold returns the token count at which tool output aging should
// begin (75% of the input budget).
func (b *ContextBudgeter) PruneThreshold() int {
	return b.InputBudget() * 3 / 4
}

// CompactThreshold returns the token count at which full compaction is
// triggered (90% of the input budget).
func (b *ContextBudgeter) CompactThreshold() int {
	return b.InputBudget() * 9 / 10
}

// FailThreshold returns the token count beyond which the request will
// likely be rejected by the provider (98% of input budget).
func (b *ContextBudgeter) FailThreshold() int {
	return b.InputBudget() * 98 / 100
}

// Evaluate decides what action the runtime should take given the current
// estimated input token count.
func (b *ContextBudgeter) Evaluate(estimatedInputTokens int) BudgetDecision {
	if estimatedInputTokens >= b.FailThreshold() {
		return BudgetFail
	}
	if estimatedInputTokens >= b.CompactThreshold() {
		return BudgetCompact
	}
	if estimatedInputTokens >= b.PruneThreshold() {
		return BudgetPrune
	}
	return BudgetContinue
}
