# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.4] - 2026-03-30

### Fixed

- Fix Apify integration sending actor input in a nested `{"input": ...}` envelope instead of as the top-level request body — actors were receiving empty input and returning no results (#194).
- Fix Apify actor ID URL format in all 6 skill docs — use `~` separator (e.g. `apidojo~tweet-scraper`) instead of `/` which caused 404s (#194).

### Added

- Gateway support for `body_path: "."` on integration params — lets a JSON param's parsed value become the entire request body.

## [0.7.3] - 2026-03-30

### Fixed

- Fix staircase/waterfall rendering in raw-mode interactive TUI — newlines now correctly return to column 0 (#191).

## [0.7.2] - 2026-03-30

### Added

- Full Twitter/X OAuth2 flow with PKCE via the hosted callback relay — `toc integrate add twitter` now opens the browser and captures tokens automatically, matching the Slack experience (#190).

## [0.7.1] - 2026-03-30

### Added

- Official toc-native support for anthropic/claude-haiku-4.5 and anthropic/claude-haiku-4.6 with pricing (#189).

### Fixed

- Fix analyze-x SKILL.md frontmatter that broke `toc agent add x-grower` — YAML array in metadata field could not unmarshal into string (#189).

## [0.7.0] - 2026-03-30

### Added

- X-grower agent template for X/Twitter growth automation (#188).
- Co-founder agent with skills and Apollo integration (#186).
- Runtime-enforced post-turn triggers — agents can declare hooks that fire after every assistant turn (#184).
- Runtime-aware sub-agent instructions template (#185).
- Apify integration and link import skills for content-writer agent (#181).
- Network web access for content-writer agent (#174).
- Show individual todo items in TodoWrite CLI output (#176).

### Improved

- Summarize WebFetch content via small model to reduce context usage (#183).
- Reduce TodoWrite overhead in short sessions (#182).

### Fixed

- Alt+Backspace word deletion in interactive CLI (#175).
- Write tool permission bypass on 0-byte writes (#177).

## [0.6.5] - 2026-03-29

### Added

- Content-writer agent template to registry (#173).
- Registry workspaces for multi-agent installs (#170).
- Poll for sub-agent notifications in interactive loop (#169).
- Thinking/reasoning support for toc-native runtime (#165).
- Official toc-native support for anthropic/claude-opus-4.6 (#168) and anthropic/claude-sonnet-4.6 (#167).
- Native runtime profiles for GLM-5, Kimi K2.5, MiMo-V2-Flash, and Qwen 3.5 397B (#159).
- Session inspect and compare tooling (#153).
- WebFetch native tool with network permission gate (#155).
- Show estimated cost next to token count in toc status (#161).
- Detached question CLI workflow (#148).
- toc-native runtime architecture doc (#150).
- Research folder with coding agent system prompts (#151).

### Changed

- Switch superfounder agents to open-source models via OpenRouter (#158).
- Tighten superfounder agent handoffs and review loops (#162).

### Fixed

- GLM-5 MaxOutputTokens exceeding context window, add InputBudget guard (#163).
- Fix toc stop: write PID files for interactive sessions (#164).
- Native runner self-recovery from tool execution timeouts (#166).
- Inspect: gzip capture and Anthropic cache token counting (#157).

## [0.6.0] - 2026-03-27

### Added

- Question tool for the native runtime — agents can now prompt users for input mid-session (#137).
- TodoWrite session tracking — native agents can create and manage task lists within sessions (#144, #147).
- `small_model` support in toc-native compaction — use a cheaper model for context compaction (#145).
- `TOC_NATIVE_BASE_URL` environment variable to configure the toc-native API endpoint (#143).
- Claude Code default model alias support in toc-native (#146).

## [0.5.2] - 2026-03-27

### Added

- Debug trace mode for toc-native runtime (#136).
- Raised default max iterations to 126 with user prompt on limit (#126).
- `--resume` instructions added to superfounder agent templates (#124).

### Fixed

- Log HTTP status, model, endpoint, and response body on OpenRouter errors (#125).

## [0.5.1] - 2026-03-26

### Added

- `anthropic/claude-opus-4.6` added to toc-native model registry (#123).
- Superfounder agent group: product-founder, cto, swe templates (#122).

## [0.5.0] - 2026-03-26

### Added

- Redesigned context management with token budgets, pruning, and structured handoff (#114).
- `toc stop` and `toc runtime stop` commands for killing stuck sessions (#108).
- `toc watch` command (workspace-level) with shared logic (#109).
- First-class skill support in toc-native, agentskills.io spec (#120).
- Sub-agent completion notifications (#121).
- Integration activation TUI after adding integrations (#119).
- Exa extract action for structured data extraction (#117).
- Configurable max tool iterations via `--max-iterations` flag (#111).
- Per-request token tracking alongside cumulative (#112).
- GPT-5.4 and GPT-5.3-codex model pricing (#112).

### Improved

- Token optimization: per-tool output budgets, middle-out truncation, cache breakpoints (#105).
- Debug diagnosis engine: verdicts, exit signals, last words, tool timings, cost estimates (#106).
- Debug prefix matching on session IDs (#109).
- Slack setup wizard with foolproof step-by-step OAuth guidance (#107).

## [0.4.5] - 2026-03-26

### Added

- Show active integrations in session startup banner (#104).

## [0.4.4] - 2026-03-26

### Changed

- Eliminate config duplication, add unit tests (#103).

### Fixed

- Exa integration test returning 400 (#102).
- Resolve credentials from workspace root, not CWD (#101).

## [0.4.3] - 2026-03-26

### Changed

- Update implementer agent to use openai/gpt-5.3-codex (#99).

### Fixed

- Compaction produces invalid messages for OpenAI models via OpenRouter (#100).

## [0.4.2] - 2026-03-26

### Added

- Implementer agent template for 3-tier hierarchy (#97).
- `toc debug` command and native crash logs (#96).
- Slack capability permission model (#95).
- `toc-setup` agent skill (#94).
- Exa semantic search integration (#88).
- Runtime invoke discoverability and structured errors (#86).
- Integrations and runtimes docs, updated architecture and README (#83, #87).
- Integration permissions vision doc (#89).

### Changed

- Enable OpenRouter prompt caching for native runtime (#82).
- Improved Slack OAuth2 onboarding UX (#85).
- Removed hand-written sub-agents section from CTO agent prompt (#91).

### Fixed

- CI: mock keychain in smoke tests for Linux compatibility (#98).
- Detached session resume using wrong CLI flag (#84).

## [0.4.1] - 2026-03-25

### Fixed

- Worker URL to use toc-auth-callback subdomain (#81).

## [0.4.0] - 2026-03-25

### Added

- `toc-native` beta runtime foundation (#71).
- Input/output token breakdown in runtime status (#76).
- HTTPS OAuth callback relay for Slack (#72).
- Non-interactive flag support for all interactive CLI commands (#73).

### Changed

- Switched Slack integration from bot tokens to user tokens via OAuth2 (#69).

### Fixed

- Word-deletion (Ctrl+Backspace) in interactive prompts (#70).
- Show initial user message in watch and replay output (#68).
- Sort active sessions to top in `toc status` (#67).
- Wrangler route config: add zone_name (#77).
- Workers.dev domain for OAuth callback relay (#78).

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

[unreleased]: https://github.com/louismorgner/tiny-oc/compare/v0.7.3...HEAD
[0.7.3]: https://github.com/louismorgner/tiny-oc/compare/v0.7.2...v0.7.3
[0.7.2]: https://github.com/louismorgner/tiny-oc/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/louismorgner/tiny-oc/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/louismorgner/tiny-oc/compare/v0.6.5...v0.7.0
[0.6.5]: https://github.com/louismorgner/tiny-oc/compare/v0.6.0...v0.6.5
[0.6.0]: https://github.com/louismorgner/tiny-oc/compare/v0.5.2...v0.6.0
[0.5.2]: https://github.com/louismorgner/tiny-oc/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/louismorgner/tiny-oc/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/louismorgner/tiny-oc/compare/v0.4.5...v0.5.0
[0.4.5]: https://github.com/louismorgner/tiny-oc/compare/v0.4.4...v0.4.5
[0.4.4]: https://github.com/louismorgner/tiny-oc/compare/v0.4.3...v0.4.4
[0.4.3]: https://github.com/louismorgner/tiny-oc/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/louismorgner/tiny-oc/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/louismorgner/tiny-oc/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/louismorgner/tiny-oc/compare/v0.3.3...v0.4.0
[0.3.3]: https://github.com/louismorgner/tiny-oc/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/louismorgner/tiny-oc/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/louismorgner/tiny-oc/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/louismorgner/tiny-oc/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/louismorgner/tiny-oc/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/louismorgner/tiny-oc/releases/tag/v0.1.0
