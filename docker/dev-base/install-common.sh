#!/usr/bin/env bash

set -euo pipefail
set -x

substrate_repos=${1:?substrate repository list is required}
git_lfs_version=${2:?Git LFS version is required}
git_lfs_sha256_amd64=${3:?Git LFS amd64 SHA-256 is required}
git_lfs_sha256_arm64=${4:?Git LFS arm64 SHA-256 is required}
export DEBIAN_FRONTEND=noninteractive

case "${TARGETARCH:?TARGETARCH is required}" in
  amd64)
    AWS_ARCH=x86_64
    CODEX_ARCH=x86_64
    DOCKER_ARCH=x86_64
    DOTNET_ARCH=x64
    GH_ARCH=amd64
    GO_ARCH=amd64
    GOLANGCI_ARCH=amd64
    GOOSE_ARCH=x86_64
    HELM_ARCH=amd64
    KDL_ARCH=x86_64
    KUBECTL_ARCH=amd64
    NODE_ARCH=x64
    TRUFFLEHOG_ARCH=amd64
    TS_ARCH=amd64
    YQ_ARCH=amd64
    ;;
  arm64)
    AWS_ARCH=aarch64
    CODEX_ARCH=aarch64
    DOCKER_ARCH=aarch64
    DOTNET_ARCH=arm64
    GH_ARCH=arm64
    GO_ARCH=arm64
    GOLANGCI_ARCH=arm64
    GOOSE_ARCH=aarch64
    HELM_ARCH=arm64
    KDL_ARCH=aarch64
    KUBECTL_ARCH=arm64
    NODE_ARCH=arm64
    TRUFFLEHOG_ARCH=arm64
    TS_ARCH=arm64
    YQ_ARCH=arm64
    ;;
  *)
    echo "unsupported TARGETARCH: ${TARGETARCH}" >&2
    exit 1
    ;;
esac
export AWS_ARCH CODEX_ARCH DOCKER_ARCH DOTNET_ARCH GH_ARCH GO_ARCH GOLANGCI_ARCH
export GOOSE_ARCH HELM_ARCH KDL_ARCH KUBECTL_ARCH NODE_ARCH
export TRUFFLEHOG_ARCH TS_ARCH YQ_ARCH
install -d /opt/agentic-os
printf '%s\n' \
  "AWS_ARCH=${AWS_ARCH}" \
  "CODEX_ARCH=${CODEX_ARCH}" \
  "DOCKER_ARCH=${DOCKER_ARCH}" \
  "DOTNET_ARCH=${DOTNET_ARCH}" \
  "GH_ARCH=${GH_ARCH}" \
  "GO_ARCH=${GO_ARCH}" \
  "GOLANGCI_ARCH=${GOLANGCI_ARCH}" \
  "GOOSE_ARCH=${GOOSE_ARCH}" \
  "HELM_ARCH=${HELM_ARCH}" \
  "KDL_ARCH=${KDL_ARCH}" \
  "KUBECTL_ARCH=${KUBECTL_ARCH}" \
  "NODE_ARCH=${NODE_ARCH}" \
  "TRUFFLEHOG_ARCH=${TRUFFLEHOG_ARCH}" \
  "TS_ARCH=${TS_ARCH}" \
  "YQ_ARCH=${YQ_ARCH}" \
  > /opt/agentic-os/arch.env

