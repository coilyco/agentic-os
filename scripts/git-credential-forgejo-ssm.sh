#!/usr/bin/env bash
# Git credential helper: serve the Forgejo API token from AWS SSM on demand.
# Git passes the op as $1 (get|store|erase) and the request on stdin.
# For a "get" against forgejo.coilysiren.me we fetch the coilyco-ops token and
# print username/password. The token stays in process memory, never written to disk.
# Wire via `git config --global credential.<host>.helper` pointing at this path.
# store/erase are no-ops: nothing on disk, never cache-then-persist the value.
set -euo pipefail

# git invokes helpers with a minimal env, so aws may not be on PATH.
# Prepend the fleet install dirs: linuxbrew, homebrew, /usr/local, ~/.local.
export PATH="/home/linuxbrew/.linuxbrew/bin:/opt/homebrew/bin:/usr/local/bin:${HOME}/.local/bin:${PATH}"

op="${1:-}"
[ "$op" = "get" ] || exit 0

host=""
while IFS='=' read -r key value; do
  [ -z "$key" ] && break
  case "$key" in
    host) host="$value" ;;
  esac
done

# Only answer for the Forgejo host; let git fall through for anything else.
[ "$host" = "forgejo.coilysiren.me" ] || exit 0

# Git credentials bootstrap repository access, including the checkout that owns
# the guarded operator spec, so direct AWS CLI use is the bootstrap exception.
token="$(aws ssm get-parameter \
  --name /forgejo/coilyco-ops/api-token --with-decryption \
  --query Parameter.Value --output text 2>/dev/null)" || exit 0
[ -n "$token" ] || exit 0

printf 'username=coilyco-ops\n'
printf 'password=%s\n' "$token"
