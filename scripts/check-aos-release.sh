#!/bin/sh
# Verify checksums, package metadata, and the native release binaries.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dist=${AOS_RELEASE_DIST:-"$repo_root/dist"}
version=${AOS_RELEASE_VERSION:?AOS_RELEASE_VERSION is required}
bare=${version#aos-v}

# The agent a role launches by default is configuration, owned by the launch
# profiles the release binaries are stamped from. Derive it, never restate it.
role_default_agent() {
    profiles="$repo_root/.agents/harness-launch-profiles.yaml"
    agent=$(awk -v role="$1" '
        $1 == role ":" { found = 1; next }
        found && $1 == "agent:" { print $2; exit }
        found && $0 ~ /^  [^ ]/ { exit }
    ' "$profiles")
    if [ -z "$agent" ]; then
        echo "$profiles declares no default agent for role $1" >&2
        exit 1
    fi
    printf '%s\n' "$agent"
}

(
    cd "$dist"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c SHA256SUMS
    else
        shasum -a 256 -c SHA256SUMS
    fi
)

target_count=$(awk '!/^[[:space:]]*(#|$)/ { count++ } END { print count+0 }' \
    "$repo_root/aos-cli/release-targets.txt")
target_count=$((target_count * 7))
checksum_count=$(wc -l < "$dist/SHA256SUMS" | tr -d ' ')
if [ "$target_count" -ne "$checksum_count" ]; then
    echo "checksum count does not match release target count" >&2
    exit 1
fi

for target in $(awk '!/^[[:space:]]*(#|$)/ { print }' \
    "$repo_root/aos-cli/release-targets.txt"); do
    goos=${target%%/*}
    goarch=${target#*/}
    bundle="$dist/aos-bundle-${goos}-${goarch}.tar.gz"
    tar -tzf "$bundle" | grep -Fx './share/aos/aosguard-skill/aosguard/SKILL.md' >/dev/null
    tar -tzf "$bundle" | grep -Fx './share/aos/aosguard-skill/aosguard/references/commands.yaml' >/dev/null
    tar -tzf "$bundle" | grep -Fx './share/aos/python/agentic_os/forgejo_actions_logs.py' >/dev/null
    tar -tzf "$bundle" | grep -Fx './share/aos/repositories/substrate-repos.txt' >/dev/null
done

python3 -m json.tool "$dist/aos.json" >/dev/null
if command -v ruby >/dev/null 2>&1; then
    ruby -c "$dist/aos.rb" >/dev/null
fi
grep -F "version \"${bare}\"" "$dist/aos.rb" >/dev/null
grep -F "\"version\": \"${bare}\"" "$dist/aos.json" >/dev/null
grep -F '"aoscompose"' "$dist/aos.rb" >/dev/null
grep -F '"aoscompose"' "$dist/aos.json" >/dev/null
grep -F "aoscomposed" "$dist/aos.rb" >/dev/null
grep -F "aoscomposed" "$dist/aos.json" >/dev/null
grep -F "aosward" "$dist/aos.rb" >/dev/null
grep -F "aosward" "$dist/aos.json" >/dev/null

case "$(uname -s)/$(uname -m)" in
    Darwin/arm64)
        native_aos="$dist/aos-darwin-arm64"
        native_aoscompose="$dist/aoscompose-darwin-arm64"
        native_aosward="$dist/aosward-darwin-arm64"
        native_aosguard="$dist/aosguard-darwin-arm64"
        native_agent_terminal="$dist/agent-terminal-darwin-arm64"
        native_aosterm="$dist/aosterm-darwin-arm64"
        ;;
    Linux/x86_64)
        native_aos="$dist/aos-linux-amd64"
        native_aoscompose="$dist/aoscompose-linux-amd64"
        native_aosward="$dist/aosward-linux-amd64"
        native_aosguard="$dist/aosguard-linux-amd64"
        native_agent_terminal="$dist/agent-terminal-linux-amd64"
        native_aosterm="$dist/aosterm-linux-amd64"
        ;;
    Linux/aarch64 | Linux/arm64)
        native_aos="$dist/aos-linux-arm64"
        native_aoscompose="$dist/aoscompose-linux-arm64"
        native_aosward="$dist/aosward-linux-arm64"
        native_aosguard="$dist/aosguard-linux-arm64"
        native_agent_terminal="$dist/agent-terminal-linux-arm64"
        native_aosterm="$dist/aosterm-linux-arm64"
        ;;
    *)
        native_aos=""
        native_aoscompose=""
        native_aosward=""
        native_aosguard=""
        native_agent_terminal=""
        native_aosterm=""
        ;;
