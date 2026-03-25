# Security Decisions

## Git hook prevention (PR #13)

All `git clone` operations set `core.hooksPath=/dev/null`. This prevents malicious repositories from executing arbitrary code via post-checkout or other git hooks. Applied in:
- `internal/gitutil/gitutil.go` — SafeClone for user-provided URLs
- `internal/registry/registry.go` — cloneRegistryDir for registry installs

## HTTPS-only URLs (PR #13)

`skill.IsURL()` only accepts `https://` prefixes. `gitutil.ValidateURL()` rejects `http://`. This prevents man-in-the-middle attacks that could inject malicious skill content during transit.

## Unpredictable session paths (PR #13)

Sessions use `os.MkdirTemp` instead of predictable `/tmp/toc-sessions/<name>-<timestamp>`. Prevents symlink attacks and TOCTOU races on shared systems.

## File permissions (PR #13)

- `audit.log` → 0600 (contains actor names, hostnames, workspace paths)
- `sessions.yaml` → 0600 (contains workspace paths)
- Session base directory → 0700
