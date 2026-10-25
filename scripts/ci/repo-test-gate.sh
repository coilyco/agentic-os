#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
umbra_bin=${UMBRA_BIN:-umbra}
umbra_version=$(
  PLAN=1 TOOLS=umbra bash "$repo_root/.devcontainer/features/agentic-os-tools/install.sh" | cut -f2
)
if [ -z "$umbra_version" ]; then
  echo "the tools installer does not pin umbra" >&2
  exit 1
fi
# A runner or image may carry an older umbra than the pin, so a mismatch bootstraps it.
installed=$("$umbra_bin" --version 2>/dev/null | sed -n 's/^umbra version v\([^ ]*\).*/\1/p' || true)
if [ "$installed" != "$umbra_version" ]; then
  tmpdir=$(mktemp -d)
  cleanup() {
    rm -rf "$tmpdir"
  }
  trap cleanup EXIT HUP INT TERM

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

  base="https://github.com/coilyco/umbra/releases/download/v${umbra_version}"
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

just guard-controls
uv run pytest
pre-commit run --all-files
