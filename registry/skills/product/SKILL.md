---
name: product
description: Product thinking — PMF assessment, user research, prioritization, MVP scoping, and build decisions
license: MIT
compatibility: toc-native
metadata:
  version: "0.1"
  requires_integration: none
---

# Product

Product is the core of what a startup does. Not "product management" as a discipline — the act of figuring out what to build, for whom, and whether it's working.

## Product-market fit

PMF is the only thing that matters until you have it. Everything else — hiring, process, growth — is premature without it.

### How to know if you have it

**You have PMF when:**
- Users are pulling the product from you (inbound demand, word of mouth)
- Retention curves flatten — people who start using it keep using it
- Sean Ellis test: 40%+ of users say they'd be "very disappointed" if the product went away
- You can't keep up with demand
- Usage grows without you pushing it

**You don't have PMF when:**
- Growth is entirely founder-driven (your personal network, manual outreach)
- People try it once and don't come back
- You're adding features hoping one will stick
- The value prop changes every time you pitch it
- You're spending more on acquisition than retention

### What to do when you don't have it

1. **Narrow down.** You're probably trying to serve too many people. Find the 5 users who love it most. What do they have in common? Build only for them.
2. **Talk to churned users.** Not happy users — churned users. They'll tell you what's missing.
3. **Measure retention, not signups.** Signups are vanity. Week-4 retention tells you the truth.
4. **Ship faster.** If you're not embarrassed by v1, you launched too late. The goal is learning speed, not polish.

## User research

### The Mom Test (Rob Fitzpatrick)

The foundational framework. Three rules:

1. **Talk about their life, not your idea.** "Tell me about how you handle X" > "Would you use a product that does Y?"
2. **Ask about specifics in the past, not generics about the future.** "Walk me through the last time this happened" > "How often does this happen?"
3. **Talk less, listen more.** If you're talking more than 20% of the time, you're pitching, not learning.

### What to ask

- "What's the hardest part about [doing this thing]?"
- "Tell me about the last time you encountered this problem."
- "What solutions have you tried? What worked and what didn't?"
- "If you could wave a magic wand, what would change?"
- "How much time/money does this cost you today?"

### What not to ask

- "Would you use this?" (Everyone says yes. It means nothing.)
- "How much would you pay?" (People can't predict their own purchasing behavior.)
- "Do you like this feature?" (Opinions ≠ behavior.)

### When to do research vs ship

**Research when:** You don't know what to build, or something isn't working and you don't know why.
**Ship when:** You have a hypothesis and need to test it with real usage, not opinions.

Don't research forever. 10-15 conversations is enough to see patterns. Then build and measure.

## Prioritization

### What to build next

The question is always: **What is the single most important thing we can do in the next 1-2 weeks to learn whether we're on the right track?**

Not: what's in the backlog. Not: what did a customer request. Not: what's the roadmap.

### The ICE framework (when you need to compare)

- **Impact:** If this works, how much does it move the metric that matters?
- **Confidence:** How sure are we that it will work? (Based on user conversations, data, or just gut?)
- **Ease:** How long will it take to build and ship?

Score each 1-10, multiply. Do the highest score first. The whole exercise takes 15 minutes.

### What not to build

- Features only one customer asked for (unless they're your ideal customer)
- "Nice to have" features before the core value is nailed
- Anything that makes the product more complex without making it more valuable
- Internal tools and infrastructure (pre-PMF) unless they're truly blocking you
- Competitive features ("Competitor X has this so we need it too")

## MVP scoping

An MVP is not a crappy version of your product. It's the smallest thing that tests your riskiest assumption.

### How to scope

1. **Identify your riskiest assumption.** What has to be true for this to work? Usually: "People have this problem" or "People will pay for this solution" or "We can deliver this value."
2. **Build only what tests that assumption.** Nothing else. Cut every feature that doesn't directly answer the question.
3. **Set a success metric before you build.** "If X users do Y in the first week, the assumption is validated." Without this, you'll rationalize any result.

### Signs your MVP is too big

- It takes more than 2-4 weeks to build
- You're discussing edge cases before the happy path works
- The feature list keeps growing
- You're building the "right" version instead of the "fast" version

## Build vs buy vs integrate

- **Build** when it's core to your value prop or no existing solution fits
- **Buy/integrate** when it's not your core value and a good tool exists
- **Hack it manually** when you have < 100 users — use spreadsheets, manual processes, Zapier. Don't build internal tools pre-PMF.

## Pivot decisions

### When to pivot

- You've talked to 30+ users and can't find a segment that loves the product
- Retention is flat or declining despite multiple iterations
- You've been working on the same core idea for 6+ months with no organic traction
- The market you're going after is smaller than you thought

### When not to pivot

- You haven't shipped yet (you haven't tested anything)
- A few users love it but growth is slow (that's a distribution problem, not a product problem)
- It's hard (startups are hard — that's not a reason to pivot)

### How to pivot

A good pivot preserves something you've learned — a customer segment, a technology, an insight. A bad pivot is starting completely from scratch.

## Metrics that matter

### Pre-PMF
- Retention (week 1, week 4, week 8)
- Usage frequency (are people coming back without prompting?)
- NPS/Sean Ellis score from your best users
- Qualitative: are users pulling the product from you?

### Post-PMF
- Revenue growth (MRR, ARR)
- Unit economics (CAC, LTV, payback period)
- Churn (monthly, annual — by cohort)
- Net revenue retention (expansion vs contraction)

### Always avoid
- Vanity metrics: total signups, page views, downloads
- Metrics you can't act on
- Metrics that go up when you spend money and down when you stop
