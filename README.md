# toc — tiny open company

[![CI](https://github.com/louismorgner/tiny-oc/actions/workflows/ci.yaml/badge.svg)](https://github.com/louismorgner/tiny-oc/actions/workflows/ci.yaml)

A local CLI tool for managing and spawning AI agent sessions from reusable templates.

Define agents as simple YAML configs, then spawn isolated sessions instantly. Built for engineers who want fast, reproducible agent workflows with a local-first control plane and pluggable runtimes.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/louismorgner/tiny-oc/main/get-toc.sh | bash
```

Downloads the latest prebuilt binary for your platform. Supports macOS and Linux (amd64/arm64).

Current runtime implementation: [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

`toc-native` is now on the internal beta path. Its current beta scope is local tools only: file operations, shell, glob/grep, and skills. External integrations are deferred until they are promoted into the native runtime as first-class tools. By default it uses OpenRouter, and you can override just the native runtime's API base URL with `TOC_NATIVE_BASE_URL`.

### Build from source

```bash
git clone https://github.com/louismorgner/tiny-oc.git
cd toc
make build
./install.sh
```

Requires [Go 1.25+](https://go.dev/dl/).

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
2. Lets the selected runtime provider prepare the session workspace
3. Launches a runtime session with the configured model
4. Syncs configured snapshot files back to the parent agent template in real-time (if configured)
5. Tracks the session so you can resume it later

## Documentation

- [Core concepts](docs/core-concepts.md) — canonical vocabulary for sessions, snapshots, permissions, and instructions
- [Getting started](docs/getting-started.md) — install, create your first agent, spawn a session
- [Configuration reference](docs/configuration.md) — all config fields, snapshot sync patterns, audit log format
- [Runtimes](docs/runtimes.md) — provider abstraction, Claude Code vs toc-native, hooks, model profiles
- [Skills guide](docs/skills.md) — create, install, and attach reusable capabilities
- [Integrations](docs/integrations.md) — API gateway, credential storage, permissions, built-in integrations
- [Architecture](docs/architecture.md) — project structure and design decisions

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
| `toc agent skills <name>` | Manage skills for an agent |
| `toc skill create` | Create a new local skill |
| `toc skill list` | List available skills |
| `toc skill add <name-or-url>` | Install a skill from a Git URL or the registry |
| `toc skill remove <name>` | Remove a skill |
| `toc agent add <name>` | Install an agent template from the registry |
| `toc registry search [query]` | Browse skills and agent templates |
| `toc registry install <name>` | Install a skill or agent from the registry |
| `toc integrate add <name>` | Add an integration (GitHub, Slack) |
| `toc integrate list` | List configured integrations |
| `toc integrate test <name>` | Test an integration connection |
| `toc integrate remove <name>` | Remove an integration |
| `toc audit` | View the audit log |
| `toc config set <key> <value>` | Set workspace config or secrets |
| `toc completion <shell>` | Generate shell completion script |

### Runtime commands (for agents during sessions)

| Command | Description |
|---|---|
| `toc runtime list` | List agents available to spawn as sub-agents |
| `toc runtime spawn <name> -p "..."` | Spawn a sub-agent in the background |
| `toc runtime status [session-id]` | Check status of sub-agent sessions |
| `toc runtime output <session-id>` | Read the output of a completed sub-agent |
| `toc runtime invoke <integration> <action>` | Call an external API through the gateway |
| `toc runtime watch <session-id>` | Live-tail a sub-agent's session |
| `toc runtime replay <session-id>` | Replay session steps, tokens, and errors |
| `toc runtime cancel <session-id>` | Cancel a running sub-agent |

## Roadmap

- [x] Agent creation and isolated session spawning via a runtime provider
- [x] Snapshot sync — persist selected session outputs back to agent templates
- [x] Audit log — append-only JSON Lines log for compliance and traceability
- [x] Skills — reusable, shareable agent capabilities
- [x] Registry — unified catalog of skills and agent templates with search and install
- [x] Sub-agents — agents that spawn and coordinate other agents
- [x] Integrations — API gateway with scoped permissions, credential vault, and rate limiting
- [x] Native runtime (beta) — built-in agent loop via OpenRouter with pluggable models
- [ ] Session sandbox — unforgeable identity and permissions ([#12](https://github.com/louismorgner/tiny-oc/issues/12))
- [ ] Cost controls — per-agent and per-session spending limits
- [ ] Git backing for `.toc/` and agent sync
- [ ] **v1 release**

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
