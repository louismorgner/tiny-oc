# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-03-24

### Added

- `toc init` to initialize a workspace with a `.toc/` directory.
- `toc agent create` with interactive prompts for name, description, model, context patterns, and agent instructions.
- `toc agent list` to display configured agents in a table.
- `toc agent spawn <name>` to copy an agent template to an isolated temp directory and launch a Claude Code session.
- `toc agent spawn <name> --resume <id>` to resume a previous session.
- `toc agent remove <name>` to delete an agent and its sessions.
- `toc status` with agent config validation (green/red indicators).
- Context sync: files matching `context:` patterns in `oc-agent.yaml` sync back from sessions to the agent template via Claude Code PostToolUse hooks and a post-session fallback pass.
- Audit log: append-only JSON Lines log at `.toc/audit.log` tracking all actions with timestamp, actor, hostname, and details.
- `toc audit` to view the log with `--tail`, `--action`, and `--json` flags.
- `toc completion` for bash, zsh, and fish with dynamic completion of agent names and session IDs.
- `install.sh` for building and symlinking the binary to PATH.

[unreleased]: https://github.com/tiny-oc/toc/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tiny-oc/toc/releases/tag/v0.1.0
