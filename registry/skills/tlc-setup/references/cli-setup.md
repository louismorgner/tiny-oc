# TLC CLI Setup

Use this reference when you need concrete commands, file layout, or a starter wrapper while applying the `tlc-setup` skill.

## Install

- TLC is distributed in `tla2tools.jar`.
- The TLA+ tools require Java 11 or newer.
- The most automation-friendly install path is:

```bash
mkdir -p tools/tla
curl -L https://github.com/tlaplus/tlaplus/releases/latest/download/tla2tools.jar \
  -o tools/tla/tla2tools.jar
```

- If the repository already depends on a developer-managed TLA+ installation, prefer an environment variable like `TLA2TOOLS_JAR` instead of adding a second copy.
- VS Code and the Toolbox can still be used, but keep the CLI invocation as the canonical project entrypoint.

## Suggested layout

```text
specs/
  Counter.tla
  Counter.cfg
scripts/
  run-tlc
tools/
  tla/
    tla2tools.jar
.tlc/
```

Use the existing repository layout if it is already coherent.

## Minimal module

```tla
---- MODULE Counter ----
EXTENDS Naturals

CONSTANT Limit
VARIABLE value

Init == value = 0

Next == value' \in 0..Limit

TypeInvariant == value \in 0..Limit

Spec == Init /\ [][Next]_<<value>>

====
```

This is intentionally small. Replace it with domain-specific variables and actions once the workflow is running.

## Minimal config

```text
SPECIFICATION Spec
INVARIANT TypeInvariant
CONSTANT
  Limit = 3
```

Notes:

- If you omit `-config`, TLC looks for a sibling `.cfg` file with the same basename as the `.tla` file.
- If the module does not define `Spec`, use:

```text
INIT Init
NEXT Next
INVARIANT TypeInvariant
```

## Wrapper script pattern

```bash
#!/usr/bin/env bash
set -euo pipefail

JAR="${TLA2TOOLS_JAR:-tools/tla/tla2tools.jar}"
SPEC="${1:-specs/Counter.tla}"
CFG="${2:-${SPEC%.tla}.cfg}"
NAME="$(basename "${SPEC%.tla}")"
META_DIR="${3:-.tlc/${NAME}}"

mkdir -p "$META_DIR"

java -jar "$JAR" \
  -config "$CFG" \
  -metadir "$META_DIR" \
  -workers auto \
  "$SPEC"
```

This keeps the default path simple while still allowing local overrides.

## PlusCal note

If the spec is written in PlusCal and you want to keep a hand-edited config file, run:

```bash
java -cp tools/tla/tla2tools.jar pcal.trans -nocfg specs/Counter.tla
```

Without `-nocfg`, the translator writes or overwrites `Counter.cfg`.

## CI pattern

In CI:

1. Install Java 11+.
2. Fetch `tla2tools.jar` if the repo does not already provide it.
3. Run the same wrapper script used locally.

Keep CI bounded and deterministic. Prefer one or two small models over a large exploratory state space.
