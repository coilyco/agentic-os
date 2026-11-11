#!/usr/bin/env bash
# Regenerate the Treehouse icons from the website canopy-up logo, never redrawn.
# Writes the Android launcher set, the Tauri source icon (the Mac app too), and web icons.
# Usage and geometry: tooling-aterm-client references/android.md.

set -euo pipefail

src="${1:?usage: aterm-android-icons.sh TREEHOUSE_DIR  (coilyco/website mark-exports/treehouse)}"
repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
icons="$repo_root/aterm-android/src-tauri/icons"
web="$repo_root/aterm-client/public"
export_svg="$src/treehouse-canopy-up-dark.svg"
# The ink ground is --plate in the client token block, the one source the OS chrome reads.
ink=$(sed -nE 's/^[[:space:]]*--plate:[[:space:]]*(#[0-9a-fA-F]{6});.*/\1/p' "$repo_root/aterm-client/src/app.css")
[ -n "$ink" ] || { echo "no --plate as #rrggbb in aterm-client/src/app.css" >&2; exit 1; }
# The canopy square's share of the 108dp adaptive canvas. Its mark is 57% wide, 71% tall
# inside it, and the safe zone is a 66dp circle (61%), so 85% keeps all of it inside.
fg_share=85
# The inscribed square of a round legacy icon, which fits the rounded square too.
legacy_share=70
# A maskable web icon keeps its content inside the central 80 percent.
maskable_share=80

[ -f "$export_svg" ] || { echo "missing $export_svg" >&2; exit 1; }
command -v magick >/dev/null || { echo "ImageMagick (magick) is required" >&2; exit 1; }
command -v rsvg-convert >/dev/null || { echo "rsvg-convert (librsvg) is required" >&2; exit 1; }

# The export draws the mark on its own ground rect. Swap only that rect's fill for the ink
# and render it, so the mark's paths stay as exported and the tile follows the token.
canopy_svg=$(mktemp -t canopy.XXXXXX)
canopy=$(mktemp -t canopy.XXXXXX)
trap 'rm -f "$canopy_svg" "$canopy"' EXIT
sed -E 's/(<rect x="-20" y="-20" width="140" height="140" fill=")#[0-9a-fA-F]{6}("\/>)/\1'"$ink"'\2/' "$export_svg" >"$canopy_svg"
grep -q "fill=\"$ink\"/><g" "$canopy_svg" || { echo "no ground rect to retint in $export_svg" >&2; exit 1; }
rsvg-convert -w 512 -h 512 "$canopy_svg" -o "$canopy"

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

# The status bar's small icon, drawn flat since Android keeps only its alpha: the client's
# canopy at 40% alpha or more, white, trimmed to fill 20dp of a 24dp square (COI-3613).
stat_src="$repo_root/aterm-client/src/assets/treehouse-canopy.svg"
stat=$(mktemp -t canopy.XXXXXX)
trap 'rm -f "$canopy_svg" "$canopy" "$stat"' EXIT
rsvg-convert -w 512 -h 512 "$stat_src" -o "$stat"
for spec in mdpi:100 hdpi:150 xhdpi:200 xxhdpi:300 xxxhdpi:400; do
  dir="$icons/android/drawable-${spec%%:*}"
  k=${spec##*:}
  mkdir -p "$dir"
  png "PNG32:$dir/ic_stat_treehouse.png" "$stat" -channel A -threshold 40% +channel -fill white -colorize 100 \
    -trim +repage -filter Lanczos -resize "$((20 * k / 100))x$((20 * k / 100))" \
    -background none -gravity center -extent "$((24 * k / 100))x$((24 * k / 100))"
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
