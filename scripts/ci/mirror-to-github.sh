#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)

case "${1:-}" in
  test)
    bash "$repo_root/scripts/ci-command.sh" bash "$repo_root/scripts/ci/repo-test-gate.sh"
    ;;
  mirror)
    # Refuse rather than skip. A green "Mirror to GitHub" job that mirrored
    # nothing hides the outage it is the only witness to (agentic-os#7797).
    if [ -z "${PAT:-}" ]; then
      echo "::error::mirror-to-github: GITHUB_MIRROR_PAT is empty, so nothing was mirrored." >&2
      echo "The secret is unset or unreadable from this job. GitHub is the module" >&2
      echo "origin, so an unmirrored tag is a release no Go consumer can resolve." >&2
      exit 1
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