for attempt in 1 2 3; do
  if apt-get update -o Acquire::Retries=3 \
    && apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
      curl \
      ffmpeg \
      file \
      git \
      gnupg \
      imagemagick \
      jq \
      less \
      libicu74 \
      openssh-client \
      patch \
      procps \
      python3 \
      python3-pip \
      python3-venv \
      ripgrep \
      shellcheck \
      socat \
      unzip \
      wget \
      xz-utils; then
    rm -rf /var/lib/apt/lists/*
    break
  fi
  if [ "$attempt" -eq 3 ]; then
    exit 1
  fi
  sleep 5
done
convert -version
ffmpeg -version
ffprobe -version

git_lfs_asset="git-lfs-linux-${GH_ARCH}-v${git_lfs_version}.tar.gz"
git_lfs_base="https://github.com/git-lfs/git-lfs/releases/download/v${git_lfs_version}"
case "$TARGETARCH" in
  amd64) git_lfs_sha256=$git_lfs_sha256_amd64 ;;
  arm64) git_lfs_sha256=$git_lfs_sha256_arm64 ;;
esac
curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "${git_lfs_base}/${git_lfs_asset}" -o "/tmp/${git_lfs_asset}"
printf '%s  %s\n' "$git_lfs_sha256" "$git_lfs_asset" \
  | (cd /tmp && sha256sum -c -)
install -d /tmp/git-lfs
tar -xzf "/tmp/${git_lfs_asset}" -C /tmp/git-lfs --strip-components=1
install -m 0755 /tmp/git-lfs/git-lfs /usr/local/bin/git-lfs
rm -rf /tmp/git-lfs "/tmp/${git_lfs_asset}"
git lfs install --system --skip-repo
git lfs version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://astral.sh/uv/${UV_VERSION:?}/install.sh" \
  | env INSTALLER_NO_MODIFY_PATH=1 UV_INSTALL_DIR=/usr/local/bin sh
uv --version
uv tool install pre-commit
chmod -R a+rX /opt/uv
pre-commit --version
uv python install 3.13 3.12
chmod -R a+rwX "${UV_PYTHON_INSTALL_DIR:?}"
uv python list

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://nodejs.org/dist/v${NODE_VERSION:?}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
  -o /tmp/node.tar.xz
install -d /usr/local/node
tar -xJf /tmp/node.tar.xz -C /usr/local/node --strip-components=1 \
  --exclude='*/CHANGELOG.md' \
  --exclude='*/README.md' \
  --exclude='*/LICENSE'
rm /tmp/node.tar.xz
node --version
npm --version

npm install -g "@anthropic-ai/claude-code@${CLAUDE_VERSION:?}"
timeout 5m npm install -g "mcporter@${MCPORTER_VERSION:?}"
npm_config_cache=/tmp/npm-cache \
  timeout 10m npm install -g "opencode-ai@${OPENCODE_VERSION:?}"
rm -rf /tmp/npm-cache
claude --version
timeout 1m mcporter --help >/dev/null
opencode --version
rm -rf /root/.npm

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION:?}/codex-${CODEX_ARCH}-unknown-linux-musl.tar.gz" \
  -o /tmp/codex.tar.gz
tar -xzf /tmp/codex.tar.gz -C /usr/local/bin
mv "/usr/local/bin/codex-${CODEX_ARCH}-unknown-linux-musl" /usr/local/bin/codex
chmod 0755 /usr/local/bin/codex
rm /tmp/codex.tar.gz
codex --version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://github.com/block/goose/releases/download/v${GOOSE_VERSION:?}/goose-${GOOSE_ARCH}-unknown-linux-musl.tar.gz" \
  -o /tmp/goose.tar.gz
tar -xzf /tmp/goose.tar.gz -C /usr/local/bin ./goose
chmod 0755 /usr/local/bin/goose
rm /tmp/goose.tar.gz
goose --version

agent_compose_asset="agent-compose-linux-${GO_ARCH}"
agent_compose_base="https://forgejo.coilysiren.me/coilyco-flight-deck/agent-compose/releases/download/v${AGENT_COMPOSE_VERSION:?}"
curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "${agent_compose_base}/${agent_compose_asset}" \
  -o "/tmp/${agent_compose_asset}"
curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "${agent_compose_base}/SHA256SUMS" \
  -o /tmp/agent-compose-SHA256SUMS
grep "[[:space:]]${agent_compose_asset}$" /tmp/agent-compose-SHA256SUMS \
  | (cd /tmp && sha256sum -c -)
