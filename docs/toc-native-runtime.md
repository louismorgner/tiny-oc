# toc-native Runtime

This document explains how the `toc-native` runtime works today, why it is structured the way it is, and where the current beta boundary still is.

It is meant to be the review baseline for the native runtime. The goal is not just to describe code paths, but to make the architecture legible enough that we can change it deliberately.

For runtime-neutral vocabulary, see [Core concepts](core-concepts.md). For the provider abstraction and user-facing runtime config, see [Runtimes](runtimes.md) and [Configuration reference](configuration.md).

## What toc-native is

`toc-native` is toc's built-in agent runtime. Unlike `claude-code`, which delegates the session loop to an external CLI, `toc-native` owns the loop itself:

- it composes the system prompt
- calls the model through OpenRouter
- exposes tools directly to the model
- persists state for resume and crash recovery
- writes normalized events itself
- manages sub-agent coordination and context compaction

That is the main architectural difference. `claude-code` integrates with another runtime. `toc-native` is the runtime.

## Design center

The native runtime is built around a few choices that show up repeatedly in the code.

### toc owns the session contract

At spawn time, toc resolves `oc-agent.yaml` into `.toc/sessions/<id>/session.json`. That resolved session config is the contract the runtime runs against. A live session does not keep re-reading the agent definition.

This matters because it gives us a stable input boundary:

- agent config expresses intent
- session config freezes that intent for one session
- runtime internals are derived artifacts, not a second source of truth

### Keep runtime state explicit and file-backed

The native runtime leans heavily on plain files under `.toc/sessions/<id>/`:

- `session.json` for the resolved contract
- `permissions.json` for the resolved permission manifest
- `state.json` for native runtime state
- `events.jsonl` for normalized observability
- `stderr.log` for diagnostics
- `trace.jsonl` for optional request/response tracing
- `notifications/` for sub-agent completion messages

This is less elegant than a long-lived daemon, but it is easy to inspect, easy to recover, and easy to support with simple CLI commands.

### Internalize what Claude handles with hooks

The Claude runtime relies on generated hooks like `PreToolUse`, `PostToolUse`, and `SessionEnd`. The native runtime does not generate equivalent hook scripts.

Instead, the hook behavior is pulled into Go code:

- permission checks happen directly inside tool handlers
- event writing happens in the turn loop
- end-of-session behavior runs through `finalizeNativeSession`
- sub-agent completion is delivered through a toc-owned notification queue

This is one of the bigger design decisions in the project. The native runtime is moving behavior from "external runtime integration" to "first-class runtime implementation."

### Prefer small file protocols over service infrastructure

Two examples:

- The `Question` tool in non-interactive mode writes `question.json` and waits for `answer.json`.
- Approval flow for integration invocations writes request files and waits for response files.

This is not the final shape of everything, but it keeps the system understandable while the runtime surface is still moving.

## Where toc-native sits in the session lifecycle

The native runtime is one provider behind the shared runtime interface. The lifecycle starts in [`internal/spawn/spawn.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/spawn/spawn.go).

### Spawn flow

1. Load the agent config from `.toc/agents/<name>/`.
2. Resolve it into a `SessionConfig`.
3. Create a new temp session workspace under `/tmp/toc-sessions/...`.
4. Copy the parent agent snapshot into that workspace.
5. Write `.toc/sessions/<id>/session.json`.
6. Call `nativeProvider.PrepareSession(...)`.
7. Resolve skills into `.toc-native/skills/`.
8. Write `.toc/sessions/<id>/permissions.json`.
9. Track the session in `sessions.yaml`.
10. Launch `toc __native-run`.

### Native-specific preparation

[`internal/runtime/native.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/native.go) does very little session preparation compared with the Claude provider:

- create `.toc-native/skills/`
- compose `agent.md` plus any `compose` files
- write `.toc-native/system-prompt.md`

That is intentional. The native runtime does not need generated shell hooks or a runtime-owned config directory beyond prompt and skills.

## Bootstrapping a native session

