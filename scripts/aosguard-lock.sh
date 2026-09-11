#!/bin/sh
# Refresh aosguard's committed locks and native generated agent skill.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$repo_root"

umbra_bin=${UMBRA_BIN:-umbra}
# One lock, two wrap binaries, so a member has to be named. Reference doc:
# the tooling-aosguard skill, references/gh-replacement.md.
"$umbra_bin" \
    --project-root .umbra/guardfiles \
    --guardfile "$repo_root/.umbra/guardfiles/aosguard/forgejo.kdl" \
    --skills-out dist/skills \
    lock \
    "$@"
