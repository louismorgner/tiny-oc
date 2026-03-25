---
name: agentic-memory
description: Persistent memory system with daily logs and long-term storage for agents that maintain state across sessions
license: MIT
compatibility: claude-code
---

# agentic-memory

You have a file-based memory system. Use it to maintain continuity across sessions. The model forgets — the files don't.

## Memory structure

Two tiers of memory, both stored in the `memory/` directory:

### Daily logs (`memory/YYYY-MM-DD.md`)

Append-only files for session-level notes. Create one for today if it doesn't exist. Write to it throughout the session.

What goes here:
- What you worked on and key decisions made
- Problems encountered and how they were resolved
- Things the user mentioned that might matter later
- Open questions or unfinished work

Format:
```markdown
# 2026-03-24

## Session notes
- Refactored auth middleware to use JWT validation
- User prefers explicit error types over generic error returns
- TODO: migrate remaining endpoints to new auth pattern (3 left)
```

### Long-term memory (`memory/MEMORY.md`)

Curated file for durable information. Not append-only — actively maintain it. Remove things that are no longer true. Update things that have changed.

What goes here:
- User preferences and working style observations
- Architectural decisions and their rationale
- Project conventions that aren't obvious from code
- Recurring patterns or preferences across sessions
- Open loops: things you need to follow up on

What does NOT go here:
- Things derivable from code or git history
- Session-specific details (those go in daily logs)
- Temporary debugging notes

## When to write

- **During the session**: When you learn something worth remembering, write it immediately. Don't wait until the end.
- **Before context gets long**: If the conversation is getting deep, flush important context to memory. You might lose it to compaction.
- **At session end**: Do a final pass — is there anything from this session that future-you needs to know?
- **When the user asks you to remember**: Write it down right away. Acknowledge that you've saved it.

## When to read

- **Session start**: Read today's log, yesterday's log, and MEMORY.md as part of your bootstrap sequence.
- **When the user references past work**: Check memory before saying you don't remember.
- **When making decisions**: Check if there's a prior decision or preference recorded.

## Principles

- Write for your future self. Be specific enough that the note is useful without the original context.
- Keep MEMORY.md under 200 lines. If it's growing beyond that, consolidate or archive old entries.
- Daily logs can be verbose. Long-term memory should be curated.
- If the user corrects you, update memory immediately. Don't repeat the same mistake.
