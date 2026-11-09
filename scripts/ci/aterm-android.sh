#!/usr/bin/env bash
# Build, sign, and verify the aterm Android APK. The workflow and the justfile
# verbs both call this, so a laptop build and a CI build run the same steps.
# What each step does and why: tooling-aterm-client references/android.md.

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)

# Every Android pin is manual. The command-line tools checksum is the vendor's
# published SHA-256 and is re-checked against the bytes at each download.
CMDLINE_TOOLS_REV=15859902
CMDLINE_TOOLS_SHA256=4e4c464f145a7512b57d088ac6c278c03c9eea610886b35a5e0804e74eedf583
ANDROID_PLATFORM=android-37.0
BUILD_TOOLS=36.0.0
NDK_VERSION=28.2.13676358
RUST_TARGET=aarch64-linux-android
APP_ID=dev.coilyco.treehouse
KEY_ALIAS=aterm
UPLOAD_ALIAS=upload
# Play's bundle tool, a pinned release jar. Audit and why: references/android.md.
BUNDLETOOL_VERSION=1.18.3
BUNDLETOOL_SHA256=a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29
# Every code shipped before the clock scheme is below this: major*1000000+minor*1000+patch
# stays under it while the major is 0. A code under it could not install over those.
VERSION_CODE_FLOOR=1000000
VERSION_CODE_MAX=2100000000

export ANDROID_HOME="${ANDROID_HOME:-$HOME/android-sdk}"
export NDK_HOME="$ANDROID_HOME/ndk/$NDK_VERSION"
build_tools_dir="$ANDROID_HOME/build-tools/$BUILD_TOOLS"
android_dir="$repo_root/aterm-android"
bundletool_jar="$ANDROID_HOME/bundletool/bundletool-$BUNDLETOOL_VERSION.jar"

fail() {
  echo "::error::$*" >&2
  exit 1
}

use_java() {
  local java_home
  for java_home in /usr/lib/jvm/java-17-openjdk-*; do
    [ -x "$java_home/bin/java" ] && export JAVA_HOME="$java_home" && break
  done
  [ -n "${JAVA_HOME:-}" ] || fail "no JDK 17 under /usr/lib/jvm, run the toolchain step first"
  export PATH="$JAVA_HOME/bin:$build_tools_dir:$ANDROID_HOME/cmdline-tools/latest/bin:$PATH"
}

toolchain() {
  # A hosted runner is not root. The JDK path below is Debian's openjdk-17 layout.
  local as_root=""
  if [ "$(id -u)" -ne 0 ]; then as_root="sudo"; command -v sudo >/dev/null || fail "the toolchain step installs packages and needs root or sudo"; fi
  if ! ls /usr/lib/jvm/java-17-openjdk-* >/dev/null 2>&1; then
    $as_root apt-get update
    $as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends openjdk-17-jdk-headless unzip
  fi
  if [ ! -x "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" ]; then
    local zip=/tmp/android-cmdline-tools.zip
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://dl.google.com/android/repository/commandlinetools-linux-${CMDLINE_TOOLS_REV}_latest.zip" -o "$zip"
    echo "${CMDLINE_TOOLS_SHA256}  $zip" | sha256sum -c -
    mkdir -p "$ANDROID_HOME/cmdline-tools"
    unzip -q "$zip" -d "$ANDROID_HOME/cmdline-tools"
    mv "$ANDROID_HOME/cmdline-tools/cmdline-tools" "$ANDROID_HOME/cmdline-tools/latest"
    rm "$zip"
  fi
  # dev-base carries node but no pnpm, and Gradle's Rust plugin execs a literal pnpm.
  corepack enable pnpm
  command -v pnpm >/dev/null || fail "pnpm is not on PATH after corepack enable"
  use_java
  yes | sdkmanager --licenses >/dev/null || true
  sdkmanager "platform-tools" "platforms;$ANDROID_PLATFORM" "build-tools;$BUILD_TOOLS" "ndk;$NDK_VERSION"
  rustup target add "$RUST_TARGET"
  if [ ! -f "$bundletool_jar" ]; then
    mkdir -p "$(dirname "$bundletool_jar")"
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/google/bundletool/releases/download/$BUNDLETOOL_VERSION/bundletool-all-$BUNDLETOOL_VERSION.jar" -o "$bundletool_jar.part"
    echo "${BUNDLETOOL_SHA256}  $bundletool_jar.part" | sha256sum -c -
    mv "$bundletool_jar.part" "$bundletool_jar"
  fi
  java -version
  echo "toolchain ready: platform $ANDROID_PLATFORM, build-tools $BUILD_TOOLS, ndk $NDK_VERSION, rust target $RUST_TARGET"
}