The runtime entrypoint is [`cmd/runtime_native_run.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/cmd/runtime_native_run.go), which calls `RunNativeSession(...)` in [`internal/runtime/native_runner.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/native_runner.go).

Boot does the following:

1. Load or create `state.json`.
2. Load `session.json`.
3. Load `permissions.json`.
4. Fill in missing state fields like model and prompt.
5. Recover from any interrupted turn checkpoint.
6. Ensure the system prompt is present in message history.
7. Build the OpenRouter client from environment or workspace secrets.
8. Build the enabled tool set from the session config.
9. Enter one of three paths:
   - foreground prompt execution
   - detached prompt execution
   - interactive REPL loop

### State bootstrap and recovery

`BootstrapNativeState(...)` is the first important seam.

It loads persisted state if it exists, updates mutable runtime fields like session dir and mode, then calls `recoverInterruptedTurn(...)`.

Recovery is simple on purpose:

- if `PendingTurn` exists, the runtime assumes the prior turn was interrupted
- it clears the checkpoint
- increments recovery counters
- marks the session as `interrupted`
- appends a `recovery` event

This is not transactional replay. It is pragmatic crash recovery that preserves enough information to resume safely without pretending we know exactly which side effects already happened.

## Prompt materialization and runtime notes

The actual prompt assembly is runtime-neutral. [`internal/runtime/prompt.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/prompt.go) reads:

- `agent.md`
- then each file listed in `compose`

It joins them with `---` separators and resolves a small template surface:

- `{{.AgentName}}`
- `{{.SessionID}}`
- `{{.Date}}`
- `{{.Model}}`

That output becomes `.toc-native/system-prompt.md`.

At runtime boot, `ensureSystemPrompt(...)` loads that file and may append two extra layers before inserting it as the first message:

- a generated skill catalog for provisioned skills
- runtime notes, currently used for `TodoWrite`

The skill catalog is important. Skills are not automatically in context as full documents. The model first sees a catalog of available skills, then must use the `Skill` tool to load a specific `SKILL.md`.

That keeps the base prompt smaller and makes skill use explicit.

## The core agent loop

The center of the runtime is `runNativeLoop(...)` in [`internal/runtime/native_runner.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/native_runner.go).

The loop is conventional in shape, but there are a few toc-specific choices worth calling out.

### Per-turn flow

For each iteration:

1. Run context management before the model call.
2. Build a curated context view from persisted state.
3. Save a `PendingTurn` checkpoint with phase `awaiting_model`.
4. Call OpenRouter with messages and tool definitions.
5. Accumulate usage counters.
6. Append the assistant message to state and transcript.
7. If there are no tool calls, return turn completion.
8. If there are tool calls:
   - save a new `PendingTurn` checkpoint with phase `executing_tools`
   - execute each tool synchronously
   - append a normalized event for each result
   - append a `tool` message to conversation state
   - update the working set
   - shrink the checkpoint as tools finish

The runtime currently executes tool calls serially. There is no internal tool parallelism inside one model turn.

### Transcript vs current message state

`State` stores both:

- `Messages`: the active conversation state used for future model calls
- `Transcript`: the full append-only conversation history

This split is important once compaction starts. `Messages` can be pruned or replaced with continuation artifacts. `Transcript` preserves the original interaction history for inspection and debugging.

### Turn checkpoints

`PendingTurn` is one of the more useful pieces of state in the native runtime. It gives us a lightweight answer to "what was this session doing when it died?"

The checkpoint stores:

- current phase
- latest prompt
- pending tool calls
- start time

That is used in:

- resume and interrupted-turn recovery
- debug output
- crash reporting

## Tool architecture

