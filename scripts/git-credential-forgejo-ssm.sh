#!/usr/bin/env bash
# Git credential helper: serve the Forgejo API token from AWS SSM on demand.
# Git passes the op as $1 (get|store|erase) and the request on stdin.
# For a "get" against forgejo.coilysiren.me we fetch the coilyco-ops token and
# print username/password. The token stays in process memory, never written to disk.
# Wire via `git config --global credential.<host>.helper` pointing at this path.
# A burst of requests, such as a partial clone's lazy fetches, rides git's
# credential-cache daemon (memory only) instead of an SSM read each, as install.md says.
# store is a no-op, and erase drops the cached copy so a rotated token is fetched anew.
set -euo pipefail

# git invokes helpers with a minimal env, so aws may not be on PATH.
# Prepend the fleet install dirs: linuxbrew, homebrew, /usr/local, ~/.local.
export PATH="/home/linuxbrew/.linuxbrew/bin:/opt/homebrew/bin:/usr/local/bin:${HOME}/.local/bin:${PATH}"
# Ward configuration selects an agent runtime surface. It has no place in this
# repository-access bootstrap and can interfere with nested credential lookup.
unset WARD_CONFIG_REF

# Seconds a token stays cached, 0 to turn the cache off. credential-cache needs unix
# sockets, so where git lacks it every call below fails quietly and each get reads SSM.
ttl="${FORGEJO_CREDENTIAL_CACHE_SECONDS:-600}"
forgejo_host="${FORGEJO_CREDENTIAL_HOST:-forgejo.coilysiren.me}"
aws_cmd="${FORGEJO_CREDENTIAL_AWS:-aws}"

# One cache entry per host, whatever path or user a request carries.
cache() {
  [ "$ttl" -gt 0 ] 2>/dev/null || return 1
  git credential-cache --timeout "$ttl" "$@" 2>/dev/null
}
cache_key() { printf 'protocol=https\nhost=%s\n' "$forgejo_host"; }

op="${1:-}"
case "$op" in
  get | erase) ;;
  *) exit 0 ;;
esac

host=""
while IFS='=' read -r key value; do
  [ -z "$key" ] && break
  case "$key" in
    host) host="$value" ;;
  esac
done

# Only answer for the Forgejo host; let git fall through for anything else.
[ "$host" = "$forgejo_host" ] || exit 0

# Git reports a rejected credential with erase. Dropping the copy is what makes a
# rotated token work on the next request instead of after the TTL.
if [ "$op" = "erase" ]; then
  { cache_key; echo; } | cache erase || true
  exit 0
fi

# Only the two attributes git needs: newer git also sends capability lines.
if cached="$({ cache_key; echo; } | cache get | grep -E '^(username|password)=')" && [ -n "$cached" ]; then
  printf '%s\n' "$cached"
  exit 0
fi

# Git credentials bootstrap repository access, including the checkout that owns
# the guarded operator spec, so direct AWS CLI use is the bootstrap exception.
token="$("$aws_cmd" ssm get-parameter \
  --name /forgejo/coilyco-ops/api-token --with-decryption \
  --query Parameter.Value --output text 2>/dev/null)" || exit 0
[ -n "$token" ] || exit 0

printf 'username=coilyco-ops\n'
printf 'password=%s\n' "$token"
{ cache_key; printf 'username=coilyco-ops\npassword=%s\n\n' "$token"; } | cache store || true
