---
name: company-research
description: Deep company research using Exa and Apollo — competitive analysis, funding, product, traction signals
license: MIT
compatibility: toc-native
metadata:
  version: "0.1"
  requires_integration:
    - exa
    - apollo
---

# Company Research

Given a company name or URL, produce a structured research brief. Use Apollo enrichment for structured data and Exa search for recent, qualitative information.

## How to research

1. **Enrich with Apollo** (if available) using `organizations.enrich` with the company's domain:
   - Founded year, HQ location, headcount
   - Industry, keywords, short description
   - Total funding, latest round date, funding stage
   - Technology stack
   - LinkedIn, Twitter, blog URLs

   This gives you the factual foundation. Fill in what Apollo returns directly into the output format — don't re-research what you already have.

2. **Search with Exa** to fill gaps and find qualitative signal:
   - `"[company name] funding round"` — details Apollo doesn't have (investors, narrative)
   - `"[company name] product launch"` — recent product moves
   - `"[company name]"` + domain search — their own blog, announcements
   - Search for the founder names — talks, interviews, tweets

3. **Check their website** directly for:
   - Product pages (what they sell, how they position it)
   - Pricing page (model, tiers, price points)
   - About/team page (key hires beyond what Apollo shows)
   - Blog (recent posts reveal strategy direction)

4. **Synthesize into the output format below.**

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