Tools are registered in [`internal/runtime/native_tool_registry.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/native_tool_registry.go). Each tool has:

- a public name
- a long natural-language description
- a JSON Schema-like parameter spec
- a Go handler

This registry drives two things at once:

- the tool definitions sent to the model
- the dispatch table used at execution time

That keeps the advertised tool surface and the executable tool surface in one place.

### Current tool set

The native runtime currently exposes these first-class tools:

- `Read`
- `Write`
- `Edit`
- `Glob`
- `Grep`
- `Bash`
- `Skill`
- `TodoWrite`
- `Question`
- `SubAgent`

Tool handlers live mostly in [`internal/runtime/native_tools.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/native_tools.go) and [`internal/runtime/native_tool_subagent.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/native_tool_subagent.go).

### Why the tool descriptions are long

The tool descriptions are unusually explicit. That is deliberate.

They do three jobs:

1. teach the model the intended tool choice policy
2. encode anti-patterns like "do not use Bash for file reads"
3. reduce prompt drift across different foundation models

In practice, the native runtime needs stronger tool guidance than Claude Code because it does not inherit another runtime's opinionated tool UX.

### Filesystem model

The local file tools work against the isolated session workspace, not the parent agent snapshot directly.

That means:

- edits are made in the temp session copy
- parent snapshot sync is still a separate concern
- the runtime can reject path escapes cleanly

This is consistent with toc's session model everywhere else. The runtime loop operates on the session workspace. Persistence back to the agent template is an explicit lifecycle step.

### Tool output shaping

Tool output is aggressively bounded before it is put back into conversation state.

Important details:

- output is truncated per tool type, not with one global limit
- truncation uses middle truncation, not simple tail cut
- some tools get larger budgets because their output tends to carry more signal

Examples:

- `Read` gets a larger budget because code and config reads are often real context
- `Bash` gets a moderate budget because logs grow fast
- `Glob` gets a small budget because long file listings usually are not worth carrying forward

This is one of the runtime's quiet but important quality controls. Without it, context pressure would get ugly quickly.

## Permissions and the absence of hooks

This is the cleanest place to compare `toc-native` with `claude-code`.

### Claude runtime

Claude uses generated hooks:

- `PreToolUse` for permissions
- `PostToolUse` for snapshot sync
- `SessionEnd` for `on_end`

### Native runtime

Native does not generate hooks. It handles the same concerns in-process:

- permissions in the tool handlers
- snapshot sync in post-session logic
- `on_end` by running another native prompt in `finalizeNativeSession(...)`

This is a better fit for a toc-owned loop because it removes a lot of shell-script indirection and runtime-specific behavior.

### Current permission behavior

The native tool path currently has one important limitation: filesystem permission level `ask` behaves like a denial for native local tools.

That is not just a doc caveat. It follows directly from `ValidateFilesystemPermission(...)`:

- `on` allows execution
- `off` blocks execution
- `ask` returns an error immediately

There is approval infrastructure in the repo, but today it is wired to `toc runtime invoke` style integration approval, not to native local tools.

So the real current-state story is:

- native local tools support allow or deny
- native local tools do not yet support interactive approval
- integration approval has a file-backed request/response path, but it lives outside the native tool loop

That is worth being explicit about because the config surface already exposes `ask`, which suggests a capability the native local runtime does not fully implement yet.

## Snapshot sync in the native runtime

The native runtime does not do live per-tool sync back to the parent snapshot the way Claude does with `PostToolUse`.

Instead:

- the runtime works inside the isolated session workspace
- detached sessions run a final sync during `finalizeNativeSession(...)`
- top-level spawn/resume also runs post-session sync after the runtime exits

This is a simpler model, but it is a different behavioral contract from Claude's near-real-time sync.

The upside is less moving machinery inside the turn loop. The downside is that the native runtime currently leans more on end-of-session persistence than live propagation.

## State, events, and observability

One design choice that works well in the native runtime is the separation between state and observability.

### `state.json`

`state.json` is for recovery and runtime continuity. It includes:

- runtime status
- model
- token usage
- active messages
- full transcript
- todos
- continuation artifact
- working set
- pending turn checkpoint
- crash info

This file is the source of truth for resume.

### `events.jsonl`

`events.jsonl` is the runtime's normalized event stream. It records things like:

- assistant text
- tool executions
- recovery events
- compaction events
- errors
- crashes

This is the cross-runtime observability format, not just a native detail.

The separation matters because state answers "how do I continue?" while events answer "what happened?"

### `stderr.log` and `trace.jsonl`

Two more layers help when things go wrong:

- `stderr.log` captures runtime stderr
- `trace.jsonl` can persist per-turn OpenRouter request/response payloads when tracing is enabled

`trace.jsonl` is opt-in through `--trace` or `TOC_TRACE=1`. That is a useful compromise: deep visibility is available when needed, but we do not pay the cost for every session.

### Crash preservation

The runtime also tries to preserve crash context. `runNativeLoop(...)` has a panic recovery block that stores:

- panic message
- stack trace
- last tool call
- crash timestamp

There is also a secondary diagnostic path that can recover panic information from `stderr.log` or output files for zombie sessions.

Again, the theme is pragmatic durability over perfect elegance.

## Context management and compaction

This is one of the more opinionated parts of the native runtime.

The runtime does not just keep appending messages until the provider rejects the request. It tries to manage context proactively.

### Budget model

[`internal/runtime/context_budgeter.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/context_budgeter.go) derives an input budget from the model profile:

