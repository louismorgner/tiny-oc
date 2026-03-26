# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [unreleased]

### Added

- New `tlc-setup` skill: helps agents set up and configure the TLA+ TLC model checker with CLI-first, repo-friendly workflows for specs, config files, and CI automation.

## [0.3.3] - 2026-03-25

### Changed

- Slack integration switched from OAuth2 redirect flow to manual token-paste — Slack requires HTTPS on redirect URIs which broke localhost OAuth (#66).

### Fixed

- Removed duplicate setup URL display when adding token-based integrations (#66).

## [0.3.2] - 2026-03-25

### Added

- Interactive TUI dashboard for `toc status` with real-time session monitoring (#56).
- E2e smoke test suite with mock claude binary — 10 tests, deterministic, no API key needed (#59).
- E2e smoke tests in CI pipeline (#63).
- Auto-generated human-readable session names from prompts (#62).
- Assistant text messages now visible in `toc runtime watch` output (#58).

### Fixed

- Token usage not displaying in `toc status` (#60).
- Integration registry lookup now fetches from remote correctly (#55).

### Changed

- Replaced Codecov with inline `-cover` flag in CI (#64, #65).

## [0.3.0] - 2026-03-25

### Added

- Integration system: API gateway with rate limiting, credential vault, and permission scoping for external services (#38).
- Slack integration with OAuth2 flow, channel resolution, and error handling (#49).
- `toc runtime watch` to live-tail sub-agent sessions (#52).
- Sub-agent resume capability — resume interrupted sub-agent sessions (#45).
- Session replay with `toc runtime replay` and `--json` output for runtime commands (#29).
- `toc update` command for CLI self-update (#35).
- `toc agent show` command and improved `toc agent create` wizard (#22).
- `toc init --name` flag for non-interactive workspace initialization.
- Mini-claw agent template with compose system, template variables, and first-run bootstrap (#21).
- Agent template improvements based on replay observations (#34).

### Changed

- Unified permission model with hook enforcement — permissions are now declared in `oc-agent.yaml` and enforced consistently (#27).
- Runtime CLI hardened: status tracking, cancel support, partial output, file locking (#48).

### Fixed

- Sub-agent output capture race condition (#23).
- JSONL path resolution for sub-agent replay (#31).
- Status command now sorts sessions by most recent first (#42).
- Gateway array filtering, URL param leaking, rate limiter persistence, and permission matching (#48).

## [0.2.0] - 2026-03-24

### Added

- Sub-agent spawning system: agents can now spawn other agents as background tasks during a session.
  - New `sub-agents` field in `oc-agent.yaml` controls which agents can be spawned (explicit names or `"*"` wildcard).
  - New `toc runtime` commands for agents during sessions: `list`, `spawn`, `status`, `output`.
  - Environment variables (`TOC_WORKSPACE`, `TOC_AGENT`, `TOC_SESSION_ID`) injected into every session for runtime context.
  - Parent-child session tracking in `sessions.yaml` with `parent_session_id` and `prompt` fields.
- Session end hooks: new `on_end` field in `oc-agent.yaml` runs a prompt via Claude Code's `SessionEnd` hook before the session closes, useful for persisting context and memory.
- Composable agent instructions: new `compose` field in `oc-agent.yaml` lists files appended after `agent.md` when building `CLAUDE.md` at spawn time.
- Template variables in agent instructions: `{{.AgentName}}`, `{{.SessionID}}`, `{{.Date}}`, `{{.Model}}` are replaced at spawn time in `agent.md` and compose files.
- `toc agent add <name>` command to install agent templates from the registry.
- `toc skill add` now auto-detects registry names in addition to Git URLs.
- Cross-type error messages: `toc skill add <agent-name>` suggests `toc agent add` and vice versa.
- New `mini-claw` agent template: persistent agent with identity, memory, and session awareness inspired by OpenClaw.
- New `agentic-memory` skill: two-tier memory system with daily logs (`memory/YYYY-MM-DD.md`) and long-term storage (`memory/MEMORY.md`).
- Token usage tracking: `toc status` now shows per-session token usage (input, output, cache read/create) parsed from Claude Code JSONL logs.

### Changed

- Git hook injection prevention: all git clone operations now disable hooks via `-c core.hooksPath=/dev/null`.
- HTTPS-only enforcement for all skill and agent URLs.
- Session directories now use `os.MkdirTemp` for unpredictable paths (prevents symlink attacks).
- Audit log and session files hardened to 0600 permissions (owner-only read/write).
- HTTP client timeout (30s) added to registry fetches.
- UTF-8 safe truncation in skill/agent table display.

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

[unreleased]: https://github.com/louismorgner/tiny-oc/compare/v0.3.3...HEAD
[0.3.3]: https://github.com/louismorgner/tiny-oc/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/louismorgner/tiny-oc/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/louismorgner/tiny-oc/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/louismorgner/tiny-oc/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/louismorgner/tiny-oc/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/louismorgner/tiny-oc/releases/tag/v0.1.0
