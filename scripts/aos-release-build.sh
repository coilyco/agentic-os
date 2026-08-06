#!/bin/sh
# Cross-compile the version-stamped native AOS binaries from the target manifest.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
targets="$repo_root/aos-cli/release-targets.txt"
. "$repo_root/aos-cli/release.env"
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
launch_profiles="$repo_root/.agents/harness-launch-profiles.yaml"
if [ ! -s "$launch_profiles" ]; then
    echo "missing AOS launch profiles: $launch_profiles" >&2
    exit 1
fi
launch_profiles_b64=$(base64 < "$launch_profiles" | tr -d '\n')

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
    specgen_version=$SPECGEN_VERSION
    asset="specgen-${host_os}-${host_arch}"
    host_suffix=""
    if [ "$host_os" = "windows" ]; then
        host_suffix=".exe"
        asset="${asset}${host_suffix}"
    fi
    specgen="$release_build/specgen${host_suffix}"
    base="https://forgejo.coilysiren.me/coilyco-flight-deck/cli-guard/releases/download/v${specgen_version}"
    if [ -z "$specgen_version" ]; then
        echo "aos-cli/release.env does not pin specgen" >&2
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

build_aosguard_skill() {
    source="$release_build/aosguard-skill-source"
    host_binary="$release_build/aosguard-skill-host"
    cp -R "$repo_root/.specgen" "$source"
    "$specgen" \
        --project-root "$source/guardfiles" \
        --skills-out "$release_build/aosguard-skill" \
        build \
        --set-version "$version" \
        --out "$host_binary"
    test -s "$release_build/aosguard-skill/aosguard/SKILL.md"
    test -s "$release_build/aosguard-skill/aosguard/references/commands.yaml"
    rm -rf "$source" "$host_binary"
}

build_bundle() {
    target=$1
    goos=${target%%/*}
    goarch=${target#*/}
    suffix=""
    if [ "$goos" = "windows" ]; then
        suffix=".exe"
    fi

    bundle_root="$release_build/aos-bundle-${goos}-${goarch}"
    bundle="$dist/aos-bundle-${goos}-${goarch}.tar.gz"
    mkdir -p \
        "$bundle_root/bin" \
        "$bundle_root/share/aos/python/agentic_os" \
        "$bundle_root/share/aos/repositories"
    cp "$dist/aos-${goos}-${goarch}${suffix}" \
        "$bundle_root/bin/aos${suffix}"
    cp "$dist/aosguard-${goos}-${goarch}${suffix}" \
        "$bundle_root/bin/aosguard${suffix}"
    cp -R "$release_build/aosguard-skill" \
        "$bundle_root/share/aos/aosguard-skill"
    cp "$repo_root/agentic_os/__init__.py" \
        "$repo_root/agentic_os/forgejo_actions_list.py" \
        "$repo_root/agentic_os/forgejo_actions_logs.py" \
        "$repo_root/agentic_os/forgejo_actions_rerun.py" \
        "$repo_root/agentic_os/forgejo_actions_web.py" \
        "$bundle_root/share/aos/python/agentic_os/"
    cp "$repo_root/aos-cli/repositories/substrate-repos.txt" \
        "$repo_root/aos-cli/repositories/sealed-repos.gitignore" \
        "$bundle_root/share/aos/repositories/"
    printf '%s\n' "$version" > "$bundle_root/share/aos/version.txt"
    tar -czf "$bundle" -C "$bundle_root" .
    rm -rf "$bundle_root"
    echo "$bundle"
}

