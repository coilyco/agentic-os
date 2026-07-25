#!/bin/sh
# Cross-compile version-stamped aos and aguard binaries from the target manifest.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
targets="$repo_root/aos/release-targets.txt"
version=${AOS_RELEASE_VERSION:-$(
    git -C "$repo_root" describe --tags --exact-match --match 'aos-v*' 2>/dev/null ||
        git -C "$repo_root" rev-parse --short HEAD
)}
version=$(printf '%s' "$version" | tr -d '\r')
if [ -n "${AOS_RELEASE_DIST:-}" ]; then
    dist=$AOS_RELEASE_DIST
else
    dist="$repo_root/dist"
    rm -rf "$dist"
fi

checksum() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1"
    else
        shasum -a 256 "$1"
    fi
}

verify_checksum() {
    directory=$1
    if command -v sha256sum >/dev/null 2>&1; then
        (cd "$directory" && sha256sum -c -)
    else
        (cd "$directory" && shasum -a 256 -c -)
    fi
}

download_specgen() {
    host_os=$(go env GOOS | tr -d '\r')
    host_arch=$(go env GOARCH | tr -d '\r')
    specgen_version=$(
        sed -n 's/^ARG SPECGEN_VERSION=//p' \
            "$repo_root/docker/dev-base/Dockerfile" | tr -d '\r'
    )
    asset="specgen-${host_os}-${host_arch}"
    host_suffix=""
    if [ "$host_os" = "windows" ]; then
        host_suffix=".exe"
        asset="${asset}${host_suffix}"
    fi
    specgen="$release_build/specgen${host_suffix}"
    base="https://forgejo.coilysiren.me/coilyco-flight-deck/cli-guard/releases/download/v${specgen_version}"
    if [ -z "$specgen_version" ]; then
        echo "Dockerfile does not pin specgen" >&2
        exit 1
    fi
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
        "${base}/${asset}" -o "$release_build/$asset"
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
        "${base}/SHA256SUMS" -o "$release_build/SHA256SUMS"
    grep "[[:space:]]${asset}$" "$release_build/SHA256SUMS" \
        | verify_checksum "$release_build"
    mv "$release_build/$asset" "$specgen"
    chmod 0755 "$specgen"
}

build_aguard() {
    target=$1
    goos=${target%%/*}
    goarch=${target#*/}
    source="$release_build/aguard-source-${goos}-${goarch}"
    raw="$release_build/aguard"
    wrapper="$release_build/aguard-release"
    out="$dist/aguard-${goos}-${goarch}"
    if [ "$goos" = "windows" ]; then
        raw="${raw}.exe"
        out="${out}.exe"
    fi

    cp -R "$repo_root/.specgen" "$source"
    gzip -dc "$source/aguard/forgejo.swagger.lock.json.gz" \
        > "$source/aguard/forgejo.swagger.lock.json"
    mv "$source/aguard/forgejo.swagger.lock.json" \
        "$source/aguard/forgejo.swagger.lock.json.gz"
    python3 - "$source/specverb.lock" "$source/go.mod" "$source/go.sum" <<'PY'
import json
import pathlib
import sys

lock = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
pathlib.Path(sys.argv[2]).write_text("\n".join(lock["goMod"]) + "\n", encoding="utf-8")
pathlib.Path(sys.argv[3]).write_text("\n".join(lock["goSum"]) + "\n", encoding="utf-8")
PY
    "$specgen" --project-root "$source" gen --out "$source/main.go"
    (
        cd "$source"
        GOPROXY=direct GOSUMDB=off GOPRIVATE=forgejo.coilysiren.me \
            GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -trimpath -ldflags "-s -w -X main.Version=${version}" -o "$raw" .
    )
    cp -R "$repo_root/aguard-release" "$wrapper"
    mkdir -p "$wrapper/payload/agentic_os"
    cp "$raw" "$wrapper/payload/aguard"
    cp "$repo_root/agentic_os/__init__.py" \
        "$repo_root/agentic_os/forgejo_actions_list.py" \
        "$repo_root/agentic_os/forgejo_actions_logs.py" \
        "$repo_root/agentic_os/forgejo_actions_rerun.py" \
        "$repo_root/agentic_os/forgejo_actions_web.py" \
        "$wrapper/payload/agentic_os/"
    (
        cd "$wrapper"
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -trimpath -ldflags "-s -w" -o "$out" .
    )
    rm -rf "$source" "$wrapper" "$raw"
    echo "$out"
}

mkdir -p "$dist"
release_build=$(mktemp -d)
trap 'rm -rf "$release_build"' EXIT HUP INT TERM
download_specgen

while IFS= read -r target || [ -n "$target" ]; do
    target=$(printf '%s' "$target" | tr -d '\r')
    case "$target" in
        ""|\#*) continue ;;
    esac
    goos=${target%%/*}
    goarch=${target#*/}
    if [ "$goos" = "$target" ] || [ "$goarch" = "$target" ] ||
        [ -z "$goos" ] || [ -z "$goarch" ]; then
        echo "invalid release target: $target" >&2
        exit 1
    fi
    case "$goos/$goarch" in
        *[!a-z0-9/_-]*|*/*/*)
            echo "unsafe release target: $target" >&2
            exit 1
            ;;
    esac
    out="$dist/aos-${goos}-${goarch}"
    if [ "$goos" = "windows" ]; then
        out="${out}.exe"
    fi
    if [ -e "$out" ]; then
        echo "duplicate release output: $out" >&2
        exit 1
    fi
    (
        cd "$repo_root/aos"
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -trimpath -ldflags "-s -w -X main.version=${version}" \
            -o "$out" .
    )
    build_aguard "$target"
    echo "$out"
done < "$targets"

(
    cd "$dist"
    for asset in aos-* aguard-*; do
        checksum "$asset"
    done > SHA256SUMS
)
echo "$dist/SHA256SUMS"
echo "version: $version"
