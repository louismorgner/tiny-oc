# Getting started

This guide walks you through installing toc and spawning your first agent session.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed and authenticated

## Installation

```bash
git clone https://github.com/tiny-oc/toc.git
cd toc
make build
./install.sh
```

`install.sh` builds the binary and symlinks it to `/usr/local/bin/toc` (or `~/.local/bin/toc` if `/usr/local/bin` is not writable).

Verify the install:

```bash
toc --version
```

## Initialize a workspace

Navigate to any project directory and run:

```bash
toc init
```

This creates a `.toc/` directory to store agent configs, session history, and the audit log. You should commit `.toc/` to version control — it contains your agent definitions, which are meant to be shared.

## Create an agent

```bash
toc agent create
```

The interactive prompt asks for:

1. **Name** — lowercase alphanumeric with hyphens (e.g. `pr-reviewer`)
2. **Description** — shown in `toc agent list` and shell completions
3. **Model** — `sonnet`, `opus`, or `haiku`
4. **Context patterns** — optional glob patterns for files that sync back from sessions (see [context sync](configuration.md#context-sync-patterns))
5. **Instructions** — the agent's system prompt, written to `agent.md`

This creates two files in `.toc/agents/<name>/`:

- `oc-agent.yaml` — the agent config
- `agent.md` — the agent's instructions (loaded as `CLAUDE.md` in sessions)

## Spawn a session

```bash
toc agent spawn pr-reviewer
```

Or use the shorthand:

```bash
toc pr-reviewer
```

This:

1. Copies the agent template to an isolated temp directory
2. Builds `CLAUDE.md` from `agent.md` + any `compose` files, applying template variables (`{{.AgentName}}`, `{{.Date}}`, etc.)
3. Resolves and provisions any configured skills
4. Sets up context sync hooks (if context patterns are defined)
5. Launches a Claude Code session with the configured model

You're now inside a full Claude Code session with your agent's instructions loaded.

## Resume a session

Every session gets a UUID. To pick up where you left off:

```bash
toc agent spawn pr-reviewer --resume <session-id>
```

Find session IDs with:

```bash
toc status
```

## Next steps

- [Configuration reference](configuration.md) — all config fields and options
- [Skills guide](skills.md) — add reusable capabilities to your agents
- [Architecture](architecture.md) — how toc works under the hood
