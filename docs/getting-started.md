# Getting started

This guide walks you through installing toc and spawning your first agent session.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- A supported runtime installed and authenticated

Supported external runtimes: [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [OpenAI Codex CLI](https://developers.openai.com/codex/cli)

## Installation

```bash
git clone https://github.com/louismorgner/tiny-oc.git
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
3. **Runtime** — choose `claude-code`, `codex`, or `toc-native`
4. **Model** — depends on the selected runtime; `claude-code` supports Claude aliases, `codex` defaults to `gpt-5-codex`, and `toc-native` uses an OpenRouter model ID
5. **Context patterns** — optional snapshot-sync patterns for files that should flow back into the parent agent snapshot (see [snapshot sync](configuration.md#snapshot-sync-patterns))
6. **Instructions** — the agent's system prompt, written to `agent.md`

This creates two files in `.toc/agents/<name>/`:

- `oc-agent.yaml` — the agent config
- `agent.md` — the agent's instructions (materialized into the runtime's instruction format at spawn time)

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
2. Lets the runtime provider build the final instruction payload from `agent.md` + any `compose` files, applying template variables (`{{.AgentName}}`, `{{.Date}}`, etc.)
3. Resolves and provisions any configured skills
4. Sets up any runtime-specific snapshot-sync or permission machinery
5. Launches a runtime session with the configured model

With `claude-code`, you're dropped into Claude Code with `CLAUDE.md` provisioned for the session. With `codex`, toc provisions `AGENTS.md` and launches the OpenAI Codex CLI in the isolated session workspace.

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
- [Runtimes](runtimes.md) — Claude Code vs Codex vs toc-native, hooks, model profiles
- [Skills guide](skills.md) — add reusable capabilities to your agents
- [Integrations](integrations.md) — connect agents to GitHub, Slack, and other APIs
- [Architecture](architecture.md) — how toc works under the hood