- context window
- reserved output tokens
- reserved buffer for tool definitions and framing

From that, it defines thresholds:

- 75 percent: start pruning stale tool output
- 90 percent: compact old history into a continuation artifact
- 98 percent: fail-safe boundary

Unknown custom models fall back to conservative defaults.

### Curated context view

The runtime does not send raw `state.Messages` straight to the model. It first builds a context view with `BuildContextView(...)`.

That gives us a clean seam for injecting runtime-owned context such as:

- current todo list
- working set summary
- continuation artifacts

This is subtle but important. Persisted state and model-facing context are related, but they are not identical.

### Working set tracking

The runtime tracks recent:

- files read
- files edited
- files written
- bash commands
- sub-agent actions

This working set is updated incrementally from completed tool calls and then injected back into the model context as a compact summary.

That is a good example of toc doing agent runtime work that most LLM wrappers skip. The runtime is trying to preserve the session's local sense of "what have I been touching?" without replaying every full tool result.

### Structured continuation artifacts

When pruning is not enough, the runtime compacts old messages into a structured continuation artifact instead of a freeform summary.

The artifact can include:

- goal
- constraints
- decisions
- discoveries
- working files
- completed work
- remaining work
- open loops
- next steps

This is currently stored both:

- in `state.Continuation`
- and as a synthetic conversation message prefixed with `[toc-continuation]`

If `small_model` is configured, the runtime prefers it for compaction synthesis. Otherwise it uses the main model. If the LLM-generated continuation fails, it falls back to a heuristic builder.

That fallback matters. Compaction is a control-plane responsibility. It cannot depend on the summarizer path being perfect.

## Sub-agents and session notifications

Sub-agents are a first-class part of the native runtime, but they are intentionally isolated.

### Spawn model

The `SubAgent` tool does not launch a goroutine inside the parent runtime. It asks the spawn layer to create a real child session:

- separate temp workspace
- separate session metadata
- separate state
- separate output file
- separate process

This keeps the mental model clean. A sub-agent is another toc session, not just a nested loop.

### Tool restrictions for native sub-agents

When spawning a native sub-agent, the spawn layer removes:

- `TodoWrite`
- `SubAgent`

That prevents recursive coordination weirdness and keeps the child runtime simpler.

### Completion flow

Detached runs use a small wrapper script that:

- writes a PID file
- captures output
- writes an exit code file
- notifies the parent on completion

Notification delivery is file-backed. A completed child writes a notification JSON file into the parent's session metadata. The parent runtime polls for notifications and turns a completion into a new prompt that says, in effect, "here is the sub-agent result; continue."

This is simple, but it fits the rest of the architecture well:

- no broker
- no daemon
- no in-memory dependency between parent and child

## Interactive mode, detached mode, and questions

The runtime supports three operating styles:

- interactive foreground session
- foreground prompt execution
- detached background execution

That split shows up in a few places:

- interactive mode intercepts `SIGINT` and exits gracefully
- detached mode waits for notifications then finalizes automatically
- `Question` can read directly from stdin in TTY mode, or fall back to question/answer files in non-interactive mode

