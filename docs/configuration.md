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
small_model: haiku
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
permissions:
  sub-agents:
    "*": "on"
```

### Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `runtime` | string | yes | — | Agent runtime implementation. Current runtimes are `claude-code` and `toc-native` |
| `name` | string | yes | — | Lowercase alphanumeric with hyphens (e.g. `pr-reviewer`). Must match `^[a-z0-9][a-z0-9-]*$` |
| `description` | string | no | — | Shown in `toc agent list` and shell tab completion |
| `model` | string | yes | — | Declarative model preference for the selected runtime. For `claude-code`: `default`, `sonnet`, `opus`, or `haiku`. For `toc-native`, use an OpenRouter model ID such as `openai/gpt-4o-mini` |
| `small_model` | string | no | — | Optional secondary model preference for lightweight work. In `toc-native`, structured compaction uses this model when set, otherwise it falls back to `model` |
| `allow_custom_native_model` | bool | no | `false` | For `toc-native`, allow a model outside the supported beta profile set. Use only as an explicit override for experimental models |
| `context` | list | no | — | Glob patterns for snapshot sync between the parent snapshot and spawned sessions (see below) |
| `skills` | list | no | — | Skill names (local) or Git URLs (remote) |
| `on_end` | string | no | — | End-of-session prompt or instruction for the selected runtime (see below) |
| `compose` | list | no | — | Instruction-compose layers appended after `agent.md` when building the runtime instruction payload (see below) |
| `permissions` | object | no | — | Unified permission spec (see below) |

### Agent instructions

Each agent also has an `agent.md` file in the same directory. This is the runtime-neutral source of truth for the agent's system instructions.

At spawn time, toc hands `agent.md` and any `compose` files to the runtime provider, which materializes the runtime-specific instruction file or payload. For the current `claude-code` runtime, that means generating `CLAUDE.md`.

You can put anything in `agent.md` that you'd put in a system prompt or runtime instruction file — task descriptions, constraints, output format requirements, tool usage rules, etc.

### Config layers

`oc-agent.yaml` is the declarative agent contract. It captures intent: runtime, model, context sync, skills, permissions, and end-of-session behavior.

At spawn time, toc resolves that into a stable per-session config written to `.toc/sessions/<id>/session.json`. Runtime providers launch and resume from that resolved config, so an in-flight session does not change if `oc-agent.yaml` is edited later.

For `toc-native`, this resolved session config is the native runtime input boundary. Runtime internals such as `.toc-native/system-prompt.md` are derived artifacts, not a second config source of truth.

The layers are:

- `oc-agent.yaml`: declarative agent intent
- `.toc/sessions/<id>/session.json`: resolved session contract
- runtime internals: provider-specific implementation details such as Claude hooks or the native OpenRouter loop

### Native model policy

`toc-native` does not treat every OpenRouter model as beta-supported by default.

By default, the `model` value must match one of toc's supported native profiles. Those profiles also have to satisfy the native beta capability contract, which currently requires reliable tool-calling support.

If you want to experiment with a model outside that supported set, you must opt in explicitly:

```yaml
runtime: toc-native
model: some/provider-model
small_model: some/provider-small-model
allow_custom_native_model: true
```

This escape hatch is for experimentation, not the default beta path. Unknown custom models may work, but toc will not treat them as supported native profiles.

### Native beta scope

The current `toc-native` beta scope is local tools plus first-class public URL viewing.

That means the native tool loop is expected to support:

- file reads and writes
- file edits
- glob and grep
- shell execution
- public URL fetch and HTML-to-Markdown conversion
- skill reads

Authenticated integrations and browser automation are intentionally out of scope for this beta. They are not yet part of the native runtime's broader first-class tool contract, even if the workspace supports integrations elsewhere.

### Instruction compose

The `compose` field is the current config name for instruction compose. It lists additional files to append to `agent.md` when building the final runtime instruction payload at spawn time. Files are appended in order, separated by `---`. The `agent.md` content is always first.

This is useful for separating concerns — e.g. keeping identity (`soul.md`) and user profile (`user.md`) as standalone files that get composed into the final instructions.

### Template variables

Both `agent.md` and compose files support template variables that are replaced at spawn time:

| Variable | Description |
|---|---|
| `{{.AgentName}}` | The agent's name from config |
| `{{.SessionID}}` | Unique session UUID |
| `{{.Date}}` | Today's date (`YYYY-MM-DD`) |
| `{{.Model}}` | The model being used |

## Permissions

The `permissions` block controls what an agent is allowed to do. It has four areas: filesystem access, network access, integration access, and sub-agent spawning.

### Filesystem permissions

Controls access to file and shell tools. Each level is `on` (allowed), `ask` (requires confirmation), or `off` (blocked). Defaults to all `on` if not specified.

```yaml
permissions:
  filesystem:
    read: "on"
    write: "on"
    execute: "ask"
```

| Level | Tools controlled |
|---|---|
| `read` | Read, Glob, Grep |
| `write` | Edit, Write, MultiEdit, NotebookEdit |
| `execute` | Bash |

With the `claude-code` runtime, these are enforced via a `PreToolUse` hook script. With `toc-native`, they are checked directly before tool execution.

### Network permissions

Controls outbound web fetching from first-class native tools. Each level is `on` (allowed), `ask` (requires confirmation), or `off` (blocked). Defaults to `off`.

```yaml
permissions:
  network:
    web: "on"
