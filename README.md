# toc — tiny open company

A local CLI tool for managing and spawning AI agent sessions from reusable templates.

Define agents as simple YAML configs, then spawn isolated sessions instantly. Built for engineers who want fast, reproducible agent workflows without cloud dependencies.

## Install

```bash
git clone https://github.com/tiny-oc/toc.git
cd toc
make build
./install.sh
```

Requires [Go 1.25+](https://go.dev/dl/) and [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

## Quick start

```bash
toc init                                      # initialize a workspace
toc agent create                              # create an agent interactively
toc agent spawn my-agent                      # spawn a session
toc agent spawn my-agent --resume <session-id>  # resume a session
toc status                                    # workspace overview
```

## How it works

`toc` manages a `.toc/` directory in your project root:

```
.toc/
  config.yaml            # workspace config
  sessions.yaml          # session history
  audit.log              # append-only audit log (JSON Lines)
  agents/
    pr-reviewer/
      oc-agent.yaml      # agent config
      agent.md           # agent instructions
```

When you run `toc agent spawn`, it:
1. Copies the agent template to an isolated temp directory
2. Launches a Claude Code session with the configured model
3. Syncs context files back to the agent template in real-time (if configured)
4. Tracks the session so you can resume it later

## Agent config

Each agent is defined by an `oc-agent.yaml` file:

```yaml
runtime: claude-code
name: pr-reviewer
description: Reviews pull requests for code quality
model: sonnet
context:
  - context/*.md
  - docs/
```

**Fields:**
- `runtime` — agent runtime, currently `claude-code` only
- `name` — lowercase alphanumeric with hyphens
- `description` — optional, shown in `toc agent list` and tab completion
- `model` — `sonnet`, `opus`, or `haiku`
- `context` — optional list of glob patterns for files that sync back from sessions to the agent template

## Context sync

Files matching `context:` patterns are synced back from the temp session directory to the agent template automatically:

- **Real-time**: Claude Code's PostToolUse hook fires on every file write and copies matching files back immediately
- **Post-session**: a final sync pass runs when the session ends as a safety net

Patterns support standard globs (`*.md`), directory matching (`docs/`), and recursive matching (`context/**/*.md`).

## Audit log

Every action is logged to `.toc/audit.log` as append-only JSON Lines — one JSON object per line with timestamp, action, actor, hostname, and details. No secrets or file contents are logged.

```bash
toc audit                    # show last 20 events
toc audit --tail 50          # show last 50 events
toc audit --action agent     # filter by action prefix
toc audit --json             # raw JSON Lines output (pipe to jq)
```

## Shell completion

```bash
# zsh (add to ~/.zshrc)
source <(toc completion zsh)

# bash (add to ~/.bashrc)
source <(toc completion bash)

# fish (run once)
toc completion fish > ~/.config/fish/completions/toc.fish
```

Tab-completes agent names, session IDs, and shell types.

## Commands

| Command | Description |
|---|---|
| `toc init` | Initialize a workspace in the current directory |
| `toc status` | Show workspace overview with agent validation |
| `toc agent create` | Create a new agent interactively |
| `toc agent list` | List all configured agents |
| `toc agent spawn <name>` | Spawn a new agent session |
| `toc agent spawn <name> --resume <id>` | Resume an existing session |
| `toc agent remove <name>` | Remove an agent and its sessions |
| `toc audit` | View the audit log |
| `toc completion <shell>` | Generate shell completion script |

## Roadmap

- [x] Agent creation and isolated session spawning via Claude Code
- [x] Context sync — persist session outputs back to agent templates
- [x] Audit log — append-only JSON Lines log for compliance and traceability
- [ ] Skills — reusable, shareable agent capabilities
- [ ] **v1 beta release**
- [ ] Sub-agents — agents that spawn and coordinate other agents
- [ ] Cost controls — per-agent and per-session spending limits
- [ ] Integrations and permissions — connect agents to external tools with scoped access

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
