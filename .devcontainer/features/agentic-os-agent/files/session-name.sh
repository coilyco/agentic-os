#!/usr/bin/env bash
# SessionStart announcement: tell the agent what it is called.
# See docs/features-agents.md.
set -euo pipefail

payload="$(cat)"
payload_flat="$(printf '%s' "$payload" | tr -d '\n')"

# Claude Code passes the project root via workspace.project_dir (preferred) or
# cwd. Fall back to $CLAUDE_PROJECT_DIR / $PWD only if both are absent.
project_dir="$(printf '%s' "$payload_flat" \
  | sed -n 's/.*"workspace"[[:space:]]*:[[:space:]]*{[^}]*"project_dir"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
[ -z "$project_dir" ] && project_dir="$(printf '%s' "$payload_flat" \
  | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
[ -z "$project_dir" ] && project_dir="${CLAUDE_PROJECT_DIR:-$PWD}"

# Agent Compose is the single authority; this never derives a name of its own.
# See docs/dev-base-agent-identity.md.
command -v acompose >/dev/null 2>&1 || exit 0
name="$(acompose whoami --target "$project_dir" 2>/dev/null || true)"

# No projection means no composed name. Announce nothing rather than invent one.
[ -n "$name" ] || exit 0

printf '🐾 You are %s this session. When asked "who are you" or "what is your status", lead with this name.\n' "$name"
