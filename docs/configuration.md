# Configuration reference

## Workspace config

Created by `toc init` at `.toc/config.yaml`.

```yaml
name: my-project
```

| Field | Type | Description |
|---|---|---|
| `name` | string | Workspace name, set during `toc init` |

## Agent config

Each agent lives in `.toc/agents/<name>/oc-agent.yaml`.

```yaml
runtime: claude-code
name: pr-reviewer
description: Reviews pull requests for code quality
model: sonnet
context:
  - context/*.md
  - docs/
skills:
  - code-review
  - https://github.com/example/my-skill.git
```

### Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `runtime` | string | yes | — | Agent runtime. Currently only `claude-code` is supported |
| `name` | string | yes | — | Lowercase alphanumeric with hyphens (e.g. `pr-reviewer`). Must match `^[a-z0-9][a-z0-9-]*$` |
| `description` | string | no | — | Shown in `toc agent list` and shell tab completion |
| `model` | string | yes | — | Claude model: `sonnet`, `opus`, or `haiku` |
| `context` | list | no | — | Glob patterns for context sync (see below) |
| `skills` | list | no | — | Skill names (local) or Git URLs (remote) |

### Agent instructions

Each agent also has an `agent.md` file in the same directory. This is loaded as `CLAUDE.md` when a session is spawned, serving as the agent's system instructions.

You can put anything in `agent.md` that you'd put in a `CLAUDE.md` — task descriptions, constraints, output format requirements, tool usage rules, etc.

## Context sync patterns

The `context` field defines which files sync back from the isolated session directory to the agent template. This lets agents accumulate knowledge across sessions.

Patterns support:

| Pattern | Example | Matches |
|---|---|---|
| Standard glob | `*.md` | Any `.md` file (including in subdirectories) |
| Directory | `docs/` | Everything under `docs/` |
| Path glob | `context/*.md` | `.md` files directly in `context/` |
| Recursive glob | `docs/**/*.md` | `.md` files at any depth under `docs/` |

Sync happens at two points:

1. **Real-time** — a Claude Code `PostToolUse` hook fires on every `Edit`, `Write`, and `MultiEdit` tool call. If the written file matches a context pattern, it's immediately copied back to the agent template.
2. **Post-session** — when the session ends, a full sync pass runs as a safety net.

### How it works

When a session is spawned, toc generates:

- `.claude/toc-sync.sh` — a shell script that reads the PostToolUse hook payload and copies matching files
- `.claude/settings.json` — registers the sync script as a PostToolUse hook

These are created in the session's temp directory and do not affect your project.

## Session storage

Sessions are tracked in `.toc/sessions.yaml`:

```yaml
- id: 550e8400-e29b-41d4-a716-446655440000
  agent: pr-reviewer
  created: "2026-03-24T10:30:00Z"
  workspace: /tmp/toc-sessions/pr-reviewer-1711273800
```

Session workspaces live in `/tmp/toc-sessions/`. They persist until your system clears temp files or you manually delete them.

## Audit log

All actions are recorded in `.toc/audit.log` as append-only JSON Lines.

Each entry contains:

| Field | Description |
|---|---|
| `ts` | UTC timestamp (`2006-01-02T15:04:05.000Z`) |
| `action` | Action identifier (e.g. `agent.create`, `session.spawn`) |
| `actor` | Username from `$USER` environment variable |
| `hostname` | Machine hostname |
| `workspace` | Workspace name from config |
| `cwd` | Working directory |
| `details` | Action-specific metadata |
| `version` | toc version |

### Logged actions

| Action | When |
|---|---|
| `workspace.init` | `toc init` |
| `agent.create` | `toc agent create` |
| `agent.remove` | `toc agent remove` |
| `session.spawn` | `toc agent spawn` |
| `session.resume` | `toc agent spawn --resume` |
| `skill.create` | `toc skill create` |
| `skill.add` | `toc skill add` |
| `skill.remove` | `toc skill remove` |

No secrets or file contents are logged.

## Workspace directory structure

```
.toc/
├── config.yaml              # Workspace metadata
├── sessions.yaml            # Session history
├── audit.log                # Audit trail (JSON Lines)
├── agents/
│   └── pr-reviewer/
│       ├── oc-agent.yaml    # Agent config
│       └── agent.md         # Agent instructions
├── skills/
│   └── code-review/
│       └── SKILL.md         # Local skill definition
└── skills.yaml              # URL skill references
```
