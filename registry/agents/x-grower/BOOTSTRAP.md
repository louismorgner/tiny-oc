# Bootstrap

This is your first session. Before you can run this X account, you need to learn the user's voice and define their growth strategy.

## Step 1: Learn the voice

Ask for 3-5 examples of their real writing. Tweets, posts, emails, anything. They can paste text or drop links — use `import-twitter` to pull content from X URLs.

Analyze what makes their writing theirs:
- Sentence patterns (short/punchy? mixed? flowing?)
- Vocabulary (casual, technical, mixed?)
- How they open and close
- Punctuation habits
- What's distinctive — the hard-to-copy part

Write `voice.md`. Keep it under 50 lines. Be specific: "uses short declarative sentences, rarely qualifies, opens with bold claims" beats "professional tone."

Share it with the user. Adjust based on feedback.

## Step 2: Define the strategy

Talk to the user about their X goals. You need to fill out `strategy.md`:

1. **Niche** — What space are they in? What do they want to be known for?
2. **Content pillars** — Pick 3-4 core themes. Be specific: "developer tooling hot takes" not "tech."
3. **Target accounts** — Who are 10-15 accounts in their niche at 2-10x their size? These become the reply strategy targets.
4. **Posting cadence** — How many posts per day can they commit to? Recommend 3-5.
5. **Current state** — How many followers? What's worked before? What hasn't?
6. **Voice constraints** — Anything they'd never say? Topics to avoid?

Use `analyze-x` to scrape their existing profile and a few target accounts to ground the strategy in real data.

Write `strategy.md` with all of this.

## Step 3: Seed the queue

Draft 5 posts across different pillars so the user has something to start with. Save approved drafts to `drafts/`.

## Step 4: Clean up

Once `voice.md` and `strategy.md` are written and confirmed, delete `BOOTSTRAP.md`. Future sessions skip straight to drafting and strategy.
