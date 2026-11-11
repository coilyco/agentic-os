#!/usr/bin/env bash
# Build, install and wire up the windowed Treehouse app on a Mac. The just verbs call it.
# What each step does and why: tooling-aterm-client references/desktop.md.

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tauri_dir="$repo_root/aterm-android/src-tauri"
app_name=Treehouse
bundle_id=dev.coilyco.treehouse.desktop
built="$tauri_dir/target/release/bundle/macos/$app_name.app"
installed="$HOME/Applications/$app_name.app"
# The login item the app replaces, so a Mac has one startup item and never two.
replaces="${TREEHOUSE_REPLACES:-Portfolio Director}"
replaces_path="$HOME/Applications/$replaces.app"

fail() {
  echo "error: $*" >&2
  exit 1
}

[ "$(uname -s)" = Darwin ] || fail "the windowed app builds on a Mac only"

test_shell() {
  (cd "$tauri_dir" && cargo test)
}

# The Android app's client bundle, then the Tauri shell around it. The bundle lists this
# Mac's loopback daemon from the build (COI-3476).
local_host=ws://127.0.0.1:7419/
build() {
  (cd "$repo_root" && just aterm-client-install \
    && VITE_ATERM_APP=1 VITE_ATERM_LOCAL_HOST="$local_host" VITE_ATERM_LOCAL_LABEL="this Mac" just aterm-client-build \
    && just aterm-android-install)
  (cd "$repo_root/aterm-android" && pnpm exec tauri build --ci --bundles app)
  [ -d "$built" ] || fail "no app at $built"
  # Tauri leaves the linker's ad-hoc signature, with no Info.plist or resources sealed.
  # Sign again under the bundle id. TREEHOUSE_SIGN_IDENTITY: desktop.md, Not yet.
  codesign --force --deep --sign "${TREEHOUSE_SIGN_IDENTITY:--}" --identifier "$bundle_id" "$built"
  codesign --verify --deep --strict "$built"
  echo "built $built"
}

install() {
  [ -d "$built" ] || fail "no build at $built, run the build verb first"
  mkdir -p "$HOME/Applications"
  # A running copy holds its files, and ditto over it would swap the binary under it.
  ! pgrep -f "$installed/Contents/MacOS" >/dev/null || fail "$app_name is running, quit it first"
  [ ! -e "$installed" ] || rm -rf "$installed"
  ditto "$built" "$installed"
  echo "installed $installed"
}

login_items() {
  osascript -e 'tell application "System Events" to get the name of every login item'
}

has_item() {
  login_items | tr ',' '\n' | sed 's/^ *//' | grep -Fxq "$1"
}

remove_item() {
  osascript -e "tell application \"System Events\" to delete (every login item whose name is \"$1\")" >/dev/null
}

add_item() {
  osascript -e "tell application \"System Events\" to make login item at end with properties {path:\"$1\", hidden:false}" >/dev/null
}

# on adds Treehouse before it removes the old item, and puts back what it can if the
# removal fails, so a failed run leaves one startup item and never two or none.
login() {
  case "${1:-status}" in
    on)
      [ -d "$installed" ] || fail "$installed is not installed, run the install verb first"
      login_items >/dev/null || fail "System Events refused the login item list"
      if has_item "$app_name" && ! has_item "$replaces"; then
        echo "login: unchanged"
      else
        has_item "$app_name" || add_item "$installed"
        if has_item "$replaces" && ! remove_item "$replaces"; then
          remove_item "$app_name" || true
          fail "could not remove the $replaces login item, so $app_name was taken off again"
        fi
        echo "login: changed"
      fi
      ;;
    off)
      [ ! -d "$replaces_path" ] || has_item "$replaces" || add_item "$replaces_path"
      remove_item "$app_name"
      ;;
    status) ;;
    *) fail "login takes on, off or status" ;;
  esac
  login_items
}

case "${1:-}" in
  test) test_shell ;;
  build) build ;;
  install) install ;;
  login) shift; login "${1:-status}" ;;
  *) fail "usage: treehouse-mac.sh test|build|install|login [on|off|status]" ;;
esac
