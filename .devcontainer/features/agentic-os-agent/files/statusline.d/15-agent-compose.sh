#!/usr/bin/env bash
# Status-line provider: projected Agent Compose identity and composition health.
set -euo pipefail

command -v acompose >/dev/null 2>&1 || exit 0
exec acompose statusline \
  --target "${AOS_STATUSLINE_PROJECT_DIR:-$PWD}" \
  --color
