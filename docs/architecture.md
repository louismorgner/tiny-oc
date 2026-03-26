# Architecture

A brief overview of how toc is structured internally.

For canonical vocabulary, see [Core concepts](core-concepts.md).

## Design principles

- **Local-first** — everything runs on your machine, with runtime integrations isolated behind explicit provider boundaries
- **Template-driven** — agents are defined as config + instructions, versioned alongside your code
- **Isolated sessions** — each spawn gets a fresh copy, so agents can't corrupt each other or your project
- **Auditable** — every action is logged for traceability

## Session lifecycle

```
toc agent spawn pr-reviewer
        │
        ▼
┌─────────────────┐
│  Load agent      │  Read .toc/agents/pr-reviewer/oc-agent.yaml
│  config          │  and agent.md
└───────┬─────────┘
        │
        ▼
┌─────────────────┐
│  Create session  │  Generate UUID, create /tmp/toc-sessions/pr-reviewer-<ts>/
│  workspace       │  Copy the parent agent snapshot to a temp working dir
└───────┬─────────┘
        │
        ▼
┌─────────────────┐
│  Provision       │  Let the runtime provider prepare instructions,
│  session         │  runtime dirs, hooks, and other session files
│                  │  Resolve and persist session.json, then resolve skills
│                  │  into the provider's runtime workspace
└───────┬─────────┘
        │
        ▼
┌─────────────────┐
│  Launch runtime  │  Dispatch to the configured runtime provider
│  Code            │  Working directory: session temp dir
└───────┬─────────┘
        │
        ▼  (during session)
┌─────────────────┐
│  Real-time sync  │  Runtime-specific machinery can push matching files
│                  │  back to the parent snapshot while the session runs
└───────┬─────────┘
        │
        ▼  (session ending, if on_end configured)
┌─────────────────┐
│  Session end     │  Runtime-specific end-of-session behavior can persist
│  hook            │  context before the session fully closes
└───────┬─────────┘
        │
        ▼  (session closed)
┌─────────────────┐
│  Post-session    │  Final sync pass copies any remaining matches
│  sync            │  Session recorded in .toc/sessions.yaml
└─────────────────┘
```

## Project structure

```
├── main.go                    # Entry point
├── cmd/                       # CLI commands (Cobra)
│   ├── root.go                # Root command, version flag, agent shorthand
│   ├── init.go                # toc init
│   ├── config.go              # toc config (set/get workspace secrets)
│   ├── status.go              # toc status (static + TUI dashboard)
│   ├── status_tui.go          # BubbleTea interactive dashboard
│   ├── audit.go               # toc audit
│   ├── completion.go          # toc completion
│   ├── agent_*.go             # toc agent {create,list,spawn,show,inspect,remove,skills,add}
│   ├── skill_*.go             # toc skill {create,list,add,remove}
│   ├── registry_*.go          # toc registry {search,install}
│   ├── integrate*.go          # toc integrate {add,test,remove,list} (test is integrate_test_cmd.go to avoid Go test runner)
│   ├── runtime.go             # toc runtime (parent command, agent-facing)
│   ├── runtime_list.go        # toc runtime list
│   ├── runtime_spawn.go       # toc runtime spawn
│   ├── runtime_status.go      # toc runtime status
│   ├── runtime_output.go      # toc runtime output
│   ├── runtime_invoke.go      # toc runtime invoke (integration gateway)
│   ├── runtime_watch.go       # toc runtime watch (live-tail session)
│   ├── runtime_replay.go      # toc runtime replay (session playback)
│   ├── runtime_cancel.go      # toc runtime cancel
│   └── runtime_native_run.go  # toc __native-run (native runtime entry point)
├── internal/
│   ├── agent/                 # Agent config: load, save, validate, permissions
│   ├── audit/                 # Append-only JSON Lines audit log
│   ├── config/                # Workspace config, paths, secrets
│   ├── integration/           # API gateway: definitions, credentials, permissions, rate limiting
│   ├── registry/              # Remote registry: fetch, search, install skills and agents
│   ├── runtime/               # Provider interface, Claude Code + native implementations
│   ├── runtimeinfo/           # Runtime metadata, native model profiles
│   ├── session/               # Session tracking (sessions.yaml), parent-child relationships
│   ├── skill/                 # Skill management: local + URL
│   ├── spawn/                 # Session orchestration, sub-agent spawning
│   ├── sync/                  # Snapshot sync: patterns and file copy primitives
│   ├── tail/                  # Live output streaming from session logs
│   ├── ui/                    # Terminal output helpers, TUI components
│   ├── fileutil/              # Directory and file copy utilities
│   ├── gitutil/               # Safe git clone (HTTPS only, hooks disabled)
│   ├── naming/                # Session name generation from prompts
│   └── usage/                 # Token usage parsing and formatting
├── registry/                  # Built-in skills, agents, and integration definitions
│   ├── agents/                # cto, mini-claw
│   ├── skills/                # open-source-cto, agentic-memory
│   └── integrations/          # github, slack
├── web/                       # OAuth callback worker (Cloudflare)
├── e2e/                       # End-to-end smoke tests
├── Makefile                   # build, test, lint, test-e2e targets
└── install.sh                 # Build + symlink to PATH
```