# Prints the unsigned release APK the Gradle build wrote, and fails on zero or many.
find_unsigned() {
  local found
  found=$(find "$android_dir/src-tauri/gen/android/app/build/outputs/apk" -name '*-release-unsigned.apk')
  [ "$(printf '%s\n' "$found" | grep -c .)" -eq 1 ] || fail "expected one unsigned release APK, found: ${found:-none}"
  printf '%s\n' "$found"
}

# version-code [EPOCH_SECONDS]: minutes since the Unix epoch. Why that scheme is in
# tooling-aterm-client references/android.md.
version_code() {
  echo $((${1:-$(date -u +%s)} / 60))
}

# resolve_code: the versionCode to build with, refused outside the range that installs
# over every earlier build and fits Android's cap.
resolve_code() {
  local code="${ATERM_ANDROID_VERSION_CODE:-$(version_code)}"
  [[ "$code" =~ ^[0-9]+$ ]] && [ "$code" -ge "$VERSION_CODE_FLOOR" ] && [ "$code" -le "$VERSION_CODE_MAX" ] \
    || fail "version code must be $VERSION_CODE_FLOOR to $VERSION_CODE_MAX, got $code"
  echo "$code"
}

# tauri_config VERSION CODE: the --config patch both the APK and the bundle build take.
tauri_config() {
  printf '{"version":"%s","bundle":{"android":{"versionCode":%s}}}' "$1" "$2"
}

# build VERSION OUT: the client bundle, then the unsigned APK at OUT. versionName is
# VERSION, versionCode is ATERM_ANDROID_VERSION_CODE or else the clock code.
build() {
  local version="${1:?usage: build VERSION OUT}" out="${2:?usage: build VERSION OUT}"
  local code
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "version must be major.minor.patch, got $version"
  code=$(resolve_code)
  echo "building $version with version code $code"
  use_java
  cd "$repo_root"
  just aterm-client-install
  VITE_ATERM_APP=1 just aterm-client-build
  just aterm-android-install
  cargo metadata --locked --format-version 1 --filter-platform "$RUST_TARGET" \
    --manifest-path "$android_dir/src-tauri/Cargo.toml" >/dev/null || fail "Cargo.lock is missing or stale, run just aterm-android-lock"
  rm -rf "$android_dir/src-tauri/gen/android"
  just aterm-android-tauri android init --ci
  just aterm-android-tauri android build --apk --target aarch64 --ci --config "$(tauri_config "$version" "$code")"
  if [ -e "$repo_root/.git" ] && ! git -C "$repo_root" diff --quiet -- aterm-android/src-tauri/Cargo.lock; then
    fail "the build changed Cargo.lock, commit the lock it resolved"
  fi
  cp "$(find_unsigned)" "$out"
  echo "unsigned APK: $out"
}

# Keystore and password come from ATERM_ANDROID_KEYSTORE_* env. Unset: a throwaway key,
# which proves the chain and cannot update a real build.
sign() {
  local in="${1:?usage: sign IN OUT}" out="${2:?usage: sign IN OUT}" ks work
  use_java
  work=$(mktemp -d)
  trap "rm -rf '$work'" EXIT
  ks="$work/aterm.p12"
  if [ -n "${ATERM_ANDROID_KEYSTORE_B64:-}" ]; then
    [ -n "${ATERM_ANDROID_KEYSTORE_PASSWORD:-}" ] || fail "ATERM_ANDROID_KEYSTORE_B64 is set without ATERM_ANDROID_KEYSTORE_PASSWORD"
    printf '%s' "$ATERM_ANDROID_KEYSTORE_B64" | base64 -d >"$ks"
  else
    echo "no signing keystore in the environment: signing with a throwaway key" >&2
    ATERM_ANDROID_KEYSTORE_PASSWORD="throwaway-$RANDOM$RANDOM"
    export ATERM_ANDROID_KEYSTORE_PASSWORD
    keytool -genkeypair -keystore "$ks" -storetype PKCS12 -alias "$KEY_ALIAS" -keyalg RSA -keysize 2048 \
      -validity 30 -dname "CN=aterm throwaway" -storepass:env ATERM_ANDROID_KEYSTORE_PASSWORD >/dev/null
  fi
  zipalign -c -P 16 4 "$in" || fail "the unsigned APK is not 16 KiB page aligned"
  apksigner sign --ks "$ks" --ks-key-alias "$KEY_ALIAS" --ks-pass env:ATERM_ANDROID_KEYSTORE_PASSWORD \
    --key-pass env:ATERM_ANDROID_KEYSTORE_PASSWORD --out "$out" "$in"
  rm -f "$out.idsig"
  if ! (verify "$out"); then
    rm -f "$out"
    fail "$out failed verification and was removed"
  fi
}