build_aosguard() {
    target=$1
    goos=${target%%/*}
    goarch=${target#*/}
    source="$release_build/aosguard-source-${goos}-${goarch}"
    raw="$release_build/aosguard"
    wrapper="$release_build/aosguard-release"
    out="$dist/aosguard-${goos}-${goarch}"
    if [ "$goos" = "windows" ]; then
        raw="${raw}.exe"
        out="${out}.exe"
    fi

    cp -R "$repo_root/.specgen" "$source"
    project="$source/guardfiles"
    python3 - "$project/specverb.lock" "$project/go.mod" "$project/go.sum" <<'PY'
import json
import pathlib
import sys

lock = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
pathlib.Path(sys.argv[2]).write_text("\n".join(lock["goMod"]) + "\n", encoding="utf-8")
pathlib.Path(sys.argv[3]).write_text("\n".join(lock["goSum"]) + "\n", encoding="utf-8")
PY
    "$specgen" --project-root "$project" gen --out "$project/main.go"
    # Specgen materialization decodes gzip locks before embedding. After `gen`,
    # this direct cross-build stages every decoded lock.
    find "$project" -type f -name '*.lock.json.gz' -print |
        while IFS= read -r encoded_lock; do
            decoded_lock=${encoded_lock%.gz}
            gzip -dc "$encoded_lock" > "$decoded_lock"
            mv "$decoded_lock" "$encoded_lock"
        done
    (
        cd "$project"
        GOPROXY=direct GOSUMDB=off GOPRIVATE=forgejo.coilysiren.me \
            GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -trimpath -ldflags "-s -w -X main.Version=${version}" -o "$raw" .
    )
    cp -R "$repo_root/aosguard-release" "$wrapper"
    mkdir -p "$wrapper/payload/agentic_os"
    cp "$raw" "$wrapper/payload/aosguard"
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

build_agent_terminal() {
    target=$1
    goos=${target%%/*}
    goarch=${target#*/}
    suffix=""
    out="$dist/agent-terminal-${goos}-${goarch}"
    alias_out="$dist/aosterm-${goos}-${goarch}"
    if [ "$goos" = "windows" ]; then
        suffix=".exe"
        out="${out}${suffix}"
        alias_out="${alias_out}${suffix}"
    fi
    if [ -e "$out" ] || [ -e "$alias_out" ]; then
        echo "duplicate release output: $out or $alias_out" >&2
        exit 1
    fi
    (
        cd "$repo_root/agent-terminal"
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -trimpath \
            -ldflags "-s -w -X main.version=${version} -X main.compiledHarnessLaunchProfilesBase64=${launch_profiles_b64}" \
            -o "$out" .
    )
    cp "$out" "$alias_out"
    echo "$out"
    echo "$alias_out"
}

mkdir -p "$dist"
release_build=$(mktemp -d)
trap 'rm -rf "$release_build"' EXIT HUP INT TERM
download_specgen
build_aosguard_skill

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
    compose_out="$dist/aoscompose-${goos}-${goarch}"
    ward_out="$dist/aosward-${goos}-${goarch}"
    if [ "$goos" = "windows" ]; then
        out="${out}.exe"
        compose_out="${compose_out}.exe"
        ward_out="${ward_out}.exe"
    fi
    if [ -e "$out" ] || [ -e "$compose_out" ] || [ -e "$ward_out" ]; then
        echo "duplicate release output: $out, $compose_out, or $ward_out" >&2
        exit 1
    fi
    (
        cd "$repo_root/aos-cli"
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -trimpath \
            -ldflags "-s -w -X main.version=${version} -X main.compiledHarnessLaunchProfilesBase64=${launch_profiles_b64}" \
            -o "$out" .
    )
    cp "$out" "$compose_out"
    cp "$out" "$ward_out"
    build_agent_terminal "$target"
    build_aosguard "$target"
    build_bundle "$target"
    echo "$out"
    echo "$compose_out"
    echo "$ward_out"
done < "$targets"

(
    cd "$dist"
    for asset in agent-terminal-* aos-* aoscompose-* aosguard-* aosward-* aosterm-*; do
        checksum "$asset"
    done > SHA256SUMS
)
echo "$dist/SHA256SUMS"
echo "version: $version"
