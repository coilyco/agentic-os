#!/bin/sh
# Verify checksums, package metadata, and the native release binaries.
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
target_count=$((target_count * 5))
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
grep -F "aoscomposed" "$dist/aos.rb" >/dev/null
grep -F "aoscomposed" "$dist/aos.json" >/dev/null
grep -F "aosward" "$dist/aos.rb" >/dev/null
grep -F "aosward" "$dist/aos.json" >/dev/null

case "$(uname -s)/$(uname -m)" in
    Darwin/arm64)
        native_aos="$dist/aos-darwin-arm64"
        native_aoscomposed="$dist/aoscomposed-darwin-arm64"
        native_aosward="$dist/aosward-darwin-arm64"
        native_aosguard="$dist/aosguard-darwin-arm64"
        native_agent_terminal="$dist/agent-terminal-darwin-arm64"
        ;;
    Linux/x86_64)
        native_aos="$dist/aos-linux-amd64"
        native_aoscomposed="$dist/aoscomposed-linux-amd64"
        native_aosward="$dist/aosward-linux-amd64"
        native_aosguard="$dist/aosguard-linux-amd64"
        native_agent_terminal="$dist/agent-terminal-linux-amd64"
        ;;
    Linux/aarch64 | Linux/arm64)
        native_aos="$dist/aos-linux-arm64"
        native_aoscomposed="$dist/aoscomposed-linux-arm64"
        native_aosward="$dist/aosward-linux-arm64"
        native_aosguard="$dist/aosguard-linux-arm64"
        native_agent_terminal="$dist/agent-terminal-linux-arm64"
        ;;
    *)
        native_aos=""
        native_aoscomposed=""
        native_aosward=""
        native_aosguard=""
        native_agent_terminal=""
        ;;
esac
if [ -n "$native_aos" ]; then
    actual=$("$native_aos" version)
    if [ "$actual" != "$version" ]; then
        echo "native binary reports $actual, expected $version" >&2
        exit 1
    fi
    actual=$("$native_aoscomposed" version)
    if [ "$actual" != "$version" ]; then
        echo "native aoscomposed binary reports $actual, expected $version" >&2
        exit 1
    fi
    actual=$("$native_aosward" version)
    if [ "$actual" != "$version" ]; then
        echo "native aosward binary reports $actual, expected $version" >&2
        exit 1
    fi
    aos_plan=$("$native_aos" \
        --agent codex \
        --role engineer \
        --image agentic-os:test \
        --dry-run \
        -- \
        --version)
    printf '%s\n' "$aos_plan" | grep -F -- "--composed" >/dev/null
    printf '%s\n' "$aos_plan" | grep -F -- "--guarded" >/dev/null
    aoscomposed_plan=$("$native_aoscomposed" \
        --agent codex \
        --role engineer \
        --image agentic-os:test \
        --dry-run \
        -- \
        --version)
    printf '%s\n' "$aoscomposed_plan" | grep -F -- "--composed" >/dev/null
    printf '%s\n' "$aoscomposed_plan" | grep -F -- "--guarded" >/dev/null
    aosward_plan=$("$native_aosward" \
        --agent codex \
        --role director \
        --image agentic-os:test \
        --dry-run)
    printf '%s\n' "$aosward_plan" | grep -F -- "--composed" >/dev/null
    printf '%s\n' "$aosward_plan" | grep -F -- "--guarded" >/dev/null
    printf '%s\n' "$aosward_plan" | grep -F "ward agent director" >/dev/null
    smoke_dir=$(mktemp -d)
    trap 'rm -rf "$smoke_dir"' EXIT HUP INT TERM
    (
        cd "$smoke_dir"
        "$native_aosguard" --help >/dev/null
        "$native_aosguard" --version | grep -Fx "aosguard version $version" >/dev/null
        "$native_aosguard" ops aws --help >/dev/null
        "$native_aosguard" ops actions --help >/dev/null
        "$native_agent_terminal" --help >/dev/null
        "$native_agent_terminal" --version |
            grep -Fx "agent-terminal version $version" >/dev/null

        cp "$repo_root/agent-terminal/testdata/agent-compose" .
        cp "$repo_root/agent-terminal/testdata/director-overlay.json" .
        chmod 0755 agent-compose
        "$native_agent_terminal" \
            --role director \
            --seat codex \
            --expression acting \
            --task-title agentic-os-release-smoke \
            --working-directory "$smoke_dir" \
            --agent-compose-bin "$smoke_dir/agent-compose" \
            --dry-run \
            -- printf ready > launch.json
        python3 - "$smoke_dir/launch.json" "$smoke_dir" <<'PY'
import json
import pathlib
import sys

plan = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
assert plan["format"] == "agent-terminal.launch.v1"
assert plan["working_directory"] == sys.argv[2]
assert plan["executable"] == "alacritty"
assert plan["arguments"][-3:] == ["-e", "printf", "ready"]
PY
    )
fi

echo "verified aos, aoscomposed, aosward, aosguard, and agent-terminal release $version"
