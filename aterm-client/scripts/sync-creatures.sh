#!/usr/bin/env bash
# Copy each role's creature out of an agentic-os checkout as a small WebP.
# macOS only: iconutil unpacks the .icns aterm embeds. Provenance: docs/architecture.md.
set -euo pipefail

source_dir="${1:?usage: sync-creatures.sh <agentic-os checkout>}/aterm/icons"
target_dir="$(cd "$(dirname "$0")/.." && pwd)/src/assets/creatures"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$target_dir"
for icns in "$source_dir"/*.icns; do
  role="$(basename "$icns" .icns)"
  iconutil -c iconset "$icns" -o "$work/$role.iconset"
  magick "$work/$role.iconset/icon_256x256.png" -resize 160x160 -quality 82 "$target_dir/$role.webp"
  echo "synced $role"
done
