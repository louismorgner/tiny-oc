---
name: tlc-setup
description: Set up and configure the TLA+ TLC model checker in a repo, including CLI workflows, config files, and CI-friendly automation
license: MIT
compatibility: claude-code
---

# tlc-setup

Use this skill when the user wants help setting up TLC, wiring TLA+ into a repository, creating runnable `.tla` and `.cfg` files, or turning a natural-language system description into a reproducible model-checking workflow.

Prefer a repo-local, scriptable setup over editor-only instructions. The goal is to leave behind files and commands that humans, CI, and coding agents can all run the same way.

## Workflow

1. Determine the scope of the request.
   Typical scopes: first-time install, initial spec scaffolding, config cleanup, CI wiring, or troubleshooting an existing TLC run.
2. Inspect the repository before changing anything.
   Look for existing `*.tla`, `*.cfg`, Java setup, wrapper scripts, `Makefile` targets, and CI workflows. Reuse the existing layout if one already exists.
3. Default to CLI-first TLC setup.
   TLC is easiest to automate through `tla2tools.jar`. Prefer a checked-in script or `make` target so the same entrypoint works locally and in CI.
4. Create the smallest runnable model that matches the user's request.
   Start with a finite model, explicit constants, a named behavior spec, and at least one invariant. Avoid premature liveness checks or large state spaces.
5. Keep configuration explicit.
   Prefer `SPECIFICATION Spec` in the `.cfg` file when the module already defines `Spec`. Use `INIT` and `NEXT` only when the spec has not been wrapped into a single behavior operator yet.
6. Keep operational output out of the source tree where practical.
   Direct TLC metadata and generated state-space files into a generated directory such as `.tlc/`.
7. Verify the baseline before tuning.
   Get one successful TLC run first. Only then add worker, memory, coverage, dump, or trace-export options if the user actually needs them.
8. Explain the modeling assumptions.
   Call out bounded constants, omitted real-world detail, fairness assumptions, and anything that keeps the model finite.

## Defaults

- Prefer a directory like `specs/` unless the repository already has a clear convention.
- Prefer one small wrapper script over scattered ad hoc shell commands.
- Keep the TLC invocation deterministic and visible in versioned files.
- Do not commit downloaded jars unless the repository already vendors binaries.
- If the user mentions PlusCal, preserve hand-edited `.cfg` files by using `pcal.trans -nocfg` when appropriate.

## Natural-language mapping

- "Set up TLC for this service" means: install or reference `tla2tools.jar`, scaffold a spec directory, create a runnable `.cfg`, and add a script or `make` target.
- "Model-check this workflow" means: derive a minimal state model from the workflow, encode bounds and invariants explicitly, and run TLC on a finite instance.
- "Add TLC to CI" means: make the CLI path the source of truth, install Java in CI, fetch or reference the jar, and run the same wrapper used locally.
- "Fix this TLC config" means: inspect operator names, constants, module filenames, spec/config alignment, and state-space bounds before changing flags.

## Guardrails

- Keep module filenames and `---- MODULE Name ----` aligned.
- Keep `.cfg` references pointed at named operators that actually exist in the module.
- Prefer type invariants and small bounded constants first; they catch real mistakes quickly.
- If the model explodes, reduce constants or introduce stronger state constraints before reaching for advanced flags.
- If the request is really about TLAPS or Apalache, say so explicitly; those are different tools with different workflows.

For command templates, config skeletons, and a reusable wrapper pattern, read [references/cli-setup.md](references/cli-setup.md).
