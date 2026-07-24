#!/usr/bin/env bash
set -euo pipefail

exec python3 -m agentic_os.forgejo_actions_rerun rerun-failed-jobs "$@"
