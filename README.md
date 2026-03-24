# toc — tiny open company

A local CLI tool for managing and spawning AI agent sessions from reusable templates.

Define agents as simple YAML configs, then spawn isolated sessions instantly. Built for engineers who want fast, reproducible agent workflows without cloud dependencies.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/tiny-oc/toc/main/get-toc.sh | bash
```

Downloads the latest prebuilt binary for your platform. Supports macOS and Linux (amd64/arm64).

Requires [Claude Code](https://docs.anthropic.com/en/docs/claude-code) to spawn agent sessions.

### Build from source

```bash
git clone https://github.com/tiny-oc/toc.git
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
2. Launches a Claude Code session with the configured model
3. Syncs context files back to the agent template in real-time (if configured)
4. Tracks the session so you can resume it later

## Documentation

- [Getting started](docs/getting-started.md) — install, create your first agent, spawn a session
- [Configuration reference](docs/configuration.md) — all config fields, context sync patterns, audit log format
- [Skills guide](docs/skills.md) — create, install, and attach reusable capabilities
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
| `toc skill add <url>` | Install a skill from a Git URL |
| `toc skill add --registry <name>` | Install a skill from the registry |
| `toc skill remove <name>` | Remove a skill |
| `toc registry search [query]` | Browse skills in the registry |
| `toc audit` | View the audit log |
| `toc completion <shell>` | Generate shell completion script |

## Roadmap

- [x] Agent creation and isolated session spawning via Claude Code
- [x] Context sync — persist session outputs back to agent templates
- [x] Audit log — append-only JSON Lines log for compliance and traceability
- [x] Skills — reusable, shareable agent capabilities
- [ ] Skills registry — browsable catalog of installable skills
- [ ] Agent registry — browsable catalog of agent templates
- [ ] Sub-agents — agents that spawn and coordinate other agents
- [ ] Cost controls — per-agent and per-session spending limits
- [ ] Integrations and permissions — connect agents to external tools with scoped access
- [ ] **v1 release**

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