# verify APK: signature, manifest, and 16 KiB page alignment of the native library.
verify() {
  local apk="${1:?usage: verify APK}" cert want badging libs lib work
  use_java
  apksigner verify --verbose "$apk" | grep -E "^Verified using v(2|3) scheme .*: true" >/dev/null \
    || fail "$apk has no valid v2 or v3 signature"
  cert=$(apksigner verify --print-certs "$apk" | sed -n 's/^Signer #1 certificate SHA-256 digest: //p')
  [ -n "$cert" ] || fail "no signer certificate on $apk"
  want=$(printf '%s' "${ATERM_ANDROID_CERT_SHA256:-}" | tr 'A-F' 'a-f')
  if [ -n "$want" ] && [ "$cert" != "$want" ]; then
    fail "signer certificate $cert is not the expected $want, an update would not install over the last release"
  fi
  badging=$(aapt2 dump badging "$apk")
  printf '%s\n' "$badging" | grep -q "^package: name='$APP_ID' " || fail "package name is not $APP_ID"
  printf '%s\n' "$badging" | grep -q "^native-code: 'arm64-v8a'$" || fail "native code is not arm64-v8a only"
  printf '%s\n' "$badging" | grep -E "^(package:|sdkVersion:|targetSdkVersion:|application-label:)"
  work=$(mktemp -d)
  unzip -q -o "$apk" 'lib/arm64-v8a/*.so' -d "$work"
  libs=$(find "$work/lib/arm64-v8a" -name '*.so')
  [ -n "$libs" ] || fail "no native library in $apk"
  for lib in $libs; do
    for align in $("$ANDROID_HOME/ndk/$NDK_VERSION/toolchains/llvm/prebuilt/linux-x86_64/bin/llvm-readelf" -lW "$lib" \
      | awk '$1 == "LOAD" { print $NF }'); do
      [ $((align)) -ge 16384 ] || fail "$(basename "$lib") has a LOAD segment aligned at $align, below 16 KiB"
    done
  done
  rm -rf "$work"
  echo "signer SHA-256: $cert"
}

# build-debug ABI OUT: a debuggable APK for one emulator ABI (x86_64 or aarch64), signed
# by Gradle's debug key. Release builds have no WebView DevTools, see android.md.
build_debug() {
  local abi="${1:?usage: build-debug ABI OUT}" out="${2:?usage: build-debug ABI OUT}" rust_target native badging
  case "$abi" in
    x86_64) rust_target=x86_64-linux-android native=x86_64 ;;
    aarch64) rust_target=aarch64-linux-android native=arm64-v8a ;;
    *) fail "ABI must be x86_64 or aarch64, got $abi" ;;
  esac
  use_java
  cd "$repo_root"
  just aterm-client-install
  VITE_ATERM_APP=1 just aterm-client-build
  just aterm-android-install
  rustup target add "$rust_target"
  rm -rf "$android_dir/src-tauri/gen/android"
  just aterm-android-tauri android init --ci
  just aterm-android-tauri android build --debug --apk --target "$abi" --ci \
    --config "$(tauri_config 0.0.0 "$(resolve_code)")"
  local found
  found=$(find "$android_dir/src-tauri/gen/android/app/build/outputs/apk" -name '*-debug.apk')
  [ "$(printf '%s\n' "$found" | grep -c .)" -eq 1 ] || fail "expected one debug APK, found: ${found:-none}"
  badging=$(aapt2 dump badging "$found")
  printf '%s\n' "$badging" | grep -q "^package: name='$APP_ID' " || fail "package name is not $APP_ID"
  printf '%s\n' "$badging" | grep -q "^native-code: '$native'$" || fail "native code is not $native only"
  printf '%s\n' "$badging" | grep -q "^application-debuggable" || fail "the APK is not debuggable"
  cp "$found" "$out"
  echo "debug APK ($abi): $out"
}

bundletool() {
  java -jar "$bundletool_jar" "$@"
}

# bundle VERSION OUT: the unsigned release AAB at OUT, from the project build left behind.
# Run after build, so the version code is the same.
bundle() {
  local version="${1:?usage: bundle VERSION OUT}" out="${2:?usage: bundle VERSION OUT}" code found
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "version must be major.minor.patch, got $version"
  code=$(resolve_code)
  [ -d "$android_dir/src-tauri/gen/android" ] || fail "no Android project, run build first"
  use_java
  cd "$repo_root"
  just aterm-android-tauri android build --aab --target aarch64 --ci --config "$(tauri_config "$version" "$code")"
  found=$(find "$android_dir/src-tauri/gen/android/app/build/outputs/bundle" -name '*.aab')
  [ "$(printf '%s\n' "$found" | grep -c .)" -eq 1 ] || fail "expected one AAB, found: ${found:-none}"
  cp "$found" "$out"
  echo "unsigned AAB: $out"
}

