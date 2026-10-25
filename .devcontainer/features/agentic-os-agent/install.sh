#!/usr/bin/env bash
# Agent surface for the fleet dev container. See docs/dev-container.md.
set -euo pipefail

feature_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
arch=$(dpkg --print-architecture)
git_name=coilyco-ops
git_email=coilyco-ops@coilysiren.me
export DEBIAN_FRONTEND=noninteractive
export PATH="/usr/local/share/nvm/current/bin:/usr/local/go/bin:/usr/local/bin:$PATH"

require() {
  command -v "$1" >/dev/null 2>&1 || { echo "agentic-os-agent needs $1 from an earlier feature" >&2; exit 1; }
}

fetch_listed() {
  local base=$1 file=$2
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$base/$file" -o "$work/$file"
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$base/SHA256SUMS" -o "$work/SHA256SUMS"
  (cd "$work" && grep "[[:space:]]${file}\$" SHA256SUMS | sha256sum -c -)
}

install_packages() {
  apt-get update -o Acquire::Retries=3
  apt-get install -y --no-install-recommends build-essential ca-certificates curl file git gnupg \
    jq less openssh-client patch procps python3 python3-pip python3-venv python3-yaml redis-tools \
    ripgrep shellcheck socat unzip wget xz-utils
  rm -rf /var/lib/apt/lists/*
}

install_aos_bundle() {
  local site
  fetch_listed "$AOSRELEASEBASE/aos-v$AOSVERSION" "aos-bundle-linux-$arch.tar.gz"
  mkdir -p "$work/bundle"
  tar -xzf "$work/aos-bundle-linux-$arch.tar.gz" -C "$work/bundle"
  install -m 0755 "$work/bundle/bin/aos" /usr/local/bin/aos
  install -m 0755 "$work/bundle/bin/aosguard" /usr/local/bin/aosguard
  install -d /opt/agentic-os
  cp -R "$work/bundle/share/aos/aosguard-skill" /opt/agentic-os/aosguard-skill
  install -m 0644 "$work/bundle/share/aos/repositories/substrate-repos.txt" /opt/agentic-os/substrate-repos.txt
  site=$(python3 -c 'import site; print(site.getsitepackages()[0])')
  mkdir -p "$site"
  cp -R "$work/bundle/share/aos/python/agentic_os" "$site/agentic_os"
  python3 -c 'import agentic_os'
  aos --version
  aosguard --version
}

build_shims() {
  local src="$work/bundle/share/aos/shim-src"
  [ -d "$src" ] || { echo "the aos $AOSVERSION bundle carries no shim-src" >&2; exit 1; }
  sh "$src/fs-shims/install.sh" /opt/agentic-os/shims
  GOCACHE="$work/gocache" GOPATH="$work/gopath" \
    umbra --project-root "$src/shims" build --set-version "$AOSVERSION" --out /opt/agentic-os/shims/gh
  chmod -R a+rX /opt/agentic-os/shims
}

install_agent_compose() {
  local asset="agent-compose-linux-$arch" data
  fetch_listed "$AGENTCOMPOSERELEASEBASE/v$AGENTCOMPOSEVERSION" "$asset"
  install -m 0755 "$work/$asset" /usr/local/bin/agent-compose
  ln -sf agent-compose /usr/local/bin/acompose
  install -d /usr/local/share/agent-compose
  for data in agent-compose-roster.tar.gz agent-compose-bundles.tar.gz; do
    fetch_listed "$AGENTCOMPOSERELEASEBASE/v$AGENTCOMPOSEVERSION" "$data"
    tar -xzf "$work/$data" -C /usr/local/share/agent-compose
  done
  agent-compose version
}

install_harnesses() {
  npm install -g "@anthropic-ai/claude-code@$CLAUDEVERSION"
  timeout 5m npm install -g "mcporter@$MCPORTERVERSION"
  npm_config_cache="$work/npm-cache" timeout 10m npm install -g "opencode-ai@$OPENCODEVERSION"
  claude --version
  timeout 1m mcporter --help >/dev/null
  opencode --version
}

install_identity_and_settings() {
  git config --system user.name "$git_name"
  git config --system user.email "$git_email"
  install -d /etc/claude-code /opt/agentic-os/statusline.d
  install -m 0644 "$feature_dir/files/claude-managed-settings.json" /etc/claude-code/managed-settings.json
  install -m 0755 "$feature_dir"/files/session-name.sh "$feature_dir"/files/git-identity.sh \
    "$feature_dir"/files/statusline.sh "$feature_dir"/files/shell-entrypoint.sh /opt/agentic-os/
  install -m 0755 "$feature_dir"/files/statusline.d/* /opt/agentic-os/statusline.d/
}

install_uv_pythons() {
  UV_PYTHON_INSTALL_DIR=/opt/uv/python uv python install 3.13 3.12
}

open_toolchain_dirs() {
  local dir
  for dir in /go /usr/local/cargo /usr/local/rustup /opt/uv; do
    if [ -d "$dir" ]; then find "$dir" -type d -exec chmod a+rwx {} +; fi
  done
}

verify_guards() {
  local scratch tool
  for tool in git gh rm find mv; do
    [ "$(PATH="/opt/agentic-os/shims:$PATH" command -v "$tool")" = "/opt/agentic-os/shims/$tool" ] \
      || { echo "$tool is not the guarded shim" >&2; exit 1; }
  done
  scratch=$(mktemp -d "${TMPDIR:-/tmp}/guard-check.XXXXXX")
  (
    cd "$scratch"
    export PATH="/opt/agentic-os/shims:$PATH"
    git init -q .
    git config user.email a@b.invalid
    git config user.name t
    echo x > f
    git add f
    if git commit --no-verify -q -m x 2>refusal; then echo "commit --no-verify was not refused" >&2; exit 1; fi
    grep -q denied refusal
    git commit -q -m ok
  )
  /bin/rm -rf "$scratch"
}

for needed in npm go umbra just uv codex goose; do require "$needed"; done
work=$(mktemp -d "${TMPDIR:-/tmp}/agent-surface.XXXXXX")
trap '/bin/rm -rf "$work"' EXIT
install_packages
install_aos_bundle
build_shims
install_agent_compose
install_harnesses
install_identity_and_settings
install_uv_pythons
open_toolchain_dirs
verify_guards
echo "agentic-os agent surface installed"
