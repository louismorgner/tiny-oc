---
name: toc-dev
description: Use when working on the toc CLI codebase — adding features, fixing bugs, or understanding architecture. Provides project context, code layout, conventions, and development workflow.
metadata:
  author: tiny-oc
  version: "0.1"
---

# toc development skill

toc is a local CLI tool for managing and spawning AI agent sessions from reusable templates. Written in Go, it uses Cobra for commands, charmbracelet/huh for interactive prompts, and fatih/color for terminal styling.

## Architecture

```
main.go                         → entry point, calls cmd.Execute()
cmd/
  root.go                       → root cobra command, version injection via ldflags
  init.go                       → toc init — create .toc/ workspace
  status.go                     → toc status — workspace overview with agent validation
  agent.go                      → parent command group for agent subcommands
  agent_create.go               → interactive agent creation with huh select for model
  agent_list.go                 → table listing of agents
  agent_remove.go               → remove agent + associated sessions
  agent_spawn.go                → spawn/resume sessions, tab completion functions
  completion.go                 → shell completion generation (bash/zsh/fish)
internal/
  config/config.go              → workspace config (.toc/config.yaml), path helpers
  agent/agent.go                → agent CRUD, validation, oc-agent.yaml schema
  session/session.go            → session tracking (.toc/sessions.yaml)
  spawn/spawn.go                → session lifecycle: copy template, setup hooks, launch claude, post-session sync
  sync/sync.go                  → context file pattern matching and sync-back logic
  sync/sync_test.go             → pattern matching tests
  ui/ui.go                      → terminal UI: prompts, select menus, colored output helpers
```

## Key data structures

**AgentConfig** (`internal/agent/agent.go`):
- Fields: Runtime, Name, Description, Model, Context (glob patterns)
- Stored as `.toc/agents/<name>/oc-agent.yaml`
- Validate() checks runtime (claude-code), model (sonnet/opus/haiku), name format

**Session** (`internal/session/session.go`):
- Fields: ID (uuid), Agent, CreatedAt, WorkspacePath
- Stored in `.toc/sessions.yaml`

**WorkspaceConfig** (`internal/config/config.go`):
- Fields: Name
- Stored in `.toc/config.yaml`

## How spawning works

1. `spawn.SpawnSession()` creates a temp dir at `/tmp/toc-sessions/<name>-<timestamp>/`
2. Copies the entire agent template directory into it
3. If the agent has `context:` patterns, generates `.claude/settings.json` with a PostToolUse hook and `.claude/toc-sync.sh` — this syncs matching files back to the agent template in real-time
4. Launches `claude` CLI with `--model` and `--session-id` flags
5. After session ends, runs a post-session sync pass as a safety net
6. Prints resume command

## Context sync system

The `context:` field in `oc-agent.yaml` defines glob patterns for files that should be synced back from temp sessions to the agent template. Supports: `*.md`, `docs/`, `context/**/*.md`, bare filenames. Pattern matching is in `internal/sync/sync.go` with tests in `sync_test.go`.

Real-time sync uses Claude Code's PostToolUse hook (fires on Edit/Write/MultiEdit). Post-session sync walks the session dir as a fallback.

## Development workflow

```bash
make build                      # build to bin/toc
make test                       # run all tests
make lint                       # go vet
./install.sh                    # build + symlink to PATH
```

Version injection: `make build VERSION=0.1.0` sets `cmd.version` via ldflags.

## Conventions

- All commands check `config.EnsureInitialized()` before running (except `init` and `completion`)
- Tab completion functions for agent names and session IDs live in `cmd/agent_spawn.go` and are reused by `agent_remove.go`
- UI output uses the `internal/ui` package — never raw fmt.Print in commands
- Agent removal also cleans up associated sessions
- The `huh` library is used for interactive select menus (model picker in create)
- Error handling in `launchClaude`: ExitError = normal (user quit), other errors = real failures

## Adding a new command

1. Create `cmd/<command>.go`
2. Define a `cobra.Command` variable
3. In `init()`, add it to the parent command (`rootCmd` or `agentCmd`)
4. Check `config.EnsureInitialized()` at the start of RunE if the command needs a workspace
5. Add `ValidArgsFunction` if the command takes a positional argument that can be completed
6. Use `ui.*` functions for all output

## Gotchas

- `config.go` uses relative paths (`.toc/`) — all commands must run from the workspace root
- Session workspace paths are absolute (`/tmp/toc-sessions/...`) but the agent template paths are relative
- The sync shell script uses absolute paths for both session and agent dirs, resolved at spawn time
- `agent.List()` silently skips agents with unparseable configs — this is intentional so one broken agent doesn't break the whole list
- The `.claude/` directory in session temp dirs is skipped during post-session sync to avoid syncing hook artifacts back
