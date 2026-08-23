#!/usr/bin/env bash

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: just aos-composition-smoke <issue-or-freeform>" >&2
  exit 2
fi

exec ./aos-cli/aos \
  --agent codex \
  --role platform \
  --warded \
  --composed \
  --guarded \
  --image agentic-os:aos-local \
  -- "$@" --print
