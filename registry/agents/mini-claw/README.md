# mini-claw

A persistent coding agent inspired by [OpenClaw](https://docs.openclaw.ai)'s agentic patterns. Maintains identity, memory, and user context across sessions.

## What's in this template

| File | Purpose |
|---|---|
| `oc-agent.yaml` | Agent config — model, skills, context sync patterns, compose files |
| `agent.md` | Core operating instructions — bootstrap, session awareness, safety defaults |
| `soul.md` | Agent identity — tone, boundaries, personality. Evolves over time. |
| `user.md` | User profile — preferences and working style, observed and recorded by the agent |
| `memory/` | Persistent memory — daily logs (`YYYY-MM-DD.md`) and long-term store (`MEMORY.md`) |

## How it works

### Compose

The `compose` field in `oc-agent.yaml` lists files to append after `agent.md` when building `CLAUDE.md` at spawn time. This means `soul.md` and `user.md` are injected directly into the agent's instructions — no "read this file" fragility.

### Template variables

`agent.md` uses template variables that get replaced at spawn time:

- `{{.AgentName}}` — the agent's name
- `{{.SessionID}}` — unique session identifier
- `{{.Date}}` — today's date (YYYY-MM-DD)
- `{{.Model}}` — the model being used

### Context sync

All identity and memory files are listed under `context:` in the config. This means changes the agent makes to `soul.md`, `user.md`, or anything in `memory/` are synced back to the agent template after each session. The agent accumulates knowledge over time.

### Memory (via `agentic-memory` skill)

Two-tier system:
- **Daily logs** (`memory/YYYY-MM-DD.md`) — session-level notes, append-only
- **Long-term memory** (`memory/MEMORY.md`) — curated decisions, preferences, open loops

## Install

```bash
toc registry install mini-claw
```

## OpenClaw concepts adapted

| OpenClaw | mini-claw |
|---|---|
| `SOUL.md` | `soul.md` — composed into CLAUDE.md + synced |
| `AGENTS.md` bootstrap sequence | `agent.md` with template vars + compose |
| `USER.md` | `user.md` — composed + synced |
| Daily memory logs | `memory/YYYY-MM-DD.md` via agentic-memory skill |
| `MEMORY.md` long-term store | `memory/MEMORY.md` via agentic-memory skill |
| Session isolation | toc session workspaces in /tmp |
| Safety defaults | Built into agent.md |
