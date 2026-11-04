#!/usr/bin/env bash
# Regenerate the aterm Android launcher icons from the website brand kit, never redrawn.
# Usage and geometry: tooling-aterm-client references/android.md.

set -euo pipefail

brand="${1:?usage: aterm-android-icons.sh BRAND_DIR  (coilyco/website src/images/brand)}"
repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
icons="$repo_root/aterm-android/src-tauri/icons"
mark="$brand/coilyco-mark-colour-dark.png"
avatar="$brand/coilyco-avatar-dark.png"
# The ink ground behind the colour coil, from the avatar SVG's background rect.
ink="#080711"
# The mark's share of the 108dp adaptive canvas, by height. The safe zone is a 66dp circle
# (61%) and the tail makes the mark as tall as it is far from centre, so 58% leaves slack.
fg_share=58
legacy_share=64

for f in "$mark" "$avatar"; do
  [ -f "$f" ] || { echo "missing $f" >&2; exit 1; }
done
command -v magick >/dev/null || { echo "ImageMagick (magick) is required" >&2; exit 1; }

# png OUT ARGS...: a reproducible PNG, with no timestamp chunks to churn the diff.
png() { local out=$1; shift; magick "$@" -strip -depth 8 -define png:exclude-chunks=date,time "$out"; }

# density K*100 (mdpi 100 ... xxxhdpi 400)
for spec in mdpi:100 hdpi:150 xhdpi:200 xxhdpi:300 xxxhdpi:400; do
  dir="$icons/android/mipmap-${spec%%:*}"
  k=${spec##*:}
  canvas=$((108 * k / 100))
  legacy=$((48 * k / 100))
  mkdir -p "$dir"
  # Foreground layer: the coil alone on transparent, centred by its ink box.
  png "$dir/ic_launcher_foreground.png" -size "${canvas}x${canvas}" xc:none \
    \( "$mark" -trim +repage -filter Lanczos -resize "x$((canvas * fg_share / 100))" \) \
    -gravity center -composite
  # Legacy icons, unused from API 26 up but in the Tauri template: an ink tile and disc.
  for shape in square round; do
    if [ "$shape" = round ]; then
      draw="circle $((legacy / 2)),$((legacy / 2)) $((legacy / 2)),0"
      out="$dir/ic_launcher_round.png"
    else
      draw="roundrectangle 0,0 $((legacy - 1)),$((legacy - 1)) $((legacy / 5)),$((legacy / 5))"
      out="$dir/ic_launcher.png"
    fi
    png "$out" -size "${legacy}x${legacy}" xc:none -fill "$ink" -draw "$draw" \
      \( "$mark" -trim +repage -filter Lanczos -resize "x$((legacy * legacy_share / 100))" \) \
      -gravity center -composite
  done
done

mkdir -p "$icons/android/values"
cat >"$icons/android/values/ic_launcher_background.xml" <<EOF
<?xml version="1.0" encoding="utf-8"?>
<resources>
  <color name="ic_launcher_background">$ink</color>
</resources>
EOF

# The 512px source the Tauri bundle config names. It is the avatar as published.
png "$icons/icon.png" "$avatar" -filter Lanczos -resize 512x512
echo "icons written under $icons"
