# mini-claw

A persistent coding agent inspired by [OpenClaw](https://docs.openclaw.ai)'s agentic patterns. Maintains identity, memory, and user context across sessions.

## What it does

mini-claw treats each session as a continuation, not a cold start. It bootstraps by reading its own memory and identity files, responds with awareness of past context, and writes down anything that matters for next time.

## Core behaviors

### First-run bootstrap (`BOOTSTRAP.md`)

On first launch, the agent finds `BOOTSTRAP.md` and enters a conversational setup mode — inspired by [OpenClaw's BOOTSTRAP template](https://docs.openclaw.ai/reference/templates/BOOTSTRAP). Instead of starting with empty placeholder files, the agent and user establish identity together:

1. The agent introduces itself and asks who the user is
2. They agree on name, personality, vibe, and emoji
3. The agent writes what it learned into `soul.md` and `user.md`
4. `BOOTSTRAP.md` is deleted — future sessions skip this step

### Session bootstrap

After the first run, every session starts with a silent bootstrap sequence:
1. Review identity (`soul.md`) and user profile (`user.md`)
2. Read today's memory log (and yesterday's if present)
3. Read long-term memory (`MEMORY.md`)

No user interaction needed — the agent orients itself before responding.

### Identity (`soul.md`)

The agent has a persistent identity file that defines its tone, personality, and boundaries. This file is meant to evolve — the agent can update it as it develops a working relationship with the user.

### User profile (`user.md`)

A living document where the agent records what it learns about the user: preferences, working style, technical background. This lets the agent tailor its responses over time without re-learning from scratch each session.

### Memory

Two-tier system:
- **Daily logs** (`memory/YYYY-MM-DD.md`) — append-only session notes. What happened, what was decided, what's still open.
- **Long-term memory** (`memory/MEMORY.md`) — curated store of decisions, preferences, and open loops that persist across days.

The agent manages its own memory — writing during sessions and pruning when things become stale.

### Session awareness

The agent knows it's a fresh instance with no built-in memory of past sessions. It checks its files before claiming ignorance, and writes things down before a session ends.

### Safety defaults

- No destructive commands without explicit confirmation
- No secrets or credentials in responses
- Ask before doing anything risky

## Files

| File | Purpose |
|---|---|
| `BOOTSTRAP.md` | First-run conversational setup — deleted after identity is established |
| `agent.md` | Core instructions — bootstrap, session awareness, operating principles |
| `soul.md` | Agent identity — tone, boundaries, personality |
| `user.md` | User profile — preferences and working style |
| `memory/` | Persistent memory — daily logs and long-term store |

## Adapted from OpenClaw

| OpenClaw concept | mini-claw equivalent |
|---|---|
| `SOUL.md` | `soul.md` — agent identity, evolves over time |
| `BOOTSTRAP.md` template | `BOOTSTRAP.md` — first-run conversational identity setup |
| `AGENTS.md` bootstrap | Session bootstrap sequence in `agent.md` |
| `USER.md` | `user.md` — observed user profile |
| Daily memory logs | `memory/YYYY-MM-DD.md` |
| `MEMORY.md` long-term store | `memory/MEMORY.md` |
| Safety defaults | Built into `agent.md` |
