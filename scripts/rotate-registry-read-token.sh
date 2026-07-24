#!/usr/bin/env bash
# Rotate the least-privilege read:package token the docker credential helper
# (scripts/docker-credential-forgejo-ssm) serves for `docker pull` from the
# forgejo.coilysiren.me container registry. The token is owned by the coilyco-ops
# bot, scoped read:package, and lands in one place: SSM
# (/forgejo/coilyco-ops/registry-read-token, the durable record the helper reads
# on demand).
# Unlike the write:package publish token and the write:repository dep-bump token,
# this one is consumed by fleet laptops at pull time, not by a CI job, so there is
# no repo Actions secret to set - SSM is the whole handoff.
#
# Why this token exists (agentic-os#265): the docker helper is written to keep the
# general admin api-token out of docker auth. The dedicated read token once expired
# with no rotation owner, 401'd every laptop's pull, and got hot-patched to serve
# the admin token instead. This script is that missing owner: re-run it whenever
# the read token needs (re-)minting, and the helper picks it up. The minted token
# carries no expiry, so the original "silently expired" failure cannot recur.
#
# Mirrors scripts/rotate-registry-token.sh: forgejo only mints tokens via
# POST /users/{username}/tokens under HTTP basic auth, so this authenticates as the
# bot with /forgejo/coilyco-ops/password to mint. The minted token never touches
# disk or stdout - it flows mint -> verify -> SSM entirely in process memory.
set -euo pipefail

HOST="forgejo.coilysiren.me"
BOT_USER="coilyco-ops"
SSM_PATH="/forgejo/coilyco-ops/registry-read-token"
# Token names are unique per user; stamp the name so re-runs never collide.
TOKEN_NAME="registry-read-$(date +%Y%m%d%H%M%S)"

api="https://${HOST}/api/v1"

aguard_ssm() { aguard ops aws ssm "$@"; }

bot_password="$(aguard_ssm get-parameter --name /forgejo/${BOT_USER}/password \
  --with-decryption --query Parameter.Value --output text)"
[ -n "$bot_password" ] || { echo "no /forgejo/${BOT_USER}/password in SSM" >&2; exit 1; }

echo "Minting read:package token '${TOKEN_NAME}' for ${BOT_USER} ..."
mint_resp="$(curl -fsS -X POST \
  -u "${BOT_USER}:${bot_password}" \
  -H "Content-Type: application/json" \
  "${api}/users/${BOT_USER}/tokens" \
  -d "{\"name\":\"${TOKEN_NAME}\",\"scopes\":[\"read:package\"]}")"

new_token="$(printf '%s' "$mint_resp" | python3 -c 'import sys,json; print(json.load(sys.stdin)["sha1"])')"
[ -n "$new_token" ] || { echo "mint returned no sha1" >&2; exit 1; }
echo "  minted (value withheld)"

echo "Verifying token against ${HOST} container registry ..."
code="$(curl -sS -o /dev/null -w '%{http_code}' -u "${BOT_USER}:${new_token}" "https://${HOST}/v2/")"
[ "$code" = "200" ] || { echo "registry /v2/ returned ${code}, not 200" >&2; exit 1; }
echo "  /v2/ -> 200"

echo "Stashing to SSM ${SSM_PATH} ..."
aguard_ssm put-parameter --name "$SSM_PATH" --type SecureString \
  --value "$new_token" --overwrite >/dev/null
echo "  stored"

echo "Done. The docker credential helper now serves this least-privilege token; no laptop change needed."
