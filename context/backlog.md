# Backlog

## High priority

- **Test coverage** — Only `internal/sync` has tests. Need tests for agent, session, skill, spawn, audit, registry, fileutil, gitutil packages.
- **Module path mismatch** — `go.mod` says `github.com/tiny-oc/toc` but repo is `github.com/louismorgner/tiny-oc`. `go install` won't work unless the tiny-oc org/repo exists. Decision needed: transfer repo or update module path.

## Medium priority

- **File locking** — `session.Add()`, `skill.AddRef()`, `skill.RemoveRef()` do read-then-write without locking. Concurrent spawns can lose data. Consider `flock` or atomic write-rename.
- **isDir heuristic** — `sync.isDir()` guesses by checking no extension + no glob chars. Breaks for `Makefile`, `Dockerfile`, or dirs named `data.v2`. Needs a better approach or documentation.

## Low priority

- **Changelog links** point to `github.com/tiny-oc/toc` (same mismatch as module path)
