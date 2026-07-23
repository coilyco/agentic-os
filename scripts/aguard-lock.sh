#!/bin/sh
# Refresh aguard's committed locks and discard specgen's generated references.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$repo_root"

specgen --project-root .specgen lock

# AOS keeps feature documentation under docs/. Specgen reference renders are
# reproducible from the committed KDL and locks, so they stay out of this tree.
for member in .specgen/aguard/*.kdl; do
    rm -f "${member%.kdl}.md"
done
