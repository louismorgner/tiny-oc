---
name: person-research
description: Meeting prep — research a person before a conversation using Exa and Apollo
license: MIT
compatibility: toc-native
metadata:
  version: "0.1"
  requires_integration: exa
---

# Person Research

Given a person's name (and optionally company/role), produce a concise meeting prep brief. The goal is a 5-minute read before a meeting that gives the founder useful context.

## How to research

1. **Search with Exa** for:
   - `"[name]" "[company]"` — general coverage
   - `"[name]" podcast OR interview OR talk` — their own words on things they care about
   - `"[name]" blog OR writing` — their published thinking

2. **Use Apollo** (if available) to enrich:
   - Current role, company, title
   - Career history
   - Contact information

3. **Check their public profiles:**
   - Twitter/X — recent posts reveal what they're thinking about *right now*
   - LinkedIn — career trajectory, mutual connections
   - Personal blog/website — deepest signal of what they care about

4. **Synthesize into the output format below.**

## Output format

Save to `research/people/YYYY-MM-DD-person-name.md`:

```markdown
# [Person Name]

**Current role:** [Title at Company]
**Location:** [City]

## Background
[Career trajectory in 3-4 sentences. Where they've been, how they got to where they are.]

## What they care about
[Based on their writing, talks, and recent activity. What topics do they engage with? What's their thesis or worldview?]

## Recent activity
[Last 3-6 months: talks, blog posts, tweets, investments, job changes. What signals their current thinking?]

## For investors
[Only if the person is an investor]
- **Fund:** [name, size if known]
- **Stage:** [what stages they invest at]
- **Thesis:** [what they look for]
- **Recent investments:** [notable recent deals]
- **Check size:** [typical range]

## For customers/partners
[Only if the person is a potential customer or partner]
- **Company context:** [what their company does, size, stage]
- **Their role:** [what they own, what they care about]
- **Likely pain points:** [based on role and company context]

## Mutual context
[Shared connections, interests, or experiences that could be relevant for the conversation.]

## Conversation angles
[2-3 specific things the founder could reference or ask about to build rapport or advance the conversation.]
```

## Guidelines

- Optimize for signal. A 1-page brief with 5 useful facts beats a 5-page brief with 50 generic ones.
- Recent > historical. What someone did last month matters more than what they did 5 years ago.
- Don't include information that's obvious or unhelpful ("they have a LinkedIn profile").
- If you can't find much, say so. A short brief with honest gaps is better than a padded one.
