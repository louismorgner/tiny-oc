# Integration permissions

This document captures the design vision for how toc handles integration permissions. It serves as the reference for implementation decisions and team alignment.

## Design principles

**Broad token, narrow grants.** Each integration has one powerful OAuth token (or API key) that can do everything. toc enforces fine-grained, human-readable capability grants per agent. The user never thinks about API scopes — they think about what the agent is allowed to do.

**Three modes: on, ask, off.** Every capability grant has a mode:
- `on` — the agent can use this capability without interruption
- `ask` — the agent can attempt it, but the user must approve each invocation before it executes
- `off` — the agent cannot use this capability (this is the default for anything not listed)

**Human-readable by default.** Permission strings should read like English. A non-engineer should understand what `slack: post:#eng` means without looking at docs.

**Default deny.** If a capability is not explicitly granted, the agent cannot use it. There is no implicit access.

## Why not MCP

MCP defines transport-level OAuth (2.1) but has no tool-level or resource-level permission model. A single OAuth token grants whatever scope the provider allows — there is no standard way to say "this agent can send messages but not read history." The spec also forces every MCP server to act as its own authorization server, which conflicts with how enterprises manage identity. 43% of deployed MCP servers have command injection vulnerabilities, and the protocol carries unresolved security issues around tool poisoning and prompt injection via data.

toc's approach is to own the permission layer entirely. MCP compatibility can be added later as an adapter without changing the core model.

## Why not Merge.dev

Merge provides unified API abstraction across 200+ integrations and a newer "Agent Handler" product with MCP servers for 1000+ tools.

Reasons to skip it for launch:

- **Token ownership.** Merge owns your customers' OAuth tokens. Leaving Merge means every customer must re-authenticate. This is hard lock-in.
- **Stale data.** Merge Unified uses a sync-and-store model. Data is cached, not live. Daily sync on lower plans.
- **Lowest common denominator.** Unified data models strip integration-specific features. Complex use cases require passthrough requests anyway.
- **Cost scaling.** ~$65 per linked account. 500 customers with one integration = $32.5k/month.
- **Coverage isn't the bottleneck.** For launch, 5-10 hand-crafted integrations cover the use cases that matter. Each takes 2-3 days of work, not months.

Merge may be worth revisiting if enterprise customers demand 50+ integrations and the team cannot keep up. Until then, hand-crafted integrations are higher quality and give full control.

## Architecture

### Two layers

```
Layer 1: Integration credential (broad, per-workspace)
  The OAuth token or API key connected to the workspace.
  Requests all scopes the integration might ever need.
  Set up once via `toc integrate add <name>`.

Layer 2: Agent capability grants (narrow, per-agent)
  Defined in .oc-agent.yaml under permissions.integrations.
  Human-readable strings that toc enforces at invocation time.
  Default deny — unlisted capabilities are blocked.
```

### Enforcement flow

1. Agent calls `toc runtime invoke <integration> <action> --params...`
2. toc loads the permission manifest for this session
3. toc parses the capability grant and checks: does any grant match this action + target?
4. If the matching grant has mode `on`: execute immediately
5. If the matching grant has mode `ask`: pause execution, show the user what the agent wants to do, wait for approval
6. If no grant matches or mode is `off`: deny with error message
7. Rate limits are checked (per-session, per-action)
8. Credentials are loaded and decrypted
9. The HTTP gateway builds and executes the API request
10. Response is filtered through the field whitelist and returned to the agent

### Permission manifest

Written at session spawn time to `.toc/sessions/<id>/permissions.json`:

```json
{
  "session_id": "abc-123",
  "agent": "slack-bot",
  "integrations": {
    "slack": [
      { "capability": "post:#eng,#ops", "mode": "on" },
      { "capability": "read:*", "mode": "ask" },
      { "capability": "search:*", "mode": "off" }
    ]
  }
}
```

## YAML syntax

### Simple form (mode defaults to `on`)

```yaml
permissions:
  integrations:
    slack:
      - post:#eng
```

This means: the agent can post to #eng without user approval.

### Explicit mode

