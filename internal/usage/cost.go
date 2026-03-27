package usage

// ModelPricing holds per-token costs in USD for a model on OpenRouter.
type ModelPricing struct {
	InputPerToken      float64 // cost per input token
	OutputPerToken     float64 // cost per output token
	CacheReadPerToken  float64 // cost per cached input token (cache hit)
	CacheWritePerToken float64 // cost per cache write token (same as input if not discounted)
}

// modelPricing maps OpenRouter model IDs to their per-token pricing.
// Pricing sourced from https://openrouter.ai/models.
var modelPricing = map[string]ModelPricing{
	"openai/gpt-4o-mini": {
		InputPerToken:      0.15 / 1_000_000,
		OutputPerToken:     0.60 / 1_000_000,
		CacheReadPerToken:  0.075 / 1_000_000,
		CacheWritePerToken: 0.15 / 1_000_000,
	},
	"openai/gpt-4o": {
		InputPerToken:      2.50 / 1_000_000,
		OutputPerToken:     10.00 / 1_000_000,
		CacheReadPerToken:  1.25 / 1_000_000,
		CacheWritePerToken: 2.50 / 1_000_000,
	},
	"anthropic/claude-sonnet-4": {
		InputPerToken:      3.00 / 1_000_000,
		OutputPerToken:     15.00 / 1_000_000,
		CacheReadPerToken:  0.30 / 1_000_000,
		CacheWritePerToken: 3.75 / 1_000_000,
	},
	"openai/gpt-5.4": {
		InputPerToken:      2.50 / 1_000_000,
		OutputPerToken:     15.00 / 1_000_000,
		CacheReadPerToken:  0.25 / 1_000_000,
		CacheWritePerToken: 2.50 / 1_000_000,
	},
	"openai/gpt-5.3-codex": {
		InputPerToken:      1.75 / 1_000_000,
		OutputPerToken:     14.00 / 1_000_000,
		CacheReadPerToken:  1.75 / 1_000_000, // no cache discount listed
		CacheWritePerToken: 1.75 / 1_000_000,
	},
}

// EstimateCost returns the estimated USD cost for the given token usage
// on the specified model. Returns 0 if the model has no pricing data.
func EstimateCost(model string, tokens TokenUsage) float64 {
	pricing, ok := modelPricing[model]
	if !ok {
		return 0
	}
	// InputTokens already includes CacheRead and CacheCreate as subsets
	// (OpenAI/OpenRouter report prompt_tokens inclusive of cached tokens).
	// Charge only the non-cached portion at the full input rate.
	nonCached := tokens.InputTokens - tokens.CacheRead - tokens.CacheCreate
	cost := float64(nonCached) * pricing.InputPerToken
	cost += float64(tokens.OutputTokens) * pricing.OutputPerToken
	cost += float64(tokens.CacheRead) * pricing.CacheReadPerToken
	cost += float64(tokens.CacheCreate) * pricing.CacheWritePerToken
	return cost
}

// LookupPricing returns the pricing for a model, if known.
func LookupPricing(model string) (ModelPricing, bool) {
	p, ok := modelPricing[model]
	return p, ok
}
