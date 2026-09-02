#!/usr/bin/env bash
# Vendor the Forgejo operator policy into deploy. docs/vendor-forgejo-policy.md.
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
src_dir="$repo_root/.umbra/guardfiles/aosguard"
target_repo="${TARGET_REPO:-coilyco-bridge/deploy}"
target_dir="services/forgejo-mcp/vendor/aosguard"
host="forgejo.coilysiren.me"
sha=$(git -C "$repo_root" rev-parse HEAD)
branch="vendor/forgejo-policy-${sha:0:12}"

if [ -z "${DEPLOY_WRITE_TOKEN:-}" ]; then
  echo "::warning::DEPLOY_WRITE_TOKEN is absent; skipping the deploy vendor push" >&2
  exit 0
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

git clone --depth 1 "https://${host}/${target_repo}.git" "$work/deploy"
cd "$work/deploy"
mkdir -p "$target_dir"
cp "$src_dir/forgejo.kdl" "$src_dir/forgejo.swagger.v1.json.gz" "$target_dir/"

# The pin is what makes the copy auditable: it names the commit the bytes came
# from, so deploy answers "which policy is this" without guessing.
printf 'repo: coilyco-flight-deck/agentic-os\ncommit: %s\nfiles: forgejo.kdl forgejo.swagger.v1.json.gz\n' \
  "$sha" >"$target_dir/SOURCE"

git add "$target_dir"
if git diff --cached --quiet; then
  echo "vendor-forgejo-policy: deploy already carries ${sha:0:12}"
  exit 0
fi

git config user.name "coilyco-ops"
git config user.email "coilyco-ops@coilysiren.me"
git commit -m "chore(forgejo-mcp): vendor aosguard Forgejo policy at ${sha:0:12}"
git push "https://coilyco-ops:${DEPLOY_WRITE_TOKEN}@${host}/${target_repo}.git" \
  "HEAD:${branch}"

# A pushed branch owes its pull request, or nothing points at the vendored copy.
body=$(printf 'Pushed by coilyco-flight-deck/agentic-os. Source commit: %s.\n\nAuthored there, rolled out here: deploy never fetches this upward at build or run time (agentic-os#1376).' "$sha")
python3 - "$branch" "${sha:0:12}" "$body" >"$work/pr.json" <<'PY'
import json
import sys

branch, short, body = sys.argv[1:4]
json.dump(
    {
        "title": f"Vendor the aosguard Forgejo policy at {short}",
        "head": branch,
        "base": "main",
        "body": body,
    },
    sys.stdout,
)
PY

curl --retry 3 --retry-all-errors -fsSL -X POST \
  -H "Authorization: token ${DEPLOY_WRITE_TOKEN}" \
  -H "Content-Type: application/json" \
  --data-binary "@$work/pr.json" \
  "https://${host}/api/v1/repos/${target_repo}/pulls" >/dev/null

echo "vendor-forgejo-policy: opened a pull request on ${branch}"