```

| Level | Tools controlled |
|---|---|
| `web` | WebFetch |

With `toc-native`, this gate applies to the native `WebFetch` tool. It does not grant access to authenticated integrations; those are still controlled separately under `permissions.integrations`.

### Integration permissions

Controls which external API actions an agent can perform. See [Integrations](integrations.md) for the full system.

```yaml
permissions:
  integrations:
    github:
      - "issues.read:*"
      - "pulls.read:*"
    slack:
      - "send_message:#engineering"
```

### Sub-agents

The `permissions.sub-agents` map controls which other agents this agent is allowed to spawn as background tasks during a session. Agents without this field cannot spawn sub-agents. Each entry maps an agent name to a permission level (`on`, `ask`, or `off`).

```yaml
permissions:
  sub-agents:
    "*": "on"          # allow spawning any agent in the workspace
```

Or restrict to specific agents:

```yaml
permissions:
  sub-agents:
    researcher: "on"
    test-writer: "on"
```

During a session, agents use `toc runtime` commands to manage sub-agents:

| Command | Description |
|---|---|
| `toc runtime list` | List agents available to spawn |
| `toc runtime spawn <name> -p "..."` | Spawn a sub-agent in the background |
| `toc runtime status [session-id]` | Check sub-agent progress |
| `toc runtime output <session-id>` | Read completed sub-agent output |

Sub-agents run in a detached runtime process, capturing output to `toc-output.txt` in the session workspace. Parent-child relationships are tracked in `sessions.yaml`. With the current `claude-code` runtime, this uses detached `claude -p` / `claude --continue` invocations.

If a detached session calls the native `Question` tool, inspect it with `toc question [session-id]` and respond with `toc answer <session-id> --text "..."`. `toc runtime status` and `toc debug` surface pending questions for sub-agents.

## Snapshot sync patterns

The `context` field is the current config name for snapshot sync paths. It defines which files sync between the parent agent snapshot and spawned sessions. In practice, this is how agents accumulate knowledge across sessions without making the full snapshot bidirectional.

Patterns support:

| Pattern | Example | Matches |
|---|---|---|
| Standard glob | `*.md` | Any `.md` file (including in subdirectories) |
| Directory | `docs/` | Everything under `docs/` |
| Path glob | `context/*.md` | `.md` files directly in `context/` |
| Recursive glob | `docs/**/*.md` | `.md` files at any depth under `docs/` |

Sync happens at two points:

1. **Real-time** — runtime-specific machinery can sync matching files back while the session is still running. With the current `claude-code` runtime, this uses a Claude Code `PostToolUse` hook on `Edit`, `Write`, and `MultiEdit`.
2. **Post-session** — when the session ends, a full sync pass runs as a safety net.

Conceptually:

- the agent template in `.toc/agents/<name>/` is the parent snapshot
- a spawned session gets an isolated working copy of that snapshot
- files matching `context` are synced back into the parent snapshot

### How it works

When a session is spawned, toc may generate runtime-specific files to support context sync.

With the current `claude-code` runtime, toc generates:

- `.claude/toc-sync.sh` — a shell script that reads the PostToolUse hook payload and copies matching files
- `.claude/settings.json` — registers the sync script as a PostToolUse hook

These are created in the session's temp directory and do not affect your project.

## Session end hook

The `on_end` field lets you define end-of-session behavior for the selected runtime.

With the current `claude-code` runtime, it is implemented via Claude Code's `SessionEnd` hook with an `agent`-type handler, meaning the prompt is evaluated by Claude with full tool access before the session fully closes.

This is useful for persisting context that the agent might not save on its own during the session. For example, you can ask it to write a summary, update a knowledge base, or capture decisions.

```yaml
on_end: >
  Review what happened in this session. Save a concise summary of
  decisions, open questions, and anything worth remembering to
  context/session-notes.md. Append to the file if it already exists.
```

The prompt is written directly in your agent config — use it to describe what you want persisted and where. When combined with `context` patterns, any files the hook writes that match a pattern will be synced back into the parent snapshot by the post-session sync pass.

`on_end` works independently of `context` — you can use it without context sync patterns if you just want end-of-session behavior without bidirectional file sync.

## Session storage

Sessions are tracked in `.toc/sessions.yaml`:

```yaml
sessions:
  - id: 550e8400-e29b-41d4-a716-446655440000
    agent: pr-reviewer
    runtime: claude-code
    metadata_dir: /path/to/project/.toc/sessions/550e8400-e29b-41d4-a716-446655440000
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
| `runtime` | Runtime used for the session |
| `metadata_dir` | toc-owned per-session metadata directory |
| `created_at` | UTC timestamp |
| `workspace_path` | Isolated session directory |
| `status` | `active` or `completed` |
| `parent_session_id` | Parent session ID (sub-agents only) |
| `prompt` | Prompt given to the sub-agent (sub-agents only) |

Session workspaces live in `/tmp/toc-sessions/`. They persist until your system clears temp files or you manually delete them.

toc-owned per-session metadata lives under `.toc/sessions/<id>/`, including:

- `session.json` — resolved session config used for launch and resume
- `permissions.json` — resolved integration permission manifest
- `events.jsonl` — normalized event stream owned by toc

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
| `runtime.invoke` | Integration API call during a session |
| `integrate.add` | Integration added |
| `integrate.remove` | Integration removed |

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
│       ├── agent.md         # Runtime-neutral instruction source
│       ├── soul.md          # (optional) Identity file, listed in compose
│       └── user.md          # (optional) User profile, listed in compose
├── sessions/
│   └── <session-id>/
│       ├── session.json     # Resolved session config
│       ├── permissions.json # Resolved session permissions
│       └── events.jsonl     # toc-owned normalized event log
├── skills/
│   └── code-review/
│       └── SKILL.md         # Local skill definition
└── skills.yaml              # URL skill references
```
