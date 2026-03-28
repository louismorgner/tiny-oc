# Runtimes

A runtime is the execution engine that runs an agent session. toc abstracts over runtimes through a provider interface, so the same agent definition can target different backends.

## Supported runtimes

| Runtime | Status | Description |
|---|---|---|
| `claude-code` | Stable | Launches Claude Code CLI sessions |
| `toc-native` | Beta | Built-in agent loop via OpenRouter |

Set the runtime in `oc-agent.yaml`:

```yaml
runtime: claude-code
```

## Provider interface

All runtimes implement the same contract:

- **PrepareSession** — materialize instructions, hooks, and runtime-specific files in the session workspace
- **SkillsDir** — return the directory where skills should be placed for this runtime
- **LaunchInteractive** — start an interactive session
- **LaunchDetached** — start a background session (for sub-agents)
- **PostSessionSync** — run the final snapshot sync pass after the session ends
- **ParseSessionLog** — normalize runtime-specific logs into toc's `Event` format

This means agents, permissions, skills, snapshot sync, and sub-agents work the same way regardless of runtime. The differences are in how the session is executed and how the model is called.

## Claude Code runtime

The default runtime. Delegates session execution to the `claude` CLI.

### How it works

1. **Instruction materialization**: `agent.md` + compose files are assembled into `CLAUDE.md` with template variable substitution
2. **Hook generation**: toc writes `.claude/settings.json` with hooks for snapshot sync (`PostToolUse`), permission enforcement (`PreToolUse`), and end-of-session behavior (`SessionEnd`)
3. **Launch**: executes `claude --model <model> --session-id <uuid>` as a subprocess
4. **Skills**: placed in `.claude/skills/` where Claude Code discovers them automatically
5. **Resume**: uses `claude --resume <session-id>`

### Hooks

The Claude Code runtime generates up to three hook types:

| Hook | Event | Purpose |
|---|---|---|
| `PostToolUse` | After `Edit`, `Write`, `MultiEdit` | Copies matching files back to parent snapshot |
| `PreToolUse` | Before any tool call | Enforces filesystem permissions (block/allow/ask) |
| `SessionEnd` | Session closes | Runs the `on_end` prompt with full tool access |

These hooks are shell scripts and JSON config generated in the session's `.claude/` directory. They don't affect your project.

**PostToolUse sync script**: Reads the tool invocation payload from stdin, extracts the file path, checks it against context patterns, and copies matching files back to the agent template.

**PreToolUse permission script**: Maps tools to permission categories (Read/Glob/Grep to `read`, Edit/Write to `write`, Bash to `execute`), checks the configured permission level, and outputs a JSON decision (`{"decision":"allow"}` or `{"decision":"block","reason":"..."}`).

**SessionEnd hook**: If `on_end` is set, registers an `agent`-type hook that runs the prompt with full tool access before the session closes.

### Models

| Model | Description |
|---|---|
| `default` | Claude Code default for the current account tier |
| `sonnet` | Claude Sonnet |
| `opus` | Claude Opus |
| `haiku` | Claude Haiku |

## Native runtime (beta)

`toc-native` is a built-in agent loop that calls models through OpenRouter. It runs the tool loop directly instead of delegating to an external CLI.

For a deeper implementation walk-through, see [toc-native Runtime](toc-native-runtime.md).

### Current scope

The native beta supports local tools plus first-class public URL viewing:

- File reads and writes
- File edits (string replacement)
- Glob and grep
- Shell execution
- Public URL fetch with HTML-to-Markdown conversion
- Skill reads
- Todo tracking with `TodoWrite`
- Sub-agent management

Authenticated integrations (`toc runtime invoke`) and browser automation are not yet part of the native tool loop.

### How it works

1. **Instruction materialization**: `agent.md` + compose files are written to `.toc-native/system-prompt.md`
2. **No hooks**: permissions are enforced directly at tool execution time, not through external scripts
3. **Launch**: runs the native agent loop as a subprocess (`toc __native-run`)
4. **State**: persists full message history, token usage, and turn checkpoints to `.toc/sessions/<id>/state.json` for resume
5. **Events**: writes directly to `.toc/sessions/<id>/events.jsonl` in toc's normalized format
6. **Skills**: placed in `.toc-native/skills/`

