#!/usr/bin/env bash

set -euo pipefail

python3 scripts/dep-bump.py plan > /tmp/dep-bump-plan.tsv
if [ ! -s /tmp/dep-bump-plan.tsv ]; then
  echo "dev-base pins are current with upstream; nothing to bump."
  exit 0
fi
echo "Bumping:"
cat /tmp/dep-bump-plan.tsv

git config user.name "coilyco-ops"
git config user.email "coilyco-ops@coilysiren.me"

while IFS="$(printf '\t')" read -r name current latest; do
  [ -z "$name" ] && continue
  python3 scripts/dep-bump.py apply --arg "$name" --version "$latest"
  git add docker/dev-base/*/Dockerfile
  git commit -m "chore(dev-base): auto-bump ${name} ${current} -> ${latest} (agentic-os#272)"
done < /tmp/dep-bump-plan.tsv

ward exec test
ward exec pre-commit-all

git push origin HEAD:main
