#!/usr/bin/env bash
# Regenerate the Treehouse icons from the website canopy-up logo, never redrawn.
# Writes the Android launcher set, the Tauri source icon (the Mac app too), and web icons.
# Usage and geometry: tooling-aterm-client references/android.md.

set -euo pipefail

src="${1:?usage: aterm-android-icons.sh TREEHOUSE_DIR  (coilyco/website mark-exports/treehouse)}"
repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
icons="$repo_root/aterm-android/src-tauri/icons"
web="$repo_root/aterm-client/public"
canopy="$src/treehouse-canopy-up-dark-512.png"
canopy_svg="$src/treehouse-canopy-up-dark.svg"
# The ink ground the canopy is drawn on, from its SVG's background rect.
ink="#080711"
# The canopy square's share of the 108dp adaptive canvas. Its mark is 57% wide, 71% tall
# inside it, and the safe zone is a 66dp circle (61%), so 85% keeps all of it inside.
fg_share=85
# The inscribed square of a round legacy icon, which fits the rounded square too.
legacy_share=70
# A maskable web icon keeps its content inside the central 80 percent.
maskable_share=80

for f in "$canopy" "$canopy_svg"; do
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
  # Foreground layer: the canopy square on ink, the same colour as the background layer.
  png "$dir/ic_launcher_foreground.png" -size "${canvas}x${canvas}" "xc:$ink" \
    \( "$canopy" -filter Lanczos -resize "x$((canvas * fg_share / 100))" \) \
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
      \( "$canopy" -filter Lanczos -resize "x$((legacy * legacy_share / 100))" \) \
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

# The 512px source the Tauri bundle names, which the Mac app takes. Tauri refuses a PNG
# that is not RGBA, so PNG32 keeps an opaque alpha channel.
png "PNG32:$icons/icon.png" "$canopy" -filter Lanczos -resize 512x512

# The web client's icons: the same canopy, resized, with a padded copy for maskable.
cp "$canopy_svg" "$web/icon.svg"
png "$web/icon-512.png" "$canopy" -filter Lanczos -resize 512x512
png "$web/icon-192.png" "$canopy" -filter Lanczos -resize 192x192
png "$web/icon-maskable-512.png" -size 512x512 "xc:$ink" \
  \( "$canopy" -filter Lanczos -resize "x$((512 * maskable_share / 100))" \) \
  -gravity center -composite
echo "icons written under $icons and $web"