### TodoWrite

The native runtime includes a `TodoWrite` tool for session-scoped task tracking.

- `TodoWrite` replaces the full todo list on each call. It is not incremental.
- Todos are persisted in `.toc/sessions/<id>/state.json` alongside the rest of the native session state.
- The current todo list is injected back into model context on each turn, so it stays available even as the conversation grows.
- `TodoWrite` is intended for the primary agent. Spawned native sub-agents do not receive the tool.

The todo item schema is:

```json
{
  "content": "Brief description of the task",
  "status": "pending | in_progress | completed | cancelled",
  "priority": "high | medium | low"
}
```

For multi-step work, the model is expected to keep the list short, keep at most one item `in_progress` when possible, and rewrite the full list whenever the plan changes.

### Models

Native runtime models are served through OpenRouter. Supported profiles:

| Model ID | Label |
|---|---|
| `openai/gpt-4o-mini` | GPT-4o Mini (default) |
| `openai/gpt-4o` | GPT-4o |
| `anthropic/claude-sonnet-4` | Claude Sonnet 4 |

To use a model outside this set, opt in explicitly:

```yaml
runtime: toc-native
model: some/other-model
small_model: some/other-small-model
allow_custom_native_model: true
```

Custom models must support tool calling to work with the native runtime.

`toc-native` also supports an optional `small_model` field. When set, the runtime uses that model for lightweight compaction summarization work and falls back to `model` for the main tool loop.

### Configuration

By default, the native runtime reads an OpenRouter API key from the environment (`OPENROUTER_API_KEY`) or from `.toc/secrets.yaml`:

```bash
toc config set openrouter_key sk-or-...
```

To point only `toc-native` at a custom OpenRouter-compatible base URL, set `TOC_NATIVE_BASE_URL`. This is useful for local reverse proxies:

```bash
# Start a reverse proxy in front of your OpenRouter-compatible endpoint
mitmweb --mode reverse:https://openrouter.ai --listen-port 8000

# Run toc-native through the proxy
OPENROUTER_API_KEY=sk-or-... \
TOC_NATIVE_BASE_URL=http://localhost:8000 \
toc agent spawn my-agent
```

`TOC_NATIVE_BASE_URL` takes precedence over `OPENROUTER_BASE_URL`, and only affects `toc-native`.

### State and resume

The native runtime persists state after each model turn to `.toc/sessions/<id>/state.json`. This includes the full message history, token usage, and a turn checkpoint for crash recovery. Resume loads this state and continues the conversation.

### Context window management

The native runtime actively manages context to maintain model quality over long sessions. It uses a multi-stage pipeline driven by token budgets derived from the model's context window:

1. **Token budget evaluation**: Before each model call, the runtime estimates input tokens and compares against the model's available input budget (context window minus output reservation minus tool overhead).
2. **Pruning**: When usage exceeds 75% of the input budget, stale tool outputs from older turns are pruned. Error outputs and diffs are protected from pruning.
3. **Structured compaction**: When usage exceeds 90% of the input budget (or pruning alone is insufficient), older messages are replaced with a structured continuation artifact that captures goal, decisions, working files, completed work, and open loops.
4. **Fail-safe**: If context remains over budget after emergency compaction, the runtime returns an error rather than sending an over-budget request.

The continuation artifact replaces the old freeform summary with structured fields that help the model resume effectively after compaction.

If `small_model` is configured, the continuation artifact is synthesized with that model instead of the primary `model`, which reduces cost and latency for compaction turns.

**Working set tracking**: The runtime tracks files read, edited, and written, along with recent shell commands. This metadata feeds into compaction and diagnostics.

Session config defaults:

- Keep recent: 12 messages (retained after compaction)
- Max continuation: ~6,000 characters

For models with known context windows (GPT-4o: 128K, Claude Sonnet 4: 200K), token budgets are computed automatically. Custom models use a conservative 128K default.

## Choosing a runtime

Use `claude-code` when you want the full Claude Code experience — its native tools, MCP support, and interactive terminal UI. This is the stable, production-ready path.

Use `toc-native` when you want to experiment with non-Anthropic models or want toc to own the full agent loop. The native runtime is in beta — expect rough edges and a narrower tool set.