```yaml
permissions:
  integrations:
    slack:
      - on: post:#eng,#ops
      - ask: post:*
      - ask: read:*
```

This means:
- Post to #eng and #ops freely
- Post to any other channel, but ask the user first
- Read any channel, but ask the user first
- Everything else (search, manage, etc.) is denied

### Full example across integrations

```yaml
permissions:
  filesystem:
    read: on
    write: on
    execute: ask
  integrations:
    slack:
      - on: post:#eng,#ops
      - ask: post:*
      - ask: read:*
    github:
      - on: issues.read:*
      - on: pulls.read:*
      - ask: issues.write:louismorgner/tiny-oc
      - ask: issues.comment:*
  sub-agents:
    "*": on
```

### Shorthand for common patterns

```yaml
# Allow everything, no approval needed
permissions:
  integrations:
    slack: "on:*"

# Allow everything, but ask for each action
permissions:
  integrations:
    slack: "ask:*"

# Single capability, defaults to on
permissions:
  integrations:
    slack: "post:#eng"
```

## Capability mapping

Each integration defines a set of **capabilities** in its registry YAML. A capability is a human-readable grouping of one or more API actions.

### Current model (action-level)

The current implementation maps permissions directly to API actions:

```yaml
# In .oc-agent.yaml
slack:
  - send_message:*
  - read_messages:#eng
  - react:*
```

This works but is engineering-focused. Users must know action names.

### Target model (intent-level)

Introduce a `capabilities` block in the integration registry YAML that maps human-readable names to underlying actions:

```yaml
# In registry/integrations/slack/integration.yaml
capabilities:
  post:
    description: Send messages and reactions
    actions:
      - send_message
      - react
  read:
    description: Read message history
    actions:
      - read_messages
  search:
    description: Search messages across channels
    actions:
      - search_messages
  discover:
    description: List channels and workspace info
    actions:
      - list_channels
```

When a user writes `post:#eng`, toc expands it to `send_message:#eng` + `react:#eng` at enforcement time.

### Proposed Slack capability syntax

Slack should expose a small, opinionated permission surface that matches how humans think about Slack while still respecting Slack's actual resource model.

#### Capability names

- `post:<target>` — send a message to a conversation
- `read:<target>` — read message history from a conversation
- `react:<target>` — add reactions in a conversation
- `discover:<target>` — list conversations the installed Slack identity can see
- `search:*` — search messages across everything the installed Slack identity can search

#### Target syntax

For Slack, `<target>` should be one of:

- `#eng` — a single channel by name
- `id/C123ABC456` — a single conversation by immutable Slack ID
- `public/*` — any public channel
- `private/*` — any private channel visible to the installed identity
- `channels/*` — any public or private channel visible to the installed identity
- `dm/*` — any existing 1:1 DM visible to the installed identity
- `mpim/*` — any existing group DM visible to the installed identity
- `conversations/*` — any visible Slack conversation

#### Deliberate constraints

- `search` should stay workspace-wide: `search:*`
  Slack's `search.messages` API is not naturally channel-scoped. Requiring `in:#channel` inside the query would be brittle and easy to bypass.
- `#name` should only mean channels, not DMs
  Channel names are reasonably human-readable; DM identities are not stable enough to treat the same way.
- Starting a new DM should be a separate future capability
  Posting into an existing DM and opening a DM with a user are different operations in Slack and should not be collapsed into one permission.
- IDs should always remain available as the escape hatch
  `id/C123...`, `id/D123...`, and `id/G123...` are less friendly than names, but they are the only immutable selectors Slack guarantees.

#### Recommended examples

```yaml
permissions:
  integrations:
    slack:
      - post:#eng
      - read:#eng
```

```yaml
permissions:
  integrations:
    slack:
      - post:channels/*
      - react:channels/*
      - read:#incidents,#eng-leads
      - search:*
```

```yaml
permissions:
  integrations:
    slack:
      - read:id/C06ABC12345
      - post:id/C06ABC12345
      - react:id/C06ABC12345
```

#### Future-only syntax we should not promise yet

- `post:@alice`
- `read:@alice`
- `search:#eng`
- glob patterns like `post:#eng-*`

