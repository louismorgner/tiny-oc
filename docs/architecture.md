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
│  Provision       │  Rename agent.md → CLAUDE.md
│  session         │  Resolve skills → .claude/skills/
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
        ▼  (session ends)
┌─────────────────┐
│  Post-session    │  Final sync pass copies any remaining matches
│  sync            │  Session recorded in .toc/sessions.yaml
└─────────────────┘
```

## Project structure

```
├── main.go                    # Entry point
├── cmd/                       # CLI commands (Cobra)
│   ├── root.go                # Root command, version flag
│   ├── init.go                # toc init
│   ├── status.go              # toc status
│   ├── audit.go               # toc audit
│   ├── completion.go          # toc completion
│   └── agent/                 # toc agent subcommands
│       ├── create.go
│       ├── list.go
│       ├── spawn.go
│       ├── remove.go
│       └── skills.go
│   └── skill/                 # toc skill subcommands
│       ├── create.go
│       ├── list.go
│       ├── add.go
│       └── remove.go
├── internal/
│   ├── agent/                 # Agent config: load, save, validate
│   ├── audit/                 # Append-only JSON Lines audit log
│   ├── config/                # Workspace config and paths
│   ├── registry/              # Remote skill registry (GitHub)
│   ├── session/               # Session tracking (sessions.yaml)
│   ├── skill/                 # Skill management: local + URL
│   ├── spawn/                 # Session orchestration
│   ├── sync/                  # Context sync: patterns, hooks, file copy
│   └── ui/                    # Terminal output helpers (colors, prompts)
├── registry/                  # Built-in skill definitions
│   └── skills/
├── Makefile                   # build, test, lint targets
└── install.sh                 # Build + symlink to PATH
```

## Key internals

### Config (`internal/config/`)

Manages workspace state. `config.Exists()` checks if `.toc/` is initialized. All paths (agents dir, skills dir, sessions file, audit log) are derived from the `.toc/` root.

### Spawn (`internal/spawn/`)

Orchestrates session creation. This is the core flow — copies the agent template, provisions CLAUDE.md, resolves skills, sets up sync hooks, and execs the `claude` CLI as a subprocess.

### Sync (`internal/sync/`)

Handles bidirectional file sync between session temp directories and agent templates. Implements glob pattern matching (including `**` recursive patterns) and generates the PostToolUse hook shell script.

### Audit (`internal/audit/`)

Append-only logger. Each event is a single JSON line written to `.toc/audit.log`. The actor and hostname are resolved once from `$USER` and `os.Hostname()`.

### Skills (`internal/skill/`)

Two-tier system: local skills in `.toc/skills/` and URL references in `.toc/skills.yaml`. Skills are validated by checking for a `SKILL.md` with required `name` and `description` frontmatter fields.
