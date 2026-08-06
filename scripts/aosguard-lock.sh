#!/bin/sh
# Refresh aosguard's committed locks and native generated agent skill.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$repo_root"

specgen_bin=${SPECGEN_BIN:-specgen}
"$specgen_bin" \
    --project-root .specgen/guardfiles \
    --skills-out dist/skills \
    lock \
    "$@"
