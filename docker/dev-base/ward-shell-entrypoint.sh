#!/usr/bin/env bash
# Seed the canonical aos checkout before the director shell starts. Read-only
# container surfaces skip host rc files, so this wrapper restores the env from
# the checkout that built the image rather than whatever cwd a child command
# happens to use later.

set -u

# The image owns deployment identity. Replace neutral WARD_GIT_* launcher
# defaults before Ward bootstraps Git in the container.
export WARD_GIT_NAME="${AOS_GIT_NAME:?AOS_GIT_NAME is required}"
export WARD_GIT_EMAIL="${AOS_GIT_EMAIL:?AOS_GIT_EMAIL is required}"
unset WARD_CONFIG_REF

_siren_aos_root=${AOS_REPO_ROOT:-${FORGEJO_WORKSPACE:-${GITHUB_WORKSPACE:-/workspace/agentic-os}}}
if ! git -C "$_siren_aos_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  for _siren_aos_root in \
    /workspace/coilyco-flight-deck/agentic-os \
    /workspace/agentic-os \
    "$HOME/projects/coilyco-flight-deck/agentic-os"; do
    if git -C "$_siren_aos_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      break
    fi
  done
  _siren_entrypoint_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
  _siren_aos_root="$(cd "$_siren_entrypoint_dir/../.." && pwd -P)"
fi
if git -C "$_siren_aos_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  export AOS_REPO_ROOT="$_siren_aos_root"
fi

if [ "$#" -gt 0 ]; then
  exec "$@"
fi

exec "${SHELL:-/bin/bash}"
