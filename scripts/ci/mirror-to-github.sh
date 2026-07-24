#!/usr/bin/env bash

set -euo pipefail

case "${1:-}" in
  test)
    ward exec test
    ward exec pre-commit-all
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
      echo "GitHub main is protected, so a human must reconcile it. See docs/mirror-to-github.md." >&2
      exit 1
    fi
    git push --tags github
    ;;
  *)
    echo "usage: $0 test|mirror" >&2
    exit 2
    ;;
esac
