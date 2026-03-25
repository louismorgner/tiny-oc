# Skills

Skills are reusable capabilities that can be attached to agents. They're loaded into sessions as Claude Code skills, giving the agent additional instructions and tool access.

## How skills work

When an agent with skills is spawned, toc:

1. Resolves each skill (local directory or Git URL)
2. Copies skill files into the session's `.claude/skills/` directory
3. Claude Code picks them up automatically

Skills are defined by a `SKILL.md` file with YAML frontmatter and markdown instructions.

## Creating a local skill

```bash
toc skill create
```

The interactive prompt asks for name, description, optional license, and instructions. This creates `.toc/skills/<name>/SKILL.md`.

### SKILL.md format

```markdown
---
name: code-review
description: Reviews code changes for quality, bugs, and style issues
license: MIT
compatibility: claude-sonnet
metadata:
  version: "1.0"
allowed-tools: read, analyze
---

# Code Review

Review all code changes for:
- Correctness and potential bugs
- Style consistency
- Performance implications

Output a summary with actionable feedback.
```

### Frontmatter fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Lowercase alphanumeric with hyphens |
| `description` | string | yes | What the skill does |
| `license` | string | no | License identifier (e.g. `MIT`) |
| `compatibility` | string | no | Recommended model (e.g. `claude-sonnet`) |
| `metadata` | map | no | Arbitrary key-value pairs (e.g. `version`) |
| `allowed-tools` | string | no | Comma-separated list of allowed tools |

The markdown body after the frontmatter is the skill's instructions — loaded into the agent's context at session time.

## Adding a remote skill

Install a skill by name (from the registry) or by Git URL:

```bash
toc skill add open-source-cto               # auto-detects as registry name
toc skill add https://github.com/example/my-skill.git  # installs from URL
```

When given a name, toc searches the registry automatically. When given a URL, it clones the repo, locates the `SKILL.md` (at root or one level deep), validates it, and registers the skill locally.

URL skill references are tracked in `.toc/skills.yaml`:

```yaml
skills:
  - name: my-skill
    url: https://github.com/example/my-skill.git
```

## Attaching skills to agents

Add skill names or URLs to the agent's `oc-agent.yaml`:

```yaml
skills:
  - code-review          # local skill in .toc/skills/
  - https://github.com/example/my-skill.git  # resolved at spawn time
```

Skills are resolved in order:

1. Check `.toc/skills/<name>/` for a local skill
2. Check `.toc/skills.yaml` for a URL reference
3. If it's a URL, clone and install on the fly

## Listing skills

```bash
toc skill list
```

Shows all available skills (local and URL-referenced) with their descriptions.

## Removing a skill

```bash
toc skill remove <name>
```

Removes the local skill directory and/or its URL reference.

## Built-in registry

The toc registry includes skills and agent templates:

| Name | Type | Description |
|---|---|---|
| `open-source-cto` | skill | Technical decision-making and code quality standards from an open-source CTO perspective |
| `agentic-memory` | skill | Persistent memory system with daily logs and long-term storage for cross-session continuity |
| `cto` | agent | Technical leadership agent — code quality, architecture, and open-source standards |
| `mini-claw` | agent | Persistent agent with identity, memory, and session awareness — inspired by OpenClaw |

Install from the registry:

```bash
toc skill add <skill-name>
toc agent add <agent-name>    # for agent templates (e.g. cto, mini-claw)
toc registry search            # browse all available entries
```

These are available from the [toc registry](https://github.com/louismorgner/tiny-oc/tree/main/registry).
