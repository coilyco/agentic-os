#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)

case "${1:-}" in
  test)
    bash "$repo_root/scripts/ci-command.sh" bash "$repo_root/scripts/ci/repo-test-gate.sh"
    ;;
  mirror)
    if [ -z "${PAT}" ]; then
      echo "mirror-to-github: GITHUB_MIRROR_PAT secret not set; skipping." >&2
      exit 0
    fi
    git remote add github "https://x-access-token:${PAT}@github.com/coilysiren/agentic-os.git"
    if ! git push github main; then
      echo "::error::mirror-to-github: fast-forward push to GitHub main rejected." >&2
      echo "GitHub main has diverged from Forgejo main or the PAT lost push access." >&2
      echo "GitHub main is protected, so a human must reconcile it. See the tooling-aosguard skill." >&2
      exit 1
    fi
    git push --tags github
    ;;
  *)
    echo "usage: $0 test|mirror" >&2
    exit 2
    ;;
esac
