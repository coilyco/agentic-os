#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
umbra_bin=${UMBRA_BIN:-umbra}
if ! command -v "$umbra_bin" >/dev/null 2>&1; then
  tmpdir=$(mktemp -d)
  cleanup() {
    rm -rf "$tmpdir"
  }
  trap cleanup EXIT HUP INT TERM

  umbra_version=$(
    sed -n 's/^ARG UMBRA_VERSION=//p' \
      "$repo_root/docker/dev-base/full/Dockerfile" | tr -d '\r'
  )
  if [ -z "$umbra_version" ]; then
    echo "Dockerfile does not pin umbra" >&2
    exit 1
  fi

  host_os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$host_os" in
    linux) asset_os=linux ;;
    darwin) asset_os=darwin ;;
    mingw*|msys*|cygwin*) asset_os=windows ;;
    *)
      echo "unsupported host OS for umbra bootstrap: $host_os" >&2
      exit 1
      ;;
  esac
  host_arch=$(uname -m)
  case "$host_arch" in
    x86_64|amd64) asset_arch=amd64 ;;
    aarch64|arm64) asset_arch=arm64 ;;
    armv7l) asset_arch=armv7 ;;
    *)
      echo "unsupported host architecture for umbra bootstrap: $host_arch" >&2
      exit 1
      ;;
  esac
  asset="umbra-${asset_os}-${asset_arch}"
  if [ "$asset_os" = windows ]; then
    asset="${asset}.exe"
    umbra_path="$tmpdir/umbra.exe"
  else
    umbra_path="$tmpdir/umbra"
  fi

  base="https://forgejo.coilysiren.me/coilyco-flight-deck/umbra/releases/download/v${umbra_version}"
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
    "${base}/${asset}" -o "$tmpdir/$asset"
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
    "${base}/SHA256SUMS" -o "$tmpdir/SHA256SUMS"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmpdir" && grep "[[:space:]]${asset}$" SHA256SUMS | sha256sum -c -)
  else
    (cd "$tmpdir" && grep "[[:space:]]${asset}$" SHA256SUMS | shasum -a 256 -c -)
  fi
  mv "$tmpdir/$asset" "$umbra_path"
  chmod 0755 "$umbra_path"
  export PATH="$tmpdir:$PATH"
  umbra_bin="$umbra_path"
fi

uv run pytest
pre-commit run --all-files
