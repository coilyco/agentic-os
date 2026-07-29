#!/usr/bin/env bash
set -euo pipefail

ref="${1:?usage: install-workflow-ward.sh <ward-ref> [output-path]}"
out="${2:-/usr/local/bin/ward}"
tmpdir="$(mktemp -d)"

cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

git -c advice.detachedHead=false clone --depth 1 --branch "${ref}" \
  https://forgejo.coilysiren.me/coilyco-flight-deck/ward.git "${tmpdir}/ward"
cd "${tmpdir}/ward"
go build -trimpath -ldflags "-s -w -X main.Version=${ref}" -o "${out}" ./cmd/ward
chmod 0755 "${out}"
"${out}" --version
