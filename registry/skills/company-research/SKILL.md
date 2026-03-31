---
name: company-research
description: Deep company research using Exa — competitive analysis, funding, product, traction signals
license: MIT
compatibility: toc-native
metadata:
  version: "0.1"
  requires_integration:
    - exa
---

# Company Research

Given a company name or URL, produce a structured research brief. Use Exa search to find recent, relevant information.

## How to research

1. **Search with Exa** for the company name + relevant queries:
   - `"[company name] funding round"` — funding history
   - `"[company name] product launch"` — recent product moves
   - `"[company name]"` + domain search — their own blog, announcements
   - Search for the founder names — talks, interviews, tweets

2. **Check their website** directly for:
   - Product pages (what they sell, how they position it)
   - Pricing page (model, tiers, price points)
   - About/team page (size, key hires)
   - Blog (recent posts reveal strategy direction)

3. **Synthesize into the output format below.**

## Output format

Save to `research/companies/YYYY-MM-DD-company-name.md`:

```markdown
# [Company Name]

**URL:** [website]
**Founded:** [year]
**HQ:** [location]
**Team size:** [approximate]

## What they do
[2-3 sentences. What the product does, in plain language.]

## Positioning
[How they describe themselves. What category they play in. Who they say they're for.]

## Product
[Core features, pricing model, recent launches or changes.]

## Funding
[Rounds raised, investors, amounts. Timeline.]

## Traction signals
[Whatever public evidence exists: traffic estimates, app store reviews, social following, hiring velocity, customer logos.]

## Recent moves
[Last 6 months: product launches, partnerships, blog posts that signal strategy, key hires, layoffs.]

## Strengths
[What they do well. Be honest.]

## Weaknesses
[Where they're vulnerable. What they're not doing.]

## Relevance
[Why this matters for the founder's business. How does this company relate to what they're building?]
```

## Guidelines

- Be factual. Cite sources where possible.
- Don't speculate beyond what the evidence supports.
- "I couldn't find information on X" is better than making something up.
- Focus on what's actionable for the founder — not an exhaustive company profile.
