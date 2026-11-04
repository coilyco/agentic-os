#!/bin/sh
# Wrap the darwin aterm binary in a signed app bundle and archive it (COI-50).
# Usage: aterm-sign-darwin.sh <aterm-darwin-arm64> <out.app.tar.gz>
#
# A bundle is keyed by identifier and a certificate signature pins no build hash,
# so both are needed for grants to survive an upgrade. See docs/aos-cli.md.
set -eu

binary=${1:?usage: aterm-sign-darwin.sh <aterm-darwin-arm64> <out.app.tar.gz>}
archive=${2:?usage: aterm-sign-darwin.sh <aterm-darwin-arm64> <out.app.tar.gz>}
version=${AOS_RELEASE_VERSION:-}

# The first signed release already uses the Treehouse identifier, so Kai approves once.
# Rotating the certificate expires every grant, hence the 250-year lifetime.
identifier=dev.coilyco.treehouse
app_name=Treehouse.app
executable=aterm

bare=${version#aos-v}
case "$bare" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) bare=0.0.0 ;;
esac

# The gate fails closed: an unsigned bundle ships looking fine and re-prompts
# on every release, which is the failure this file exists to end.
unsigned=${AOS_RELEASE_UNSIGNED:-}
if [ -z "$unsigned" ] && { [ -z "${TREEHOUSE_CODESIGN_CERT_PEM:-}" ] || [ -z "${TREEHOUSE_CODESIGN_KEY_PEM:-}" ]; }; then
    echo "TREEHOUSE_CODESIGN_CERT_PEM and TREEHOUSE_CODESIGN_KEY_PEM are required." >&2
    echo "Set AOS_RELEASE_UNSIGNED=1 for a local build that is not for release." >&2
    exit 1
fi
if ! command -v rcodesign >/dev/null 2>&1; then
    echo "rcodesign is not on PATH (scripts/ci/install-rcodesign.sh installs the pinned one)" >&2
    exit 1
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

app="$work/stage/$app_name"
mkdir -p "$app/Contents/MacOS"
cp "$binary" "$app/Contents/MacOS/$executable"
chmod 0755 "$app/Contents/MacOS/$executable"
# LSUIElement keeps a LaunchServices open of this bundle out of the Dock.
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key><string>en</string>
    <key>CFBundleExecutable</key><string>${executable}</string>
    <key>CFBundleIdentifier</key><string>${identifier}</string>
    <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
    <key>CFBundleName</key><string>Treehouse</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleShortVersionString</key><string>${bare}</string>
    <key>CFBundleVersion</key><string>${bare}</string>
    <key>LSUIElement</key><true/>
</dict>
</plist>
EOF

# Secrets ride mode-600 files removed on exit, never argv. --timestamp-url none
# skips Apple's timestamp server, which a self-signed certificate gains nothing from.
signed="$work/signed/$app_name"
if [ -n "$unsigned" ]; then
    rcodesign sign --timestamp-url none "$app" "$signed"
else
    # A subshell, because a lasting umask would write the bundle itself mode 700.
    (
        umask 077
        printf '%s\n' "$TREEHOUSE_CODESIGN_CERT_PEM" > "$work/cert.pem"
        printf '%s\n' "$TREEHOUSE_CODESIGN_KEY_PEM" > "$work/key.pem"
    )
    rcodesign sign --timestamp-url none \
        --pem-file "$work/cert.pem" --pem-file "$work/key.pem" \
        "$app" "$signed"
    if command -v openssl >/dev/null 2>&1; then
        # Public by nature, and the value a person compares with
        # `codesign -d -r-` on the Mac to confirm the same identity signed.
        openssl x509 -in "$work/cert.pem" -noout -fingerprint -sha1 | sed 's/^/signing certificate /'
    fi
fi

# rcodesign exits 0 for a bundle whose executable it did not find and writes an
# unsigned copy, so the output is read back instead of trusting the exit code.
info=$(rcodesign print-signature-info "$signed")
printf '%s\n' "$info" | grep -Fq "identifier: $identifier" || {
    echo "signed bundle does not carry identifier $identifier" >&2
    exit 1
}
if [ -z "$unsigned" ]; then
    requirement=$(printf '%s\n' "$info" | grep -F 'designated(3)' | head -1)
    case "$requirement" in
        *"(identifier \"$identifier\")"*"certificate root = H\""* | *"(identifier \"$identifier\")"*"certificate leaf = H\""*) ;;
        *)
            echo "designated requirement is not identifier plus certificate: ${requirement:-none}" >&2
            exit 1
            ;;
    esac
    case "$requirement" in
        *cdhash*)
            echo "designated requirement pins a build hash: $requirement" >&2
            exit 1
            ;;
    esac
fi

mkdir -p "$(dirname "$archive")"
tar -czf "$archive" -C "$work/signed" "$app_name"
echo "$archive"
