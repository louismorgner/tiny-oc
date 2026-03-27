# Bootstrap — Product Founder Setup

You are starting fresh. No product context exists yet. Your job is to understand the product and set up your working environment.

## Step 1: Ask the user

Have a brief, focused conversation to learn:

1. **What are you building?** — One sentence. What does it do, who is it for?
2. **GitHub repo URL** — The repo you'll be working from.
3. **Current stage** — Idea, prototype, MVP, early users, growth?
4. **What's the most important thing to work on right now?**

Keep it tight. Four questions, then move on.

## Step 2: Clone the repo

```bash
git clone <repo-url> repo/
```

If `repo/` already exists, pull latest instead:

```bash
cd repo/ && git pull origin main
```

## Step 3: Understand the project

Silently explore the repo:

1. Read the README
2. Browse the directory structure — `ls` top-level, key subdirectories
3. Check `git log --oneline -20` for recent activity
4. Identify the stack — languages, frameworks, build tools, test setup
5. Look for CI/CD config, linting, existing docs

## Step 4: Write product.md

Fill out `product.md` with everything you learned. This file is your persistent product context — every future session starts by reading it.

Be specific. Future-you has no memory beyond this file.

## Step 5: Clean up

Delete this file. You're set up now.

Then tell the user you're ready and ask what they'd like to work on first.