The `Question` fallback is rough, but it is consistent with the runtime's preference for explicit file-backed coordination.

There is one current-state wrinkle here: the tool implementation supports that non-interactive fallback, but the tool description still frames `Question` as interactive-only. That means the runtime behavior is ahead of the prompt surface.

## Model and provider layer

The model backend lives in [`internal/runtime/openrouter.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtime/openrouter.go).

Key characteristics:

- reads API key from env or `.toc/secrets.yaml`
- supports `TOC_NATIVE_BASE_URL` for OpenRouter-compatible endpoints
- supports both normal and streaming chat completions
- retries transient network and server failures
- turns provider failures into resume-friendly user errors

On top of that, [`internal/runtimeinfo/native_models.go`](/Users/louismorgner/conductor/workspaces/tiny-oc/auckland/internal/runtimeinfo/native_models.go) defines native model profiles. These profiles are not just a picker list. They carry runtime assumptions:

- tool support
- streaming support
- context window
- max output tokens
- reserved buffer

That means model selection is not just a product choice. It shapes context budgeting and runtime behavior.

## Why the runtime is structured this way

A few decisions are worth stating plainly.

### Why state and events are separate

Because they answer different questions.

- state is for continuity
- events are for inspection

Combining them would make resume logic noisier and observability less stable.

### Why there are no generated native hooks

Because once toc owns the loop, hooks stop being the cleanest abstraction. Direct code paths are easier to test, easier to reason about, and less dependent on shell behavior.

### Why tools are local-only in the beta

Because the runtime still needs a clean first-class contract for external integrations:

- permission semantics
- observability semantics
- approval semantics
- failure and retry semantics

Local tools were the smallest useful slice that let toc own the loop without inheriting all of the gateway complexity on day one.

### Why compaction uses structure instead of prose

Because freeform summaries degrade under repetition. Structured continuation gives the model a more reliable way to recover intent, files, and open loops after history is compressed.

### Why file-backed coordination shows up everywhere

Because it is inspectable and failure-tolerant. Polling JSON files is not glamorous, but it matches the current scale of the system and keeps operational behavior visible.

## Current state of the runtime

The native runtime is already more than a thin experimental shell. It has:

- a real persisted session contract
- native tool calling
- resume and interrupted-turn recovery
- context compaction
- sub-agent orchestration
- normalized events
- optional request tracing

That said, the beta boundary is still real.

### What feels structurally solid

- The split between spawn, session config, runtime state, and event log.
- The tool registry pattern.
- The context budgeter plus structured continuation path.
- The sub-agent model as separate detached sessions.
- The general "toc owns the control plane" direction.

### What is still clearly incomplete

- Native local tools do not yet implement true approval for `ask`.
- External integrations are not first-class native tools.
- Snapshot sync is simpler than Claude's live hook-driven model and currently leans on post-session sync.
- Coordination still uses polling file protocols in a few places.
- The tool loop is serial and conservative.
- Some user-facing strings still carry Claude-era assumptions. One example: spawn output still says permissions are "enforced via hooks" even when the native runtime is doing the enforcement in-process.

### What this means for review

The right review frame is not "is this finished?" It is "is this the right control plane shape?"

The most important architectural questions now are:

1. Do we like `session.json` plus `state.json` plus `events.jsonl` as the core contract?
2. Do we want approvals and integrations to be absorbed into the same native tool model, or kept adjacent longer?
3. Do we want to preserve the file-backed coordination approach, or replace parts of it with a more evented local runtime layer?
4. How much of Claude's hook behavior do we want native to match exactly, especially around sync timing and permissions?

## Short version

`toc-native` is becoming the place where toc stops being a wrapper around somebody else's runtime and starts acting like its own agent system.

The code already reflects that shift:

- config is resolved into a toc-owned session contract
- prompt assembly is explicit
- tool execution is explicit
- state and events are explicit
- sub-agent coordination is explicit
- context management is explicit

That is the real architecture story. The current rough edges are mostly about breadth and polish, not about whether the runtime has a coherent center.