# sign-bundle IN OUT: jarsigner with the Play upload key (ATERM_ANDROID_UPLOAD_*), or a
# throwaway key when unset. The upload key is never the APK key, see android.md.
sign_bundle() {
  local in="${1:?usage: sign-bundle IN OUT}" out="${2:?usage: sign-bundle IN OUT}" work ks
  use_java
  work=$(mktemp -d)
  trap "rm -rf '$work'" EXIT
  ks="$work/upload.p12"
  if [ -n "${ATERM_ANDROID_UPLOAD_KEYSTORE_B64:-}" ]; then
    [ -n "${ATERM_ANDROID_UPLOAD_KEYSTORE_PASSWORD:-}" ] || fail "ATERM_ANDROID_UPLOAD_KEYSTORE_B64 is set without ATERM_ANDROID_UPLOAD_KEYSTORE_PASSWORD"
    printf '%s' "$ATERM_ANDROID_UPLOAD_KEYSTORE_B64" | base64 -d >"$ks"
  else
    echo "no upload keystore in the environment: signing the bundle with a throwaway key, not Play-ready" >&2
    ATERM_ANDROID_UPLOAD_KEYSTORE_PASSWORD="throwaway-$RANDOM$RANDOM"
    export ATERM_ANDROID_UPLOAD_KEYSTORE_PASSWORD
    keytool -genkeypair -keystore "$ks" -storetype PKCS12 -alias "$UPLOAD_ALIAS" -keyalg RSA -keysize 2048 \
      -validity 30 -dname "CN=aterm throwaway upload" -storepass:env ATERM_ANDROID_UPLOAD_KEYSTORE_PASSWORD >/dev/null
  fi
  jarsigner -keystore "$ks" -storetype PKCS12 -storepass:env ATERM_ANDROID_UPLOAD_KEYSTORE_PASSWORD \
    -sigalg SHA256withRSA -digestalg SHA-256 -signedjar "$out" "$in" "$UPLOAD_ALIAS" >/dev/null
  if ! (verify_bundle "$out"); then
    rm -f "$out"
    fail "$out failed verification and was removed"
  fi
}

# verify-bundle AAB: bundletool validate, the signature, package, version code, and signer
# against ATERM_ANDROID_UPLOAD_CERT_SHA256. The signer must not be the APK certificate.
verify_bundle() {
  local aab="${1:?usage: verify-bundle AAB}" cert want apk_cert manifest
  use_java
  bundletool validate --bundle="$aab"
  jarsigner -verify "$aab" | grep -q "^jar verified" || fail "$aab has no valid signature"
  cert=$(keytool -printcert -jarfile "$aab" | sed -n 's/^[[:space:]]*SHA256: //p' | head -n1 | tr -d ':' | tr 'A-F' 'a-f')
  [ -n "$cert" ] || fail "no signer certificate on $aab"
  want=$(printf '%s' "${ATERM_ANDROID_UPLOAD_CERT_SHA256:-}" | tr 'A-F' 'a-f')
  if [ -n "$want" ] && [ "$cert" != "$want" ]; then
    fail "bundle signer $cert is not the expected upload certificate $want"
  fi
  apk_cert=$(printf '%s' "${ATERM_ANDROID_CERT_SHA256:-}" | tr 'A-F' 'a-f')
  if [ -n "$apk_cert" ] && [ "$cert" = "$apk_cert" ]; then
    fail "the bundle is signed with the APK key, the Play upload key must be a separate key"
  fi
  manifest=$(bundletool dump manifest --bundle="$aab")
  printf '%s\n' "$manifest" | grep -q "package=\"$APP_ID\"" || fail "bundle package is not $APP_ID"
  if [ -n "${ATERM_ANDROID_VERSION_CODE:-}" ]; then
    printf '%s\n' "$manifest" | grep -q "android:versionCode=\"$ATERM_ANDROID_VERSION_CODE\"" \
      || fail "bundle versionCode is not $ATERM_ANDROID_VERSION_CODE"
  fi
  printf '%s\n' "$manifest" | grep -E "package=|android:versionCode=|android:versionName=" | head -n3
  echo "upload signer SHA-256: $cert"
}

cmd="${1:-}"
[ $# -gt 0 ] && shift
case "$cmd" in
  toolchain | build | sign | verify | bundle) "$cmd" "$@" ;;
  build-debug) build_debug "$@" ;;
  version-code) version_code "$@" ;;
  sign-bundle) sign_bundle "$@" ;;
  verify-bundle) verify_bundle "$@" ;;
  app-id) echo "$APP_ID" ;;
  *) fail "usage: aterm-android.sh toolchain | build VERSION OUT | sign IN OUT | verify APK | bundle VERSION OUT | build-debug ABI OUT | sign-bundle IN OUT | verify-bundle AAB | version-code [EPOCH] | app-id" ;;
esac
