# Architecture

A brief overview of how toc is structured internally.

## Design principles

- **Local-first** — everything runs on your machine, no cloud dependencies beyond Claude Code itself
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
│  workspace       │  Copy agent template to temp dir
└───────┬─────────┘
        │
        ▼
┌─────────────────┐
│  Provision       │  Build CLAUDE.md from agent.md + compose files
│  session         │  Apply template variables ({{.AgentName}}, {{.Date}}, etc.)
│                  │  Resolve skills → .claude/skills/
│                  │  Generate sync hooks → .claude/toc-sync.sh + settings.json
└───────┬─────────┘
        │
        ▼
┌─────────────────┐
│  Launch Claude   │  Execute: claude --model <model> --session-id <uuid>
│  Code            │  Working directory: session temp dir
└───────┬─────────┘
        │
        ▼  (during session)
┌─────────────────┐
│  Real-time sync  │  PostToolUse hook fires on Edit/Write/MultiEdit
│                  │  Matching files copied back to agent template
└───────┬─────────┘
        │
        ▼  (session ending, if on_end configured)
┌─────────────────┐
│  Session end     │  SessionEnd hook runs on_end prompt with tool access
│  hook            │  Agent persists context before session closes
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
│   ├── status.go              # toc status
│   ├── audit.go               # toc audit
│   ├── completion.go          # toc completion
│   ├── agent.go               # toc agent (parent command)
│   ├── agent_create.go        # toc agent create
│   ├── agent_list.go          # toc agent list
│   ├── agent_spawn.go         # toc agent spawn (with --resume)
│   ├── agent_remove.go        # toc agent remove
│   ├── agent_skills.go        # toc agent skills
│   ├── agent_add.go           # toc agent add (from registry)
│   ├── skill.go               # toc skill (parent command)
│   ├── skill_create.go        # toc skill create
│   ├── skill_list.go          # toc skill list
│   ├── skill_add.go           # toc skill add (URL or registry name)
│   ├── skill_remove.go        # toc skill remove
│   ├── registry.go            # toc registry (parent command)
│   ├── registry_search.go     # toc registry search
│   ├── registry_install.go    # toc registry install
│   ├── runtime.go             # toc runtime (parent command, agent-facing)
│   ├── runtime_list.go        # toc runtime list
│   ├── runtime_spawn.go       # toc runtime spawn
│   ├── runtime_status.go      # toc runtime status
│   └── runtime_output.go      # toc runtime output
├── internal/
│   ├── agent/                 # Agent config: load, save, validate, sub-agent permissions
│   ├── audit/                 # Append-only JSON Lines audit log
│   ├── config/                # Workspace config and paths
│   ├── registry/              # Remote registry: fetch, search, install skills and agents
│   ├── runtime/               # Runtime context: env var resolution for agent sessions
│   ├── session/               # Session tracking (sessions.yaml), parent-child relationships
│   ├── skill/                 # Skill management: local + URL
│   ├── spawn/                 # Session orchestration, sub-agent spawning
│   ├── sync/                  # Context sync: patterns, hooks, file copy
│   └── ui/                    # Terminal output helpers (colors, prompts)
├── registry/                  # Built-in skills and agent templates
│   ├── agents/                # cto, mini-claw
│   └── skills/                # open-source-cto, agentic-memory
├── Makefile                   # build, test, lint targets
└── install.sh                 # Build + symlink to PATH
```

## Key internals

### Config (`internal/config/`)

Manages workspace state. `config.Exists()` checks if `.toc/` is initialized. All paths (agents dir, skills dir, sessions file, audit log) are derived from the `.toc/` root.

### Spawn (`internal/spawn/`)

Orchestrates session creation. This is the core flow — copies the agent template, builds CLAUDE.md from agent.md + compose files with template variable substitution, resolves skills, sets up sync hooks, and execs the `claude` CLI as a subprocess.

### Sync (`internal/sync/`)

Handles bidirectional file sync between session temp directories and agent templates. Implements glob pattern matching (including `**` recursive patterns) and generates the PostToolUse hook shell script.

### Audit (`internal/audit/`)

Append-only logger. Each event is a single JSON line written to `.toc/audit.log`. The actor and hostname are resolved once from `$USER` and `os.Hostname()`.

### Runtime (`internal/runtime/`)

Provides session context for `toc runtime` commands. Reads `TOC_WORKSPACE`, `TOC_AGENT`, and `TOC_SESSION_ID` environment variables (injected at launch time) to resolve the workspace, load agent configs, and enforce sub-agent permissions from within a running session.

### Skills (`internal/skill/`)

Two-tier system: local skills in `.toc/skills/` and URL references in `.toc/skills.yaml`. Skills are validated by checking for a `SKILL.md` with required `name` and `description` frontmatter fields.
