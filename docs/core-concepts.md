# Core concepts

This document defines the primary vocabulary for toc. Use these terms consistently in product decisions, documentation, runtime design, and code where practical.

## Workspace

A `workspace` is the project-local control plane rooted at `.toc/`.

It contains:

- agent definitions
- session history
- session metadata
- skills
- audit data

## Agent

An `agent` is a reusable worker definition stored under `.toc/agents/<name>/`.

An agent consists of:

- `oc-agent.yaml` for config
- `agent.md` as the instruction source of truth
- optional additional files such as compose layers, memory files, or templates

## Session

A `session` is one execution of an agent.

A session has:

- a unique session ID
- a runtime
- a model
- an isolated working directory
- toc-owned metadata under `.toc/sessions/<id>/`

Use `session` as the primary lifecycle term rather than `run`.

## Runtime

A `runtime` is the execution engine that runs a session.

Examples:

- `claude-code` as the current implementation
- a future toc-owned runtime

The runtime is responsible for:

- preparing the session workspace
- materializing instructions for execution
- running the tool loop
- producing session events
- supporting resume and sub-agents

## Model

A `model` is the model identifier used by a runtime for a session.

Model validation is runtime-specific.

## agent.md

`agent.md` is the canonical instruction source for an agent.

It is intentionally fixed as the root instruction file. toc does not expose a configurable root instruction file path.

Runtime providers may transform `agent.md` into a runtime-specific format, but `agent.md` remains the user-facing source of truth.

## Instruction compose

`instruction compose` is the concept for layering additional instruction files after `agent.md`.

The current config field name is:

- `compose`

Those files are appended in order to form the final instruction payload for the runtime.

## Parent snapshot

The `parent snapshot` is the agent template directory in `.toc/agents/<name>/` at the moment a session is spawned.

When a session starts, toc copies that snapshot into an isolated session workspace.

## Snapshot sync

`snapshot sync` is the mechanism for syncing selected files from a session workspace back into the parent snapshot.

The current config field name is:

- `context`

This term is preferred conceptually over calling the feature “context sync,” because the synced files are not necessarily prompt context; they are persisted outputs from a session.

## Permissions

`permissions` is the top-level term for what an agent is allowed to do.

Keep `permissions` as the public term rather than `policy`.

Current permission areas include:

- filesystem
- integrations
- sub-agents

## Skill

A `skill` is a reusable capability bundle provisioned into a session by toc.

A skill can contribute:

- instructions
- tool guidance
- reusable workflows

## Tool

A `tool` is an action the runtime can execute during a session.

Examples:

- file reads and writes
- shell commands
- searches
- skill invocations

For the current `toc-native` beta, `tool` should be read narrowly as local runtime tools. External integrations are not yet part of the native tool loop.

## Event

An `event` is the append-only record of something that happened during a session.

toc owns the normalized event log at:

- `.toc/sessions/<id>/events.jsonl`

This is the preferred observability format across runtimes.

## Metadata directory

Each session has a toc-owned metadata directory:

- `.toc/sessions/<id>/`

This is where toc stores runtime-independent session artifacts such as:

- `events.jsonl`
- `permissions.json`
- future persisted runtime state

## Naming guidance

Prefer these terms:

- `session`
- `permissions`
- `parent snapshot`
- `snapshot sync`
- `instruction compose`
- `agent.md`

Avoid introducing competing top-level terms unless there is a strong reason:

- `run`
- `policy`
- `artifact` as the main persisted-session term
- configurable root instruction file names
