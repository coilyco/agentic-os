#!/usr/bin/env bash
# Status-line composer for dev-base containers. See docs/dev-base-agent-identity.md.
#
# Receives Claude Code's statusLine JSON payload on stdin, runs each discovered
# provider in filename order (handing it the same payload on stdin), and joins
# their non-empty output into the multi-row status line. Replaces the hand-wired
# "agent-name.sh appends statusline.sh": every segment is now a provider.
#
# Provider contract: exit 0 with stdout = that segment; empty stdout or a
# non-zero exit = skipped, so a segment self-suppresses when irrelevant (the
# 15-agent-compose provider renders nothing outside a projected workspace).
#
# Provider discovery walks three dirs, lowest precedence first, so an overlay
# customizes WITHOUT forking this composer:
#   1. base   - <composer-dir>/statusline.d (baked in; $AOS_STATUSLINE_DIR overrides)
#   2. user   - ${XDG_CONFIG_HOME:-$HOME/.config}/agentic-os/statusline.d
#   3. repo   - <project_dir>/.agentic-os/statusline.d
# Same filename in a higher dir overrides the lower one (drop in
# 15-agent-compose.sh to replace it, or a NN-*.sh of your own to add a row). A
# shadowing file that is not executable masks the lower provider, rendering
# nothing.
#
# Format-identical to the host status line is the contract this exists to keep:
# the same providers run on host and in-container, so the composed rows match.
set -euo pipefail

payload="$(cat)"
payload_flat="$(printf '%s' "$payload" | tr -d '\n')"

# Project root for the repo overlay: workspace.project_dir (preferred), else cwd,
# else $CLAUDE_PROJECT_DIR / $PWD. Same extraction as session-name.sh.
project_dir="$(printf '%s' "$payload_flat" \
  | sed -n 's/.*"workspace"[[:space:]]*:[[:space:]]*{[^}]*"project_dir"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
[ -z "$project_dir" ] && project_dir="$(printf '%s' "$payload_flat" \
  | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
[ -z "$project_dir" ] && project_dir="${CLAUDE_PROJECT_DIR:-$PWD}"

# Resolve the composer's own dir so providers can find sibling scripts.
self="${BASH_SOURCE[0]}"
self_dir="$(cd "$(dirname "$self")" && pwd)"
export AOS_STATUSLINE_HOME="$self_dir"
export AOS_STATUSLINE_PROJECT_DIR="$project_dir"

base_dir="${AOS_STATUSLINE_DIR:-$self_dir/statusline.d}"
user_dir="${XDG_CONFIG_HOME:-$HOME/.config}/agentic-os/statusline.d"
repo_dir="$project_dir/.agentic-os/statusline.d"

# Unique provider basenames across all three dirs, in filename (precedence-free)
# order - 10- before 20-. Use 2-digit prefixes; lexical sort puts 100 before 20.
names="$(
  for dir in "$base_dir" "$user_dir" "$repo_dir"; do
    [ -d "$dir" ] || continue
    for f in "$dir"/*.sh; do
      [ -e "$f" ] || continue
      basename "$f"
    done
  done | sort -u
)"

result=""
while IFS= read -r name; do
  [ -n "$name" ] || continue
  # Resolve to the highest-precedence dir that holds this basename (repo > user >
  # base); existence there overrides, so a non-executable shadow masks the rest.
  provider=""
  for dir in "$repo_dir" "$user_dir" "$base_dir"; do
    if [ -f "$dir/$name" ]; then
      provider="$dir/$name"
      break
    fi
  done
  [ -n "$provider" ] && [ -x "$provider" ] || continue
  out="$(printf '%s' "$payload" | "$provider" 2>/dev/null)" && rc=0 || rc=$?
  [ "$rc" -eq 0 ] && [ -n "$out" ] || continue
  result="${result:+$result
}$out"
done <<EOF
$names
EOF

printf '%s' "$result"
