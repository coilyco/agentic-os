#!/usr/bin/env bash
# Install the pinned agent-compose release from fetched assets, for the
# guards that read the roster. The release is private. See docs/dev-container.md.

set -euo pipefail

version=2.220.0
binary_sha256=b084826ba3a1fe27b342b66b01f68ef49b4977902f21a9164c74ff30542c670f
roster_sha256=f9c7bc0f593939b81829e42e407f2f2d54f1a617ab10770ebd95fc993d4d55d2
tag=agent-compose-v${version}
if [ "${1:-}" = tag ]; then
  echo "$tag"
  exit 0
fi
base=${AGENT_COMPOSE_BASE:?set AGENT_COMPOSE_BASE to the directory fetch-release wrote, see docs/dev-container.md}
dest=${AGENT_COMPOSE_DEST:-${RUNNER_TEMP:?RUNNER_TEMP is not set}/agent-compose}

case "$(uname -m)" in
  x86_64 | amd64) ;;
  *)
    echo "the pinned agent-compose is linux-amd64 only, got $(uname -m)" >&2
    exit 1
    ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fetch() {
  cp "${base#file://}/$1" "$work/$1"
  echo "$2  $work/$1" | sha256sum -c - >/dev/null || {
    echo "$1 does not match the pinned checksum" >&2
    exit 1
  }
}

fetch agent-compose-linux-amd64 "$binary_sha256"
fetch agent-compose-roster.tar.gz "$roster_sha256"

# The binary finds its roster at ../share/agent-compose/roster from its own directory.
mkdir -p "$dest/bin" "$dest/share/agent-compose"
install -m 0755 "$work/agent-compose-linux-amd64" "$dest/bin/agent-compose"
tar -xzf "$work/agent-compose-roster.tar.gz" -C "$dest/share/agent-compose"
echo "$dest/bin" >> "${GITHUB_PATH:-/dev/null}"
"$dest/bin/agent-compose" version
