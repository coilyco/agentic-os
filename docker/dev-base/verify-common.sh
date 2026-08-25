#!/usr/bin/env bash

set -euo pipefail

aosguard --version
test -s /opt/agentic-os/aosguard-skill/aosguard/SKILL.md
test -s /opt/agentic-os/aosguard-skill/aosguard/references/commands.yaml
test -s /opt/agentic-os/aosguard-skill/aosguard-forgejo/SKILL.md
test -s /opt/agentic-os/aosguard-skill/aosguard-forgejo/references/commands.yaml

roster_dir="$(mktemp -d)"
trap 'rm -rf "$roster_dir"' EXIT
agent-compose version
agent-compose roster --out "$roster_dir"
# A bare `jq -e` exits 1 naming neither the field nor what it saw, so a roster
# that grows fails the image build with no way to tell which count moved.
expect_roster() {
  local filter=$1 want=$2 got
  got="$(jq -r "$filter" "$roster_dir/person.json")"
  if [ "$got" != "$want" ]; then
    echo "roster $filter = $got, expected $want" >&2
    return 1
  fi
}
expect_roster '.source' 'roster:core'
expect_roster '.role_order | length' 7
expect_roster '.personalities | length' 10
test -s "$roster_dir/AGENTS.COMPOSE.md"
test -n "$(
  find "$roster_dir/.agents/skills" \
    -type f -path '*/personality-*/SKILL.md' -print -quit
)"

python3 -m agentic_os.forgejo_actions_list --help >/dev/null

# aos#771: a pinned pre-commit hook must import its own agentic_os, not the
# image copy. Proven by behavior, not by asserting PYTHONPATH is unset.
isolated_dir="$(mktemp -d)"
trap 'rm -rf "$roster_dir" "$isolated_dir"' EXIT
python3 -m venv --without-pip "$isolated_dir/venv"
# Glob the venv's own layout. sysconfig's default scheme is distribution
# patched, and a wrong answer here would write the sentinel into the image.
for candidate in "$isolated_dir"/venv/lib/python*/site-packages; do
  sentinel_site="$candidate"
done
test -d "$sentinel_site"
mkdir -p "$sentinel_site/agentic_os"
printf 'SENTINEL = "isolated"\n' >"$sentinel_site/agentic_os/__init__.py"
test "$(
  "$isolated_dir/venv/bin/python" -c 'import agentic_os; print(agentic_os.SENTINEL)'
)" = isolated

ward --version
CLIGUARD_NO_SANDBOX=1 WARD_DOCTOR_ALLOW_PLACEHOLDERS=1 ward doctor