## Key internals

### Config (`internal/config/`)

Manages workspace state. `config.Exists()` checks if `.toc/` is initialized. All paths (agents dir, skills dir, sessions file, audit log) are derived from the `.toc/` root. Also manages workspace secrets (`.toc/secrets.yaml`) for API keys like OpenRouter.

### Spawn (`internal/spawn/`)

Orchestrates session creation. This is the core flow — copies the agent template, resolves declarative agent config into a toc-owned per-session contract, writes `.toc/sessions/<id>/session.json`, delegates runtime-specific session preparation to the provider, resolves skills into the provider's runtime directory, writes the permission manifest, records the selected runtime in session metadata, and dispatches process launch through the runtime provider.

### Runtime (`internal/runtime/`)

The largest package. It owns:

- **Provider interface** — the abstraction over execution backends (`claudeProvider`, `nativeProvider`)
- **Claude Code provider** — launches `claude` CLI, generates hooks (`.claude/settings.json`), parses Claude Code's JSONL logs into normalized events
- **Native provider** — launches the built-in agent loop, calls OpenRouter, manages state persistence and resume
- **Session config** — the resolved per-session contract (`session.json`) that freezes agent config at spawn time
- **Permission enforcement** — validates filesystem and sub-agent permissions at tool execution time
- **Events** — append-only `events.jsonl` log in toc's normalized format, shared across runtimes
- **State** — persistent session state for native runtime (`state.json`) with message history, token usage, and turn checkpoints

The native runtime's tool set is intentionally limited to local tools. Integrations remain outside the native tool loop until they are promoted into the runtime as first-class tools with the same session contract and observability semantics.

Environment variables `TOC_WORKSPACE`, `TOC_AGENT`, and `TOC_SESSION_ID` are injected at launch time and used by `toc runtime` commands to resolve context from within a running session.

### Integration (`internal/integration/`)

API gateway for external services. Handles credential encryption (AES-256-GCM with keychain-backed master key), permission checking against the session manifest, file-backed per-session rate limiting, HTTP request construction with parameter templating, and response field filtering. See [Integrations](integrations.md).

### Sync (`internal/sync/`)

Handles snapshot sync between session temp directories and parent agent templates. It implements glob pattern matching (including `**` recursive patterns) and generic file-copy behavior; provider-specific hook or callback wiring lives in the runtime implementation.

### Audit (`internal/audit/`)

Append-only logger. Each event is a single JSON line written to `.toc/audit.log`. The actor and hostname are resolved once from `$USER` and `os.Hostname()`.

### Skills (`internal/skill/`)

Two-tier system: local skills in `.toc/skills/` and URL references in `.toc/skills.yaml`. Skills are validated by checking for a `SKILL.md` with required `name` and `description` frontmatter fields.