install -m 0755 "/tmp/${agent_compose_asset}" /usr/local/bin/agent-compose
ln -s agent-compose /usr/local/bin/acompose
rm "/tmp/${agent_compose_asset}" /tmp/agent-compose-SHA256SUMS
agent-compose version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://awscli.amazonaws.com/awscli-exe-linux-${AWS_ARCH}-${AWSCLI_VERSION:?}.zip" \
  -o /tmp/awscli.zip
unzip -q /tmp/awscli.zip -d /tmp
/tmp/aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli
rm -rf /tmp/awscli.zip /tmp/aws
aws --version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://github.com/cli/cli/releases/download/v${GH_VERSION:?}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" \
  -o /tmp/gh.tar.gz
tar -xzf /tmp/gh.tar.gz -C /usr/local/bin --strip-components=2 \
  "gh_${GH_VERSION}_linux_${GH_ARCH}/bin/gh"
chmod 0755 /usr/local/bin/gh
rm /tmp/gh.tar.gz
gh --version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://get.helm.sh/helm-v${HELM_VERSION:?}-linux-${HELM_ARCH}.tar.gz" \
  -o /tmp/helm.tar.gz
tar -xzf /tmp/helm.tar.gz -C /usr/local/bin --strip-components=1 \
  "linux-${HELM_ARCH}/helm"
chmod 0755 /usr/local/bin/helm
rm /tmp/helm.tar.gz
helm version --short

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://dl.k8s.io/release/v${KUBECTL_VERSION:?}/bin/linux/${KUBECTL_ARCH}/kubectl" \
  -o /usr/local/bin/kubectl
chmod 0755 /usr/local/bin/kubectl
kubectl version --client=true

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION:?}/yq_linux_${YQ_ARCH}" \
  -o /usr/local/bin/yq
chmod 0755 /usr/local/bin/yq
yq --version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION:?}.tgz" \
  -o /tmp/docker.tgz
tar -xzf /tmp/docker.tgz -C /usr/local/bin --strip-components=1 docker/docker
chmod 0755 /usr/local/bin/docker
rm /tmp/docker.tgz
docker --version

curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
  "https://pkgs.tailscale.com/stable/tailscale_${TAILSCALE_VERSION:?}_${TS_ARCH}.tgz" \
  -o /tmp/tailscale.tgz
tar -xzf /tmp/tailscale.tgz -C /usr/local/bin --strip-components=1 \
  "tailscale_${TAILSCALE_VERSION}_${TS_ARCH}/tailscale" \
  "tailscale_${TAILSCALE_VERSION}_${TS_ARCH}/tailscaled"
chmod 0755 /usr/local/bin/tailscale /usr/local/bin/tailscaled
rm /tmp/tailscale.tgz
tailscale version
tailscaled --version

git config --system user.name "${AOS_GIT_NAME:?AOS_GIT_NAME is required}"
git config --system user.email "${AOS_GIT_EMAIL:?AOS_GIT_EMAIL is required}"
git config --system --list --show-origin \
  | grep -E 'file:/etc/gitconfig[[:space:]]+user\.(name|email)='
install -d /etc/claude-code /home/ubuntu/.ward/audit /opt/substrate-seed
chown -R 1000:1000 /home/ubuntu/.ward
chmod 0700 /home/ubuntu/.ward /home/ubuntu/.ward/audit

while read -r ref; do
  ref=${ref%$'\r'}
  case "$ref" in
    ''|\#*) continue ;;
  esac
  owner="${ref%%/*}"
  name="${ref##*/}"
  case "$owner" in
    coilysiren) base="https://github.com" ;;
    *) base="https://forgejo.coilysiren.me" ;;
  esac
  git clone --mirror "$base/$owner/$name.git" \
    "/opt/substrate-seed/${owner}__${name}.git"
done < "$substrate_repos"
chmod -R a+rX /opt/substrate-seed
