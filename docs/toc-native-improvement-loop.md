# Pushing toc-native To The Edge

This guide is about using `--inspect` and `toc inspect compare` to make `toc-native` better quickly.

The goal is simple:

1. Run the same task through a strong baseline runtime and `toc-native`
2. Capture the concrete upstream model calls
3. Compare where the loops diverge
4. Tighten `toc-native`
5. Repeat until the gap is gone

This is the fastest way to understand whether a problem is:

- prompt construction
- tool loop behavior
- context management
- retry behavior
- model selection
- tool availability
- runtime bugs

## Why this matters

User-facing output is not enough.

Two sessions can end with similar text while taking very different paths:

- one might use fewer model calls
- one might bloat context
- one might miss tools and compensate with bad reasoning
- one might retry unnecessarily
- one might send much worse prompts to the model

`--inspect` lets you compare the actual API traffic, not just the final answer.

## What data you get

### With `--inspect`

You get full HTTP capture in:

```text
.toc/sessions/<session-id>/inspect/http.jsonl
```

And first-class tooling on top:

```bash
toc inspect <session-id>
toc inspect <session-id> --call 2 --body
toc inspect compare <session-a> <session-b>
toc inspect compare <session-a> <session-b> --json
```

This is the highest-fidelity comparison path across runtimes.

### Without `--inspect`

You do not get the same richness.

What you still have:

- `events.jsonl` for normalized session behavior
- `toc runtime replay` for session steps
- `toc debug` for state, errors, and artifacts
- `trace.jsonl` for `toc-native` only, if trace was enabled

What you do not have:

- runtime-agnostic raw upstream request/response capture
- reliable Claude-vs-native call-by-call comparison
- exact prompt/body comparison across runtimes

If you want serious runtime comparison, run both sessions with `--inspect`.

## The core workflow

### 1. Choose a task that exposes a real edge

Good tasks:

- multi-step file edits
- tool-heavy bug fixes
- large-context repository exploration
- sessions that require shell plus file operations
- prompts that previously caused looping or weak decisions

Avoid trivial one-shot prompts. They do not stress the runtime enough.

### 2. Keep the task controlled

Use the same:

- repo state
- agent
- prompt
- workspace

Keep variables fixed unless you are intentionally testing one change.

### 3. Run the baseline runtime with inspect

For example:

```bash
toc agent spawn reviewer --prompt "fix the failing parser tests" --inspect
```

If the baseline agent uses `claude-code`, that session becomes your reference.

### 4. Run toc-native with inspect

Use the same task and as close to the same agent contract as possible:

```bash
toc agent spawn reviewer-native --prompt "fix the failing parser tests" --inspect
```

### 5. Inspect each session

Start with high-level summaries:

```bash
toc inspect <claude-session>
toc inspect <native-session>
```

Then compare directly:

```bash
toc inspect compare <claude-session> <native-session>
```

Use `--json` when feeding the result into another agent or script:

```bash
toc inspect compare <claude-session> <native-session> --json
```

### 6. Drill into specific calls

When a call looks suspicious:

```bash
toc inspect <native-session> --call 3 --body
toc inspect <claude-session> --call 3 --body
```

This is usually where the real cause shows up.

## What to look for

### Fewer or more model calls than expected

If `toc-native` makes many more calls than Claude for the same task, look for:

- weak system prompt construction
- poor tool result packaging
- too-small steps between tool executions
- retries caused by malformed requests
- missing stopping conditions

### Much larger token usage

If `toc-native` sends much larger requests:

- context pruning may be too weak
- tool outputs may be carried forward too aggressively
- prompt composition may be duplicating content
- the runtime may be preserving more transcript than necessary

### Path differences

If one runtime hits different API paths or different models:

- confirm the intended provider/model selection
- confirm environment overrides
- confirm the session was routed through the right base URL

### Different prompt shape

If the request bodies differ in the actual user/system/tool context:

- inspect instruction composition
- inspect how tool results are turned back into messages
- inspect whether continuation or compaction changed the prompt unexpectedly

### Different finish reasons

If one session finishes with `tool_calls` repeatedly while the other stops:

- inspect tool loop exit behavior
- inspect whether tool results are understandable to the model
- inspect whether the model is missing a clean completion path

### Errors or retries

If `toc-native` shows more failures:

- check request validity
- check provider/base URL configuration
- check whether the runtime is sending parameters in the expected shape
- check whether streaming assembly is producing malformed tool calls

## A practical comparison loop

Use this pattern repeatedly:

1. Pick one failing or weak task
2. Run Claude with `--inspect`
3. Run `toc-native` with `--inspect`
4. Compare summaries with `toc inspect compare`
5. Inspect the first obviously divergent call with `--call N --body`
6. Fix the runtime
7. Re-run the same prompt
8. Stop only when the divergence or inefficiency is explained

Do not change five things at once. One prompt, one fix, one rerun.

## Good stress cases for toc-native

Use tasks that expose structural weaknesses:

- “Read three related files, explain the bug, patch it, and run the targeted test”
- “Search the repo for all implementations of X, compare them, and edit the shared abstraction”
- “Debug a failing shell command, inspect logs, and fix the config”
- “Make a change that requires multiple file reads before the first write”
- “Resume a partially completed task and finish it cleanly”

These reveal far more than shallow generation tasks.

## Turning this into regression coverage

For non-interactive loops, prefer:

```bash
toc agent spawn my-agent --prompt "..." --inspect
```

That gives you:

- deterministic session inputs
- inspect captures on disk
- a path to e2e assertions

A useful regression pattern is:

1. Run a known prompt non-interactively
2. Assert the session completes
3. Assert the number of captured calls is within a sensible range
4. Assert the API path/model is correct
5. Assert obviously bad regressions do not appear:
   - explosive token growth
   - repeated error calls
   - unexpected extra calls

## When to use other tools

Use `toc inspect` for upstream API traffic.

Use `toc runtime replay` when you want the normalized session story:

- thinking
- tool calls
- agent-visible steps

Use `toc debug` when the session is broken and you need:

- state
- stderr
- crash info
- last error

These tools complement each other. They answer different questions.

## Recommended default habit

If you are actively improving `toc-native`, default to this:

```bash
toc agent spawn <agent> --prompt "..." --inspect
```

And when comparing against Claude:

```bash
toc inspect compare <claude-session> <native-session>
```

This should be the normal improvement path, not a special-case debugging trick.
