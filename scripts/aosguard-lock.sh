#!/bin/sh
# Refresh aosguard's committed locks and native generated agent skill.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$repo_root"

umbra_bin=${UMBRA_BIN:-umbra}
"$umbra_bin" \
    --project-root .umbra/guardfiles \
    --skills-out dist/skills \
    lock \
    "$@"
