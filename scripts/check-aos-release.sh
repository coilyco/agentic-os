#!/bin/sh
# Verify checksums, package metadata, and the native release binary.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dist=${AOS_RELEASE_DIST:-"$repo_root/dist"}
version=${AOS_RELEASE_VERSION:?AOS_RELEASE_VERSION is required}
bare=${version#aos-v}

(
    cd "$dist"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c SHA256SUMS
    else
        shasum -a 256 -c SHA256SUMS
    fi
)

target_count=$(awk '!/^[[:space:]]*(#|$)/ { count++ } END { print count+0 }' \
    "$repo_root/aos/release-targets.txt")
target_count=$((target_count * 2))
checksum_count=$(wc -l < "$dist/SHA256SUMS" | tr -d ' ')
if [ "$target_count" -ne "$checksum_count" ]; then
    echo "checksum count does not match release target count" >&2
    exit 1
fi

python3 -m json.tool "$dist/aos.json" >/dev/null
if command -v ruby >/dev/null 2>&1; then
    ruby -c "$dist/aos.rb" >/dev/null
fi
grep -F "version \"${bare}\"" "$dist/aos.rb" >/dev/null
grep -F "\"version\": \"${bare}\"" "$dist/aos.json" >/dev/null

case "$(uname -s)/$(uname -m)" in
    Darwin/arm64) native_aos="$dist/aos-darwin-arm64"; native_aguard="$dist/aguard-darwin-arm64" ;;
    Linux/x86_64) native_aos="$dist/aos-linux-amd64"; native_aguard="$dist/aguard-linux-amd64" ;;
    Linux/aarch64 | Linux/arm64) native_aos="$dist/aos-linux-arm64"; native_aguard="$dist/aguard-linux-arm64" ;;
    *) native_aos=""; native_aguard="" ;;
esac
if [ -n "$native_aos" ]; then
    actual=$("$native_aos" version)
    if [ "$actual" != "$version" ]; then
        echo "native binary reports $actual, expected $version" >&2
        exit 1
    fi
    smoke_dir=$(mktemp -d)
    trap 'rm -rf "$smoke_dir"' EXIT HUP INT TERM
    (
        cd "$smoke_dir"
        "$native_aguard" --help >/dev/null
        "$native_aguard" --version | grep -Fx "$version" >/dev/null
        "$native_aguard" ops aws --help >/dev/null
        "$native_aguard" ops actions runs --help >/dev/null
    )
fi

echo "verified aos and aguard release $version"
