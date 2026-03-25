# BOOTSTRAP.md — First Session

*You're new to this project. Time to understand what you're working with.*

There's no memory yet and `project.md` is a blank template. Your job is to fill both in so that future sessions start with real context instead of assumptions.

## Step 1: Explore the project

Do this silently before talking to the user:

1. **Read the README** — what is this project, what does it do?
2. **Browse the directory structure** — `ls` the top-level and key subdirectories. Understand the layout.
3. **Check recent git history** — `git log --oneline -30`. Who's contributing? What's the pace? What's been worked on recently?
4. **Identify the stack** — languages, frameworks, build tools, test setup. Look at package files, configs, CI.
5. **Look for conventions** — linting config, commit message style, PR templates, existing CLAUDE.md or similar docs.

## Step 2: Talk to the user

Now introduce yourself and have a brief conversation to learn what you can't get from code:

- What stage is the project at? (prototype, early users, production, etc.)
- How big is the team? Who's working on what?
- What are the current priorities or constraints?
- Any architectural decisions already made that you should know about?
- How do they want you to operate? (autonomous vs. check-in-often, opinionated vs. flexible)

Don't interrogate — keep it conversational. If they just want to get to work, that's fine too. Observe and learn as you go.

## Step 3: Write it down

Update `project.md` with what you learned — both from exploring the code and talking to the user. This file becomes your persistent project context for every future session.

Start `memory/MEMORY.md` with any preferences or decisions worth remembering.

## Step 4: Clean up

Delete this file. You don't need a bootstrap script anymore — you know the project now.

Then proceed to help the user with whatever they need.
