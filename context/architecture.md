# Architecture

Go CLI built with Cobra. Two top-level directories:

- `cmd/` — CLI command definitions (one file per command)
- `internal/` — business logic packages, each single-responsibility

## Key packages

| Package | Purpose |
|---------|---------|
| `agent` | Agent config (oc-agent.yaml) CRUD |
| `session` | Session tracking (sessions.yaml) |
| `spawn` | Session lifecycle — create temp dir, copy template, resolve skills, launch Claude |
| `skill` | Local + URL skill management, SKILL.md parsing |
| `registry` | Remote registry — fetch index, sparse clone, install |
| `sync` | Context file syncing — pattern matching, PostToolUse hooks, post-session sync |
| `audit` | Append-only JSONL audit log |
| `config` | Workspace config (.toc/config.yaml) and path constants |
| `fileutil` | Shared file copy utilities (CopyDir, CopyFile) with permission preservation |
| `gitutil` | Safe git clone with hook prevention and HTTPS enforcement |
| `ui` | Terminal output helpers (colors, prompts, formatting) |

## Conventions

- All git clones MUST go through `gitutil.SafeClone` (disables hooks, enforces HTTPS)
- All file copies MUST use `fileutil.CopyDir` / `fileutil.CopyFile` (preserves permissions)
- Audit logging goes through `auditLog()` in cmd/root.go (surfaces errors via ui.Warn)
- Sensitive files (audit.log, sessions.yaml) use 0600 permissions
- Session directories use `os.MkdirTemp` for unpredictable paths
