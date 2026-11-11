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
# COI-50's stable identity: a self-signed certificate, so the requirement pins no build.
sign_name="Coilyco Code Signing"
sign_keychain="$HOME/Library/Keychains/treehouse-signing.keychain-db"

fail() {
  echo "error: $*" >&2
  exit 1
}

[ "$(uname -s)" = Darwin ] || fail "the windowed app builds on a Mac only"

test_shell() {
  (cd "$tauri_dir" && cargo test)
}

# The same client bundle the Android app ships, then the Tauri shell around it.
# tauri.macos.conf.json is merged over tauri.conf.json on this OS only.
build() {
  (cd "$repo_root" && just aterm-client-install && VITE_ATERM_APP=1 just aterm-client-build && just aterm-android-install)
  local version config=()
  version=$(app_version || true)
  [ -z "$version" ] || config=(--config "{\"version\":\"$version\"}")
  (cd "$repo_root/aterm-android" && pnpm exec tauri build --ci --bundles app ${config[@]+"${config[@]}"})
  [ -d "$built" ] || fail "no app at $built"
  sign_app "$built"
  codesign --verify --deep --strict "$built"
  echo "built $built ${version:-with no release tag}"
  codesign -d -r- "$built" 2>&1 | grep '^designated' || true
}

# The version the app shows is the client's: the newest aterm-android release tag, or
# TREEHOUSE_VERSION where the tree carries no tags. Nothing prints when neither is there.
app_version() {
  local found=${TREEHOUSE_VERSION:-}
  [ -n "$found" ] || found=$(git -C "$repo_root" tag --list 'aterm-android-v*' --sort=-v:refname 2>/dev/null | head -n1)
  found=${found#aterm-android-v}
  printf '%s' "$found" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' && printf '%s' "$found"
}

identity_present() {
  [ -f "$sign_keychain" ] && security find-identity -p codesigning "$sign_keychain" | grep -Fq "$sign_name"
}

# Tauri leaves a linker signature that seals no Info.plist, so sign again under the bundle
# id. Ad-hoc is a new identity per build to macOS, and its privacy grants do not survive.
sign_app() {
  if [ -n "${TREEHOUSE_SIGN_IDENTITY:-}" ]; then
    codesign --force --deep --sign "$TREEHOUSE_SIGN_IDENTITY" --identifier "$bundle_id" "$1"
  elif identity_present; then
    security unlock-keychain -p "" "$sign_keychain"
    codesign --force --deep --keychain "$sign_keychain" --sign "$sign_name" --identifier "$bundle_id" "$1"
  elif [ -n "${TREEHOUSE_SIGN_ADHOC:-}" ]; then
    codesign --force --deep --sign - --identifier "$bundle_id" "$1"
  else
    fail "no signing identity. Run the sign-setup verb, or set TREEHOUSE_SIGN_ADHOC=1 for a throwaway build"
  fi
}

# Reads the certificate and key from SSM once into a keychain of their own, so an
# unattended build needs no AWS session. Mode 600 files, gone on exit, never echoed.
sign_setup() {
  if identity_present; then echo "sign-setup: unchanged"; return; fi
  command -v aosguard >/dev/null || fail "aosguard is not on PATH"
  (
    umask 077
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    for part in cert key; do
      aosguard ops aws ssm get-parameter --name "/coilysiren/codesign/aterm-$part" --with-decryption \
        --query Parameter.Value --output text > "$work/$part.pem" || fail "could not read the $part from SSM"
    done
    TREEHOUSE_P12_PASS=$(openssl rand -hex 16)
    export TREEHOUSE_P12_PASS
    openssl pkcs12 -export -inkey "$work/key.pem" -in "$work/cert.pem" -name "$sign_name" \
      -passout env:TREEHOUSE_P12_PASS -out "$work/id.p12"
    [ -f "$sign_keychain" ] || security create-keychain -p "" "$sign_keychain"
    security set-keychain-settings "$sign_keychain"
    security unlock-keychain -p "" "$sign_keychain"
    security import "$work/id.p12" -k "$sign_keychain" -P "$TREEHOUSE_P12_PASS" -T /usr/bin/codesign >/dev/null
    security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "" "$sign_keychain" >/dev/null
  )
  identity_present || fail "the identity is not in $sign_keychain after the import"
  echo "sign-setup: imported $sign_name"
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
  version) app_version ;;
  sign-setup) sign_setup ;;
  install) install ;;
  login) shift; login "${1:-status}" ;;
  *) fail "usage: treehouse-mac.sh test|build|version|sign-setup|install|login [on|off|status]" ;;
esac