Those may become valid later, but only after we have an explicit Slack identity-resolution model and a safe story for DM creation/opening.

### Migration path

1. Continue supporting raw action names (`send_message:*`) for power users
2. Add the `capabilities` block to registry YAMLs
3. Prefer capabilities in docs and `toc integrate add` output
4. Both forms are valid in `.oc-agent.yaml`

## The "ask" mode in detail

### What the user sees

When an agent triggers an action that matches an `ask` grant, toc pauses the agent and shows:

```
[slack] Agent "deploy-bot" wants to post a message:

  Channel: #production
  Text: "Deployment v2.3.1 complete. All health checks passing."

  Allow?  [y]es  [n]o  [a]lways for this channel
```

### Behavior rules

- **Blocking.** The agent's execution pauses until the user responds. The runtime invoke command blocks on stdin/approval.
- **Session-scoped memory.** If the user chooses "always for this channel," toc upgrades that specific scope to `on` for the remainder of the session. This does not persist across sessions.
- **Denial is not fatal.** If the user denies, toc returns a clear error to the agent (`permission denied: user declined`). The agent can continue with other work.
- **Batching.** If an agent triggers multiple `ask` actions in quick succession, toc batches them into a single approval prompt where possible.
- **Timeout.** If the user does not respond within a configurable timeout (default: 5 minutes), the action is denied.

### Implementation approach

The `ask` mode requires a communication channel between the runtime invoke command and the user's terminal or UI. For the CLI:

1. `toc runtime invoke` detects `ask` mode
2. Writes a pending approval request to `.toc/sessions/<id>/pending_approvals/<uuid>.json`
3. The session's parent process (toc session or Conductor UI) watches for pending approvals
4. Shows the prompt to the user
5. Writes the response to `.toc/sessions/<id>/pending_approvals/<uuid>.response.json`
6. `toc runtime invoke` reads the response and proceeds or denies

For the Conductor desktop app, this becomes a native approval dialog.

## What to build for launch

### Phase 1: Foundation (must-have)

- [ ] Add `mode` field to integration permission entries in agent config
- [ ] Update `ParsePermission` to handle `on:`, `ask:`, `off:` prefixes
- [ ] Update `CheckPermission` to return the mode, not just bool
- [ ] Implement `ask` approval flow via pending approval files
- [ ] Update permission manifest to include modes
- [ ] Wire `ask` into `toc runtime invoke`

### Phase 2: Capabilities (should-have)

- [ ] Add `capabilities` block to registry YAML schema
- [ ] Implement capability-to-action expansion in permission checking
- [ ] Add capabilities to Slack and GitHub registry YAMLs
- [ ] Update `toc integrate add` to show available capabilities

### Phase 3: Polish (nice-to-have)

- [ ] Session-scoped "always allow" upgrade from ask prompts
- [ ] Batched approval prompts
- [ ] Approval timeout configuration
- [ ] Add 2-3 more integrations (Linear, Google, Notion)

## Decision log

| Decision | Chosen | Rejected | Why |
|----------|--------|----------|-----|
| Permission enforcement layer | toc application layer | OAuth scopes / MCP | OAuth scopes are engineering-focused and not granular enough. MCP has no permission model. Enforcing at our layer gives human-readable grants and full control. |
| Integration connectivity | Own OAuth flows | Merge.dev | Merge owns customer tokens (lock-in), sync model causes stale data, and we only need 5-10 integrations for launch. |
| Protocol | Direct HTTP via gateway | MCP servers | MCP adds complexity without solving our permission problem. Direct HTTP is simpler, more auditable, and lets us control the full request/response lifecycle. |
| Permission syntax | Intent-based capabilities | Raw API action names | `post:#eng` is more intuitive than `send_message:#eng`. Raw actions still supported for power users. |
| Default mode | Deny (off) | Allow (on) | Security-first. Agents can only do what is explicitly granted. |
| Ask mode scope | Session-scoped upgrades | Persistent upgrades | "Always allow" from an ask prompt should not persist across sessions. The YAML is the source of truth for persistent policy. |
