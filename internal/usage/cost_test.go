package usage

import (
	"math"
	"testing"
)

func TestEstimateCostKnownModel(t *testing.T) {
	tokens := TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 100_000,
		CacheRead:    500_000,
		CacheCreate:  50_000,
	}
	cost := EstimateCost("openai/gpt-5.4", tokens)
	if cost <= 0 {
		t.Fatalf("expected positive cost, got %f", cost)
	}
	// gpt-5.4: input=$2.50/M, output=$15/M, cache_read=$0.25/M, cache_write=$2.50/M
	// InputTokens (1M) already includes CacheRead (500k) and CacheCreate (50k).
	// non-cached: (1M - 500k - 50k) * 2.50/M = 1.125
	// output: 100k * 15/M = 1.50
	// cache_read: 500k * 0.25/M = 0.125
	// cache_write: 50k * 2.50/M = 0.125
	// total = 2.875
	expected := 2.875
	if math.Abs(cost-expected) > 0.001 {
		t.Fatalf("cost = %f, expected %f", cost, expected)
	}
}

func TestEstimateCostUnknownModel(t *testing.T) {
	tokens := TokenUsage{InputTokens: 1000, OutputTokens: 500}
	cost := EstimateCost("unknown/model", tokens)
	if cost != 0 {
		t.Fatalf("expected 0 for unknown model, got %f", cost)
	}
}

func TestEstimateCostCodex(t *testing.T) {
	tokens := TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	cost := EstimateCost("openai/gpt-5.3-codex", tokens)
	// input: 1M * 1.75/M = 1.75, output: 1M * 14/M = 14.00
	expected := 15.75
	if math.Abs(cost-expected) > 0.001 {
		t.Fatalf("cost = %f, expected %f", cost, expected)
	}
}

func TestEstimateCostClaudeSonnet46(t *testing.T) {
	tokens := TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 100_000,
		CacheRead:    500_000,
		CacheCreate:  50_000,
	}
	cost := EstimateCost("anthropic/claude-sonnet-4.6", tokens)
	// claude-sonnet-4.6: input=$3/M, output=$15/M, cache_read=$0.30/M, cache_write=$3.75/M
	// non-cached: (1M - 500k - 50k) * 3/M = 1.35
	// output: 100k * 15/M = 1.50
	// cache_read: 500k * 0.30/M = 0.15
	// cache_write: 50k * 3.75/M = 0.1875
	// total = 3.1875
	expected := 3.1875
	if math.Abs(cost-expected) > 0.001 {
		t.Fatalf("cost = %f, expected %f", cost, expected)
	}
}

func TestEstimateCostClaudeOpus46(t *testing.T) {
	tokens := TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 100_000,
		CacheRead:    500_000,
		CacheCreate:  50_000,
	}
	cost := EstimateCost("anthropic/claude-opus-4.6", tokens)
	// claude-opus-4.6: input=$15/M, output=$75/M, cache_read=$1.50/M, cache_write=$18.75/M
	// non-cached: (1M - 500k - 50k) * 15/M = 6.75
	// output: 100k * 75/M = 7.50
	// cache_read: 500k * 1.50/M = 0.75
	// cache_write: 50k * 18.75/M = 0.9375
	// total = 15.9375
	expected := 15.9375
	if math.Abs(cost-expected) > 0.001 {
		t.Fatalf("cost = %f, expected %f", cost, expected)
	}
}

func TestLookupPricing(t *testing.T) {
	if _, ok := LookupPricing("openai/gpt-5.4"); !ok {
		t.Fatal("expected pricing for gpt-5.4")
	}
	if _, ok := LookupPricing("openai/gpt-5.3-codex"); !ok {
		t.Fatal("expected pricing for gpt-5.3-codex")
	}
	if _, ok := LookupPricing("nonexistent"); ok {
		t.Fatal("expected no pricing for nonexistent model")
	}
}
