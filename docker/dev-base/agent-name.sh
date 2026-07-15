#!/usr/bin/env bash
# In-container self-name for warded agents. See docs/dev-base-self-name.md.
# Derivation MUST stay format-identical to the host scripts/agent-name.sh.
set -euo pipefail

mode="${1:-statusline}"

payload="$(cat)"
payload_flat="$(printf '%s' "$payload" | tr -d '\n')"
sid="$(printf '%s' "$payload_flat" \
  | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"

# Harness registry (prefix = harness name, value = pronoun slug), picked via
# AOS_AGENT_HARNESS (default claude). Identical to the host script's registry.
harness="${AOS_AGENT_HARNESS:-claude}"
pronoun_slug_for() {
  case "$1" in
    claude)   printf 'she-her' ;;
    codex)    printf 'he-him' ;;
    opencode) printf 'they-them' ;;
    aider)    printf 'they-them' ;;
    goose)    printf 'she-her' ;;
    *)        printf 'she-her' ;; # unknown harness: default she-her
  esac
}

# Compute the name locally. In a container os is linux and host the container
# hostname, so the name encodes which explorer is which.
local_name() {
  local os host tag sid_lc letters digits
  case "$(uname -s)" in
    Darwin)               os=macos ;;
    Linux)                os=linux ;;
    MINGW* | MSYS* | CYGWIN*) os=windows ;;
    *)                    os="$(uname -s | tr '[:upper:]' '[:lower:]')" ;;
  esac
  host="$(hostname -s 2>/dev/null || hostname)"
  host="$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]' | cut -d. -f1)"
  sid_lc="$(printf '%s' "$sid" | tr '[:upper:]' '[:lower:]')"
  letters="$(printf '%s' "$sid_lc" | tr -cd 'a-z' | cut -c1-2)"
  digits="$(printf '%s' "$sid_lc" | tr -cd '0-9' | cut -c1-2)"
  tag="${letters}${digits}"
  printf '%s-%s-%s' "$harness" "$os" "$host"
  [ -n "$tag" ] && printf -- '-%s' "$tag"
  printf -- '-%s' "$(pronoun_slug_for "$harness")"
}

# Stable per session, cached so a statusline refresh does not recompute. Key on
# harness too, so two harnesses never share a cached name.
cache="${TMPDIR:-/tmp}/agent-name-${harness}-${sid:-nosession}"
if [ -s "$cache" ]; then
  name="$(cat "$cache")"
else
  name="$(local_name)"
  printf '%s' "$name" >"$cache"
fi

# Human-readable pronouns parsed from the name's trailing slug.
pronoun_display() {
  case "$1" in
    *-she-her)   printf 'she/her' ;;
    *-he-him)    printf 'he/him' ;;
    *-they-them) printf 'they/them' ;;
  esac
}
pronouns="$(pronoun_display "$name")"

# Container-name suffix from ward's WARD_CONTAINER_NAME (the friendly docker
# name; inside, HOSTNAME is only the opaque ID). See docs/dev-base-self-name.md.
container_suffix=""
[ -n "${WARD_CONTAINER_NAME:-}" ] && container_suffix="  [${WARD_CONTAINER_NAME}]"

case "$mode" in
  sessionstart)
    printf '🐾 You are %s%s this session. When asked "who are you" or "what is your status", lead with this name.\n' "$name" "${pronouns:+ ($pronouns)}"
    ;;
  gitidentity)
    # Respect the baked git system config from the Dockerfile. Only backfill
    # a user-scoped identity when the image or runtime environment lacks one.
    if ! git config --get user.name >/dev/null 2>&1; then
      git config --global user.name "${WARD_GIT_NAME:-coilyco-ops}" 2>/dev/null || true
    fi
    if ! git config --get user.email >/dev/null 2>&1; then
      git config --global user.email "${WARD_GIT_EMAIL:-coilyco-ops@coilysiren.me}" 2>/dev/null || true
    fi
    ;;
  statusline | *)
    printf '%s%s' "$name" "$container_suffix"
    ;;
esac
