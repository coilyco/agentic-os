#!/usr/bin/env bash
# Rotate the forgejo container-registry publish token used by the release
# workflow's publish-image job (.forgejo/workflows/release.yml). The token is
# owned by the coilyco-ops bot, scoped write:package, and lands in two places:
# SSM (/forgejo/coilyco-ops/registry-token, the durable record) and the repo
# Actions secret REGISTRY_TOKEN (what the workflow's `docker login` reads).
#
# Forgejo only mints tokens via POST /users/{username}/tokens under HTTP basic
# auth (token auth is rejected, and there is no admin mint endpoint), so this
# authenticates as the bot with /forgejo/coilyco-ops/password to mint, then uses
# the admin /forgejo/api-token only to write the repo Actions secret. The minted
# token carries no expiry. It never touches disk or stdout - it flows
# mint -> verify -> SSM -> Actions secret entirely in process memory.
#
# Why a script and not a one-off: forgejo PATs can be revoked or expire, and the
# auto-issued Actions token can read but not write the registry (docs/dev-base-image.md),
# so this is the documented recovery path when publish-image starts failing
# `docker login` with `unauthorized`. Re-run it to mint a fresh token.
set -euo pipefail

HOST="forgejo.coilysiren.me"
BOT_USER="coilyco-ops"
REPO="coilyco-flight-deck/agentic-os"
SECRET_NAME="REGISTRY_TOKEN"
SSM_PATH="/forgejo/coilyco-ops/registry-token"
# Token names are unique per user; stamp the name so re-runs never collide.
TOKEN_NAME="registry-publish-$(date +%Y%m%d%H%M%S)"

api="https://${HOST}/api/v1"

aguard_ssm() { aguard ops aws ssm "$@"; }

admin_token="$(aguard_ssm get-parameter --name /forgejo/api-token \
  --with-decryption --query Parameter.Value --output text)"
[ -n "$admin_token" ] || { echo "no /forgejo/api-token in SSM" >&2; exit 1; }
bot_password="$(aguard_ssm get-parameter --name /forgejo/${BOT_USER}/password \
  --with-decryption --query Parameter.Value --output text)"
[ -n "$bot_password" ] || { echo "no /forgejo/${BOT_USER}/password in SSM" >&2; exit 1; }

echo "Minting write:package token '${TOKEN_NAME}' for ${BOT_USER} ..."
mint_resp="$(curl -fsS -X POST \
  -u "${BOT_USER}:${bot_password}" \
  -H "Content-Type: application/json" \
  "${api}/users/${BOT_USER}/tokens" \
  -d "{\"name\":\"${TOKEN_NAME}\",\"scopes\":[\"write:package\"]}")"

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

echo "Setting repo Actions secret ${SECRET_NAME} on ${REPO} ..."
curl -fsS -X PUT \
  -H "Authorization: token ${admin_token}" \
  -H "Content-Type: application/json" \
  "${api}/repos/${REPO}/actions/secrets/${SECRET_NAME}" \
  -d "{\"data\":$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$new_token")}" >/dev/null
echo "  set"

echo "Done. Re-run the failed publish-image job to confirm push works."
