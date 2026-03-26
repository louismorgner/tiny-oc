---
name: toc-setup
description: Set up and configure toc — a CLI tool for managing AI agent sessions from reusable templates. Use when a user wants to install toc, initialize a workspace, create agents, or configure oc-agent.yaml fields like model, context sync, permissions, and on_end hooks.
metadata:
  author: tiny-oc
  version: "0.1"
compatibility: Requires the Claude Code CLI (`claude`) for claude-code runtime agents
---

# toc setup and configuration

toc is a CLI tool for managing and spawning AI agent sessions from reusable templates. Each agent is a directory of files — instructions, context, scripts — that gets copied into a fresh temp workspace when spawned.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/louismorgner/tiny-oc/main/get-toc.sh | bash
```

Downloads the latest prebuilt binary for your platform. Supports macOS and Linux (amd64/arm64). After install, run `toc --help` to verify.

## Initialize a workspace

Run `toc init` in any project directory to create a `.toc/` workspace:

```bash
cd my-project
toc init
```

This creates:
```
.toc/
  config.yaml       # workspace name
  agents/           # agent templates live here
  sessions.yaml     # session tracking
```

You'll be prompted for a workspace name and optionally an OpenRouter API key (required for `toc-native` runtime agents).

**OpenRouter API key**: If you skip it during `toc init`, set it later:
```bash
export OPENROUTER_API_KEY=sk-or-...
```
Or re-run `toc init` won't work (already initialized) — instead set the env var in your shell profile, or run `toc init --skip-key` is for skipping only. To store the key manually, edit `.toc/secrets.yaml`:
```yaml
openrouter_key: sk-or-...
```

## Create an agent

```bash
toc agent create
```

This interactively prompts for name, description, runtime, and model, then creates:
```
.toc/agents/<name>/
  oc-agent.yaml     # agent configuration
  agent.md          # instructions loaded as CLAUDE.md when spawned
```

### `oc-agent.yaml` reference

Full example with all fields:

```yaml
runtime: claude-code          # required: "claude-code" or "toc-native"
name: pr-reviewer             # required: lowercase alphanumeric + hyphens
description: Reviews PRs      # optional: shown in toc agent list
model: sonnet                 # required: see model options below

# Optional fields:
max_iterations: 50            # limit agent turns (0 = unlimited)
allow_custom_native_model: false  # allow arbitrary OpenRouter model IDs

context:                      # glob patterns to sync back from sessions
  - "*.md"
  - "docs/"
  - "context/**/*.md"

skills:                       # skill names to load (from .claude/skills/)
  - toc-dev

on_end: |                     # prompt run by Claude when session ends
  Write a summary of what you did to session-summary.md

compose:                      # files appended to agent.md to build CLAUDE.md
  - shared/coding-standards.md

permissions:
  filesystem:
    read: on                  # on | ask | off (default: on)
    write: on
    execute: on
  integrations:
    github: ["read", "write"]
  sub-agents:
    "*": on                   # allow spawning any sub-agent
    # or specific: "other-agent": ask
```

### Runtimes and models

**`claude-code` runtime** (default): Launches Claude Code CLI. Requires `claude` to be installed and authenticated.

| Model   | Description                    |
|---------|--------------------------------|
| sonnet  | fast, great for most tasks     |
| opus    | most capable, deeper reasoning |
| haiku   | lightweight, quick responses   |

**`toc-native` runtime**: Headless agent runtime using OpenRouter. Requires `OPENROUTER_API_KEY`.

| Model ID                     | Description                            |
|------------------------------|----------------------------------------|
| openai/gpt-4o-mini           | fast default for agent-first iteration |
| openai/gpt-4o                | stronger OpenAI general-purpose model  |
| anthropic/claude-sonnet-4    | strong coding and reasoning            |

For custom OpenRouter models with `toc-native`, set `allow_custom_native_model: true` and use any model ID from openrouter.ai.

### `agent.md` — agent instructions

`agent.md` is the main instruction file loaded as `CLAUDE.md` when a session is spawned. Write it as you would any CLAUDE.md: task context, rules, workflow steps.

Template variables available inside `agent.md` and compose files:

| Variable          | Value                              |
|-------------------|------------------------------------|
| `{{.AgentName}}`  | agent name from oc-agent.yaml      |
| `{{.SessionID}}`  | unique UUID for this session       |
| `{{.Date}}`       | today's date (YYYY-MM-DD)          |
| `{{.Model}}`      | resolved model ID                  |

Example `agent.md`:

```markdown
# PR Reviewer

You are reviewing pull requests for the {{.AgentName}} agent session {{.SessionID}}.

## Instructions
1. Read the PR diff
2. Check for obvious bugs
3. Write your review to review.md
```

## Context sync

The `context:` field defines files that sync back from the temp session into the agent template in real-time. This lets the agent accumulate knowledge across sessions.

```yaml
context:
  - "notes.md"          # exact filename
  - "*.md"              # all markdown in root
  - "docs/"             # entire directory
  - "context/**/*.md"   # nested glob
```

How it works:
- At spawn time, toc generates a PostToolUse hook (`.claude/toc-sync.sh`) that fires on every file write
- Matching files are copied back to `.toc/agents/<name>/` after each write
- After the session ends, a final sync pass runs as a safety net

## Session end hook (`on_end`)

`on_end` is a prompt Claude evaluates with full tool access when the session closes. Use it to write summaries, commit work, or clean up.

```yaml
on_end: |
  Write a concise summary of what you accomplished to session-summary.md.
  Include any decisions made and files changed.
```

Works with or without `context:` — if you want the summary file synced back to the template, add it to `context:` too.

## Compose files

`compose:` appends additional files after `agent.md` when building the session's `CLAUDE.md`. Use this to share common instructions across agents:

```yaml
compose:
  - ../../shared/team-conventions.md
  - prompts/extra-context.md
```

Paths are relative to the agent directory (`.toc/agents/<name>/`).

## Spawn a session

```bash
toc agent spawn <name>          # start a new session
toc agent spawn <name> --resume # resume the last session
```

Sessions are created in `/tmp/toc-sessions/<name>-<timestamp>/`. The session ID is printed on start and can be used to resume with Claude Code's `--session-id` flag.

## Other commands

```bash
toc status                      # workspace overview + agent validation
toc agent list                  # list all agents
toc agent remove <name>         # remove agent + all its sessions
toc completion bash             # generate shell completions
toc completion zsh
toc completion fish
```

## Typical setup workflow

1. `git clone https://github.com/tiny-oc/toc && cd toc && ./install.sh`
2. `cd my-project && toc init`
3. `toc agent create` — pick name, runtime, model
4. Edit `.toc/agents/<name>/agent.md` with your instructions
5. Edit `.toc/agents/<name>/oc-agent.yaml` to add `context:`, `skills:`, etc.
6. `toc agent spawn <name>`
