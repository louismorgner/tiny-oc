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
on_end: >
  Save a concise summary of decisions made, open questions, and
  key learnings from this session to context/session-notes.md.
compose:
  - soul.md
  - user.md
sub-agents:
  - "*"
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
| `on_end` | string | no | — | Prompt run as a Claude Code `SessionEnd` hook (see below) |
| `compose` | list | no | — | Files to append after `agent.md` when building `CLAUDE.md` (see below) |
| `sub-agents` | list | no | — | Agent names this agent can spawn as sub-agents. Use `["*"]` for all agents |

### Agent instructions

Each agent also has an `agent.md` file in the same directory. This is loaded as `CLAUDE.md` when a session is spawned, serving as the agent's system instructions.

You can put anything in `agent.md` that you'd put in a `CLAUDE.md` — task descriptions, constraints, output format requirements, tool usage rules, etc.

### Compose

The `compose` field lists additional files to append to `agent.md` when building `CLAUDE.md` at spawn time. Files are appended in order, separated by `---`. The `agent.md` content is always first.

This is useful for separating concerns — e.g. keeping identity (`soul.md`) and user profile (`user.md`) as standalone files that get composed into the final instructions.

### Template variables

Both `agent.md` and compose files support template variables that are replaced at spawn time:

| Variable | Description |
|---|---|
| `{{.AgentName}}` | The agent's name from config |
| `{{.SessionID}}` | Unique session UUID |
| `{{.Date}}` | Today's date (`YYYY-MM-DD`) |
| `{{.Model}}` | The model being used |

## Sub-agents

The `sub-agents` field controls which other agents this agent is allowed to spawn as background tasks during a session. Agents without this field cannot spawn sub-agents.

```yaml
sub-agents:
  - "*"              # allow spawning any agent in the workspace
```

Or restrict to specific agents:

```yaml
sub-agents:
  - researcher
  - test-writer
```

During a session, agents use `toc runtime` commands to manage sub-agents:

| Command | Description |
|---|---|
| `toc runtime list` | List agents available to spawn |
| `toc runtime spawn <name> -p "..."` | Spawn a sub-agent in the background |
| `toc runtime status [session-id]` | Check sub-agent progress |
| `toc runtime output <session-id>` | Read completed sub-agent output |

Sub-agents run with `claude --print` in a detached process, capturing output to `toc-output.txt` in the session workspace. Parent-child relationships are tracked in `sessions.yaml`.

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

## Session end hook

The `on_end` field lets you define a prompt that runs automatically when a Claude Code session ends. It uses Claude Code's `SessionEnd` hook with an `agent`-type handler, meaning the prompt is evaluated by Claude with full tool access — it can read, write, and edit files before the session fully closes.

This is useful for persisting context that the agent might not save on its own during the session. For example, you can ask it to write a summary, update a knowledge base, or capture decisions.

```yaml
on_end: >
  Review what happened in this session. Save a concise summary of
  decisions, open questions, and anything worth remembering to
  context/session-notes.md. Append to the file if it already exists.
```

The prompt is written directly in your agent config — use it to describe what you want persisted and where. When combined with `context` patterns, any files the hook writes that match a pattern will be synced back to the agent template by the post-session sync pass.

`on_end` works independently of `context` — you can use it without context sync patterns if you just want end-of-session behavior without bidirectional file sync.

## Session storage

Sessions are tracked in `.toc/sessions.yaml`:

```yaml
sessions:
  - id: 550e8400-e29b-41d4-a716-446655440000
    agent: pr-reviewer
    created_at: "2026-03-24T10:30:00Z"
    workspace_path: /tmp/toc-sessions/pr-reviewer-abc123
    status: completed
  - id: 770e8400-e29b-41d4-a716-446655440001
    agent: researcher
    created_at: "2026-03-24T11:00:00Z"
    workspace_path: /tmp/toc-sessions/researcher-def456
    status: active
    parent_session_id: 550e8400-e29b-41d4-a716-446655440000
    prompt: "Research the latest security advisories for our dependencies"
```

| Field | Description |
|---|---|
| `id` | Unique session UUID |
| `agent` | Agent name |
| `created_at` | UTC timestamp |
| `workspace_path` | Isolated session directory |
| `status` | `active` or `completed` |
| `parent_session_id` | Parent session ID (sub-agents only) |
| `prompt` | Prompt given to the sub-agent (sub-agents only) |

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
| `agent.add` | `toc agent add` (registry install) |
| `runtime.spawn` | Sub-agent spawned during a session |

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
│       ├── agent.md         # Agent instructions (always first in CLAUDE.md)
│       ├── soul.md          # (optional) Identity file, listed in compose
│       └── user.md          # (optional) User profile, listed in compose
├── skills/
│   └── code-review/
│       └── SKILL.md         # Local skill definition
└── skills.yaml              # URL skill references
```
