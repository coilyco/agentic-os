#!/usr/bin/env bash
# Install the pinned rcodesign on a hosted Linux runner, for the macOS signing
# step of the aterm release (COI-50). See docs/aos-cli.md.

set -euo pipefail

version=0.29.0
archive_sha256=dbe85cedd8ee4217b64e9a0e4c2aef92ab8bcaaa41f20bde99781ff02e600002
name=apple-codesign-${version}-x86_64-unknown-linux-musl
base=${RCODESIGN_BASE:-https://github.com/indygreg/apple-platform-rs/releases/download/apple-codesign/${version}}
dest=${RCODESIGN_DEST:-${RUNNER_TEMP:?RUNNER_TEMP is not set}/rcodesign}

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64 | Linux/amd64) ;;
  *)
    echo "the pinned rcodesign is linux-x86_64 only, got $(uname -s)/$(uname -m)" >&2
    exit 1
    ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$base/$name.tar.gz" -o "$work/$name.tar.gz"
echo "$archive_sha256  $work/$name.tar.gz" | sha256sum -c - >/dev/null || {
  echo "$name.tar.gz does not match the pinned checksum" >&2
  exit 1
}

mkdir -p "$dest/bin"
tar -xzf "$work/$name.tar.gz" -C "$work"
install -m 0755 "$work/$name/rcodesign" "$dest/bin/rcodesign"
echo "$dest/bin" >> "${GITHUB_PATH:-/dev/null}"
"$dest/bin/rcodesign" --version
