#!/usr/bin/env bash

set -euo pipefail

if [ -z "${REL_ID:-}" ]; then
  echo "::error::no release_id from create-release. Cannot attach ward-specs asset" >&2
  exit 1
fi
asset="ward-specs-${TAG}.tar.gz"
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
  -C .ward --exclude=./ward.yaml -cf ward-specs.tar .
gzip -n -9 -c ward-specs.tar > "${asset}"
sha256sum "${asset}" | awk '{print $1}' > "${asset}.sha256"
echo "ward-specs asset sha256: $(cat "${asset}.sha256")"

existing=$(curl -fsSL -H "Authorization: token ${FORGEJO_TOKEN}" \
  "${FORGEJO_API}/releases/${REL_ID}/assets" || echo '[]')
for name in "${asset}" "${asset}.sha256"; do
  old=$(printf '%s' "$existing" | ASSET="$name" node -e '
    const a = JSON.parse(require("fs").readFileSync(0, "utf8") || "[]");
    const m = (a || []).find(x => x.name === process.env.ASSET);
    if (m) process.stdout.write(String(m.id));
  ')
  if [ -n "$old" ]; then
    curl -fsSL -X DELETE -H "Authorization: token ${FORGEJO_TOKEN}" \
      "${FORGEJO_API}/releases/${REL_ID}/assets/${old}" || true
  fi
  curl -fsSL -X POST -H "Authorization: token ${FORGEJO_TOKEN}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"${name}" \
    "${FORGEJO_API}/releases/${REL_ID}/assets?name=${name}"
  echo "uploaded ${name}"
done