esac
if [ -n "$native_aos" ]; then
    actual=$("$native_aos" version)
    if [ "$actual" != "$version" ]; then
        echo "native binary reports $actual, expected $version" >&2
        exit 1
    fi
    actual=$("$native_aoscompose" version)
    if [ "$actual" != "$version" ]; then
        echo "native aoscompose binary reports $actual, expected $version" >&2
        exit 1
    fi
    actual=$("$native_aosward" version)
    if [ "$actual" != "$version" ]; then
        echo "native aosward binary reports $actual, expected $version" >&2
        exit 1
    fi
    aos_plan=$("$native_aos" \
        --agent codex \
        --role platform \
        --image agentic-os:test \
        --auth=false \
        --dry-run \
        -- \
        --version)
    printf '%s\n' "$aos_plan" | grep -F -- "--composed" >/dev/null
    printf '%s\n' "$aos_plan" | grep -F -- "--guarded" >/dev/null
    aoscompose_plan=$("$native_aoscompose" \
        --agent codex \
        --role platform \
        --image agentic-os:test \
        --auth=false \
        --dry-run \
        -- \
        --version)
    printf '%s\n' "$aoscompose_plan" | grep -F -- "--composed" >/dev/null
    printf '%s\n' "$aoscompose_plan" | grep -F -- "--guarded" >/dev/null
    aosward_plan=$("$native_aosward" \
        --agent codex \
        --role tpm \
        --image agentic-os:test \
        --dry-run \
        -- \
        "aos release smoke")
    printf '%s\n' "$aosward_plan" | grep -F -- "--composed" >/dev/null
    printf '%s\n' "$aosward_plan" | grep -F -- "--guarded" >/dev/null
    printf '%s\n' "$aosward_plan" | grep -F "ward agent run --role tpm" >/dev/null
    smoke_dir=$(mktemp -d)
    trap 'rm -rf "$smoke_dir"' EXIT HUP INT TERM
    # The launch profiles own which agent a role defaults to, so read the
    # expected agent from them rather than restating it here.
    platform_agent=$(role_default_agent platform)
    tpm_agent=$(role_default_agent tpm)
    (
        cd "$smoke_dir"
        aoscompose_default_plan=$("$native_aoscompose" \
            --image agentic-os:test \
            --auth=false \
            --dry-run \
            platform)
        printf '%s\n' "$aoscompose_default_plan" | grep -F -- "--role platform" >/dev/null
        printf '%s\n' "$aoscompose_default_plan" | grep -F -- "--layout $platform_agent" >/dev/null
        printf '%s\n' "$aoscompose_default_plan" | grep -F -- "-- $platform_agent" >/dev/null
        aoscompose_tpm_plan=$("$native_aoscompose" \
            --image agentic-os:test \
            --auth=false \
            --dry-run \
            tpm)
        printf '%s\n' "$aoscompose_tpm_plan" | grep -F -- "--role tpm" >/dev/null
        printf '%s\n' "$aoscompose_tpm_plan" | grep -F -- "--layout $tpm_agent" >/dev/null
        printf '%s\n' "$aoscompose_tpm_plan" | grep -F -- "-- $tpm_agent" >/dev/null
        "$native_aosguard" --help >/dev/null
        "$native_aosguard" --version | grep -Fx "aosguard version $version" >/dev/null
        "$native_aosguard" ops aws --help >/dev/null
        "$native_aosguard" ops actions --help >/dev/null
        "$native_agent_terminal" --help >/dev/null
        "$native_agent_terminal" --version |
            grep -Fx "agent-terminal version $version" >/dev/null
        "$native_aosterm" --help >/dev/null
        "$native_aosterm" --version |
            grep -Fx "aosterm version $version" >/dev/null

        cp "$repo_root/agent-terminal/testdata/agent-compose" .
        cp "$repo_root/agent-terminal/testdata/tpm-overlay.json" .
        chmod 0755 agent-compose
        "$native_agent_terminal" \
            --role tpm \
            --seat codex \
            --expression acting \
            --task-title agentic-os-release-smoke \
            --working-directory "$smoke_dir" \
            --agent-compose-bin "$smoke_dir/agent-compose" \
            --aoscompose-bin "$native_aoscompose" \
            --dry-run \
            -- printf ready > launch.json
        "$native_aosterm" \
            --expression acting \
            --task-title agentic-os-release-smoke \
            --working-directory "$smoke_dir" \
            --agent-compose-bin "$smoke_dir/agent-compose" \
            --aoscompose-bin "$native_aoscompose" \
            --dry-run \
            tpm codex -- printf ready > aosterm-launch.json
        python3 - "$smoke_dir/launch.json" "$smoke_dir" "$native_aoscompose" <<'PY'
import json
import pathlib
import sys

plan = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
assert plan["format"] == "agent-terminal.launch.v1"
assert plan["working_directory"] == sys.argv[2]
assert plan["executable"] == "alacritty"
assert plan["arguments"][-6:] == ["-e", sys.argv[3], "tpm", "codex", "printf", "ready"]
PY
        python3 - "$smoke_dir/aosterm-launch.json" "$smoke_dir" "$native_aoscompose" <<'PY'
import json
import pathlib
import sys

plan = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
assert plan["format"] == "agent-terminal.launch.v1"
assert plan["working_directory"] == sys.argv[2]
assert plan["executable"] == "alacritty"
assert plan["arguments"][-6:] == ["-e", sys.argv[3], "tpm", "codex", "printf", "ready"]
PY
    )
fi

echo "verified aos, aoscompose, aosward, aosguard, agent-terminal, and aosterm release $version"
