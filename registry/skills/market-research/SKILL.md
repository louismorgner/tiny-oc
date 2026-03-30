---
name: market-research
description: Market research — sizing, landscape, trends, and competitive mapping using Exa
license: MIT
compatibility: toc-native
metadata:
  version: "0.1"
  requires_integration: exa
---

# Market Research

Structured market analysis for a given space. Not top-down "the market is $50B" guesswork — bottoms-up sizing grounded in real data.

## How to research

1. **Define the scope** with the founder. What market are we analyzing? Be specific — "developer tools" is too broad. "CI/CD tools for teams under 50 engineers" is actionable.

2. **Search with Exa** for:
   - `"[market/category] market size"` — analyst estimates (use as cross-reference, not source of truth)
   - `"[market/category] startups"` — who's playing here
   - `"[market/category] trends 2024 2025"` — what's changing
   - `"[specific competitor] funding"` — what investors are betting on in this space

3. **Build the market map.** Who are the players? How do they segment? Where are the gaps?

4. **Bottom-up sizing.** Start with: how many potential customers exist × what they'd pay = addressable market.

5. **Synthesize into the output format below.**

## Output format

Save to `research/markets/YYYY-MM-DD-market-name.md`:

```markdown
# [Market Name]

**Date:** YYYY-MM-DD
**Scope:** [Specific definition of what market we're analyzing]

## Market sizing (bottom-up)

### TAM (Total Addressable Market)
[Everyone who could theoretically buy this, globally. Show your math.]

### SAM (Serviceable Addressable Market)
[The segment you can actually reach given your product, pricing, and distribution. Show your math.]

### SOM (Serviceable Obtainable Market)
[What you could realistically capture in 2-3 years. Show your math.]

**Method:** [How we calculated this. What data sources. What assumptions.]

## Market map

### Category leaders
[Who dominates today. Revenue/scale if known.]

### Funded startups
[Recent entrants with venture backing. What they're building, how much they've raised.]

### Adjacent players
[Companies that could enter this market. Why they might.]

## Trends

### What's changing
[Technology shifts, regulatory changes, behavioral changes that affect this market.]

### Tailwinds
[Forces making this market grow or become more accessible.]

### Headwinds
[Forces that could slow growth or make this harder.]

## Opportunities

### Gaps in the market
[What's underserved? Where are current solutions failing?]

### Wedge opportunities
[Specific entry points — a narrow use case or segment that's poorly served and could be a beachhead.]

## Key questions
[Open questions that would change the analysis. What would we need to learn?]
```

## Guidelines

- **Bottom-up > top-down.** "There are 200,000 companies in this segment × $5K avg deal size = $1B SAM" is credible. "The global market is $50B (Gartner)" is not useful.
- **Show your work.** Every number should have a source or a clear assumption. If you're estimating, say so.
- **Be honest about uncertainty.** Market sizing is inherently imprecise. Flag your confidence level.
- **Focus on actionable insight.** The founder needs to know: is this market worth pursuing, and where should we enter?
