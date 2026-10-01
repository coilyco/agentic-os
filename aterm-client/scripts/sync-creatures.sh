#!/usr/bin/env bash
# Copy each role's creature out of this repository's aterm/icons as a small WebP.
# macOS only: iconutil unpacks the .icns aterm embeds. Provenance: the architecture
# reference of the tooling-aterm-client skill.
set -euo pipefail

client_dir="$(cd "$(dirname "$0")/.." && pwd)"
source_dir="${client_dir}/../aterm/icons"
target_dir="${client_dir}/src/assets/creatures"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir -p "$target_dir"
for icns in "$source_dir"/*.icns; do
  role="$(basename "$icns" .icns)"
  iconutil -c iconset "$icns" -o "$work/$role.iconset"
  magick "$work/$role.iconset/icon_256x256.png" -resize 160x160 -quality 82 "$target_dir/$role.webp"
  echo "synced $role"
done
