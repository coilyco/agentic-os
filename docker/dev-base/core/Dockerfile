# Canonical agent dev environment core tier. See docs/dev-base-image.md.

ARG UV_VERSION=0.11.26
ARG GO_VERSION=1.26.5
# Ward raw releases are staging until aos validates them against the real .ward
# bundle. Keep the build tool pinned to the current prod/N-1 ward; the target
# shape is N-1 for build work, then candidate N only at the late config gate.
ARG WARD_VERSION=0.775.0-tmp
ARG TARGETARCH

FROM ubuntu:24.04 AS dev-base-ward-builder
ARG GO_VERSION
ARG WARD_VERSION
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive \
    PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

RUN set -eux; \
    for attempt in 1 2 3; do \
      if apt-get update -o Acquire::Retries=3 \
        && apt-get install -y --no-install-recommends \
          ca-certificates curl git openssh-client; then \
        rm -rf /var/lib/apt/lists/*; \
        break; \
      fi; \
      if [ "$attempt" -eq 3 ]; then \
        exit 1; \
      fi; \
      sleep 5; \
    done

RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) GO_ARCH=amd64 ;; \
      arm64) GO_ARCH=arm64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
      -o /tmp/go.tar.gz; \
    tar -xzf /tmp/go.tar.gz -C /usr/local; \
    rm /tmp/go.tar.gz; \
    go version

RUN set -eux; \
    export GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache \
      GOPROXY=direct GOSUMDB=off GOPRIVATE=forgejo.coilysiren.me CGO_ENABLED=0; \
    for attempt in 1 2 3; do \
      echo "cloning ward source attempt ${attempt}/3" >&2; \
      if timeout 120 git clone --progress --depth 1 --branch "v${WARD_VERSION}" \
        https://forgejo.coilysiren.me/coilyco-flight-deck/ward.git /tmp/ward-src; then \
        break; \
      fi; \
      [ "$attempt" -eq 3 ] && { echo "ward source clone failed after 3 attempts" >&2; exit 1; }; \
      echo "ward source clone attempt ${attempt} failed/timed out; retrying in 5s" >&2; \
      sleep 5; \
    done; \
    cd /tmp/ward-src; \
    for attempt in 1 2 3; do \
      echo "running go mod download attempt ${attempt}/3" >&2; \
      if timeout 240 go mod download; then break; fi; \
      [ "$attempt" -eq 3 ] && { echo "go mod download failed after 3 attempts" >&2; exit 1; }; \
      echo "go mod download attempt ${attempt} failed/timed out; retrying in 5s" >&2; \
      sleep 5; \
    done; \
    go build -trimpath -ldflags "-s -w -X main.Version=v${WARD_VERSION}" \
      -o /usr/local/bin/ward ./cmd/ward; \
    chmod 0755 /usr/local/bin/ward; \
    cd /; rm -rf /tmp/ward-src /tmp/gobuild /tmp/gocache; \
    ward --version

FROM ubuntu:24.04 AS dev-base-core
ARG UV_VERSION
ARG TARGETARCH
ARG NODE_VERSION=22.23.1
ARG CLAUDE_VERSION=2.1.206
ARG MCPORTER_VERSION=0.12.3
ARG CODEX_VERSION=0.144.1
ARG GOOSE_VERSION=1.41.0
ARG OPENCODE_VERSION=1.17.18
ARG AWSCLI_VERSION=2.35.15
ARG GH_VERSION=2.96.0
ARG DOCKER_VERSION=28.5.2
ARG HELM_VERSION=4.2.2
ARG KUBECTL_VERSION=1.36.2
ARG YQ_VERSION=4.53.3
ARG TAILSCALE_VERSION=1.98.8

ENV DEBIAN_FRONTEND=noninteractive \
    PATH=/usr/local/go/bin:/usr/local/node/bin:/usr/local/dotnet:/usr/local/cargo/bin:/usr/local/bin:/usr/bin:/bin:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin \
    HOME=/root \
    HOMEBREW_PREFIX=/home/linuxbrew/.linuxbrew \
    HOMEBREW_NO_ANALYTICS=1 \
    HOMEBREW_NO_AUTO_UPDATE=1 \
    UV_TOOL_DIR=/opt/uv/tools \
    UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python \
    CARGO_HOME=/usr/local/cargo \
    RUSTUP_HOME=/usr/local/rustup \
    AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 \
    DOTNET_ROOT=/usr/local/dotnet \
    DOTNET_CLI_TELEMETRY_OPTOUT=1 \
    DOTNET_NOLOGO=1

# Base packages: catalog-hook deps (shellcheck, python3, git) plus the inner loop's fetch/build tools.
# libicu74 is the .NET globalization backend: the SDK below runs
# with full ICU-backed globalization, NOT invariant mode, so C# string/culture
# behaviour in-container matches a normal host. Ubuntu 24.04 (the pinned base)
# ships ICU as libicu74; a base bump re-pins this to that release's libicuNN.
RUN set -eux; \
    # Ubuntu mirrors occasionally lag package index publication during release churn.
    # Retry the base package install with a fresh index so publish-image can still land.
    for attempt in 1 2 3; do \
      if apt-get update -o Acquire::Retries=3 \
        && apt-get install -y --no-install-recommends \
          ca-certificates curl wget git git-lfs gnupg jq ripgrep less unzip xz-utils socat \
          build-essential openssh-client procps shellcheck file patch \
          passwd \
          python3 python3-pip python3-venv \
          libicu74; then \
        rm -rf /var/lib/apt/lists/*; \
        break; \
      fi; \
      if [ "$attempt" -eq 3 ]; then \
        exit 1; \
      fi; \
      sleep 5; \
    done

# Git LFS is a Git-side capability, not just a client binary, so install the
# system filter config alongside the package. That makes `git lfs version`
# work and the default LFS smudge/clean filters available in future containers.
RUN set -eux; \
    git lfs install --system --skip-repo; \
    git lfs version

# Homebrew (Linuxbrew). The image bakes the prefix and PATH so non-interactive
# runs do not need shell setup or a `brew shellenv` eval at runtime. The
# installer supports both supported build architectures under buildx because the
# build container itself runs the matching Linux arch image. Homebrew itself
# must bootstrap as a non-root user, so create the dedicated user first and
# run the installer there.
RUN set -eux; \
    /usr/sbin/groupadd --gid 1001 linuxbrew; \
    /usr/sbin/useradd --uid 1001 --gid 1001 --create-home --home-dir /home/linuxbrew \
      --shell /bin/bash linuxbrew; \
    install -d -o linuxbrew -g linuxbrew /home/linuxbrew/.linuxbrew

USER linuxbrew

RUN set -eux; \
    export HOME=/home/linuxbrew CI=1 NONINTERACTIVE=1; \
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"; \
    brew --version

USER root

# Map Docker's TARGETARCH onto each upstream's arch spelling once.
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) NODE_ARCH=x64;   GO_ARCH=amd64; AWS_ARCH=x86_64; CODEX_ARCH=x86_64; GOOSE_ARCH=x86_64; GH_ARCH=amd64; DOCKER_ARCH=x86_64; HELM_ARCH=amd64; KUBECTL_ARCH=amd64; YQ_ARCH=amd64; TS_ARCH=amd64; GOLANGCI_ARCH=amd64; TRUFFLEHOG_ARCH=amd64; KDL_ARCH=x86_64; DOTNET_ARCH=x64 ;; \
      arm64) NODE_ARCH=arm64; GO_ARCH=arm64; AWS_ARCH=aarch64; CODEX_ARCH=aarch64; GOOSE_ARCH=aarch64; GH_ARCH=arm64; DOCKER_ARCH=aarch64; HELM_ARCH=arm64; KUBECTL_ARCH=arm64; YQ_ARCH=arm64; TS_ARCH=arm64; GOLANGCI_ARCH=arm64; TRUFFLEHOG_ARCH=arm64; KDL_ARCH=aarch64; DOTNET_ARCH=arm64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    echo "NODE_ARCH=${NODE_ARCH} GO_ARCH=${GO_ARCH} AWS_ARCH=${AWS_ARCH} CODEX_ARCH=${CODEX_ARCH} GOOSE_ARCH=${GOOSE_ARCH} GH_ARCH=${GH_ARCH} DOCKER_ARCH=${DOCKER_ARCH} HELM_ARCH=${HELM_ARCH} KUBECTL_ARCH=${KUBECTL_ARCH} YQ_ARCH=${YQ_ARCH} TS_ARCH=${TS_ARCH} GOLANGCI_ARCH=${GOLANGCI_ARCH} TRUFFLEHOG_ARCH=${TRUFFLEHOG_ARCH} KDL_ARCH=${KDL_ARCH} DOTNET_ARCH=${DOTNET_ARCH}" > /tmp/arch.env

# uv (Python project + tool manager).
RUN set -eux; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://astral.sh/uv/${UV_VERSION}/install.sh" \
      | env INSTALLER_NO_MODIFY_PATH=1 UV_INSTALL_DIR=/usr/local/bin sh; \
    uv --version

# pre-commit, as a uv tool on the global bin dir.
RUN set -eux; \
    uv tool install "pre-commit"; \
    chmod -R a+rX /opt/uv; \
    pre-commit --version

# Managed Pythons for the fleet's required versions. The env
# above points UV_PYTHON_INSTALL_DIR at /opt/uv/python, and the a+rX above grants
# read+traverse but NOT write - so a non-root agent whose repo needs a Python
# other than the system 3.12 (e.g. eco-app's requires-python >=3.13) hit
# `Permission denied` creating /opt/uv/python at its first `uv run`, killing the
# test/smoke gate before any commit could land. Fix both ways: pre-install the
# common versions here as root (clean ownership, no run-time download, offline-
# safe) AND leave the install dir world-writable so an agent can still download
# any version its repo requires on demand. This a+rwX runs after the pre-commit
# a+rX above so the write bit survives; add versions here as the fleet adopts them.
RUN set -eux; \
    uv python install 3.13 3.12; \
    chmod -R a+rwX "${UV_PYTHON_INSTALL_DIR}"; \
    uv python list

# Node is part of the shared substrate because the agent harnesses use its
# runtime. The language specialists remain parallel children of core.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
      -o /tmp/node.tar.xz; \
    mkdir -p /usr/local/node; \
    tar -xJf /tmp/node.tar.xz -C /usr/local/node --strip-components=1 \
      --exclude='*/CHANGELOG.md' --exclude='*/README.md' --exclude='*/LICENSE'; \
    rm /tmp/node.tar.xz; \
    node --version; npm --version

RUN set -eux; \
    npm install -g "@anthropic-ai/claude-code@${CLAUDE_VERSION}"; \
    timeout 5m npm install -g "mcporter@${MCPORTER_VERSION}"; \
    npm_config_cache=/tmp/npm-cache timeout 10m npm install -g "opencode-ai@${OPENCODE_VERSION}"; \
    rm -rf /tmp/npm-cache; \
    claude --version; \
    timeout 1m mcporter --help >/dev/null; \
    opencode --version

RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION}/codex-${CODEX_ARCH}-unknown-linux-musl.tar.gz" \
      -o /tmp/codex.tar.gz; \
    tar -xzf /tmp/codex.tar.gz -C /usr/local/bin; \
    mv "/usr/local/bin/codex-${CODEX_ARCH}-unknown-linux-musl" /usr/local/bin/codex; \
    chmod 0755 /usr/local/bin/codex; \
    rm /tmp/codex.tar.gz; \
    codex --version; \
    curl -fsSL "https://github.com/block/goose/releases/download/v${GOOSE_VERSION}/goose-${GOOSE_ARCH}-unknown-linux-musl.tar.gz" \
      -o /tmp/goose.tar.gz; \
    tar -xzf /tmp/goose.tar.gz -C /usr/local/bin ./goose; \
    chmod 0755 /usr/local/bin/goose; \
    rm /tmp/goose.tar.gz; \
    goose --version

# Operational CLIs are shared substrate capabilities, not a language tier.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${AWS_ARCH}-${AWSCLI_VERSION}.zip" \
      -o /tmp/awscli.zip; \
    unzip -q /tmp/awscli.zip -d /tmp; \
    /tmp/aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli; \
    rm -rf /tmp/awscli.zip /tmp/aws; \
    aws --version

RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" \
      -o /tmp/gh.tar.gz; \
    tar -xzf /tmp/gh.tar.gz -C /usr/local/bin --strip-components=2 \
      "gh_${GH_VERSION}_linux_${GH_ARCH}/bin/gh"; \
    chmod 0755 /usr/local/bin/gh; \
    rm /tmp/gh.tar.gz; \
    gh --version; \
    curl -fsSL "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${HELM_ARCH}.tar.gz" \
      -o /tmp/helm.tar.gz; \
    tar -xzf /tmp/helm.tar.gz -C /usr/local/bin --strip-components=1 \
      "linux-${HELM_ARCH}/helm"; \
    chmod 0755 /usr/local/bin/helm; \
    rm /tmp/helm.tar.gz; \
    helm version --short; \
    curl -fsSL "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${KUBECTL_ARCH}/kubectl" \
      -o /usr/local/bin/kubectl; \
    chmod 0755 /usr/local/bin/kubectl; \
    kubectl version --client=true; \
    curl -fsSL "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_linux_${YQ_ARCH}" \
      -o /usr/local/bin/yq; \
    chmod 0755 /usr/local/bin/yq; \
    yq --version

RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
      -o /tmp/docker.tgz; \
    tar -xzf /tmp/docker.tgz -C /usr/local/bin --strip-components=1 docker/docker; \
    chmod 0755 /usr/local/bin/docker; \
    rm /tmp/docker.tgz; \
    docker --version; \
    curl -fsSL "https://pkgs.tailscale.com/stable/tailscale_${TAILSCALE_VERSION}_${TS_ARCH}.tgz" \
      -o /tmp/tailscale.tgz; \
    tar -xzf /tmp/tailscale.tgz -C /usr/local/bin --strip-components=1 \
      "tailscale_${TAILSCALE_VERSION}_${TS_ARCH}/tailscale" \
      "tailscale_${TAILSCALE_VERSION}_${TS_ARCH}/tailscaled"; \
    chmod 0755 /usr/local/bin/tailscale /usr/local/bin/tailscaled; \
    rm /tmp/tailscale.tgz; \
    tailscale version; \
    tailscaled --version

RUN set -eux; \
    git config --system user.name "${WARD_GIT_NAME:-coilyco-ops}"; \
    git config --system user.email "${WARD_GIT_EMAIL:-coilyco-ops@coilysiren.me}"; \
    git config --system --list --show-origin | grep -E 'file:/etc/gitconfig[[:space:]]+user\.(name|email)='; \
    mkdir -p /opt/agentic-os /etc/claude-code /home/ubuntu/.ward/audit; \
    chown -R 1000:1000 /home/ubuntu/.ward; \
    chmod 0700 /home/ubuntu/.ward /home/ubuntu/.ward/audit
COPY agent-name.sh /opt/agentic-os/agent-name.sh
COPY statusline.sh /opt/agentic-os/statusline.sh
COPY statusline.d/ /opt/agentic-os/statusline.d/
COPY ward-shell-entrypoint.sh /opt/agentic-os/ward-shell-entrypoint.sh
COPY claude-managed-settings.json /etc/claude-code/managed-settings.json
RUN set -eux; \
    chmod 0755 /opt/agentic-os/agent-name.sh /opt/agentic-os/statusline.sh; \
    chmod 0755 /opt/agentic-os/statusline.d/*.sh /opt/agentic-os/ward-shell-entrypoint.sh; \
    chmod 0644 /etc/claude-code/managed-settings.json

COPY substrate-image-repos.txt /tmp/substrate-image-repos.txt
RUN set -eux; \
    mkdir -p /opt/substrate-seed; \
    while read -r ref; do \
      case "$ref" in ''|\#*) continue ;; esac; \
      owner="${ref%%/*}"; name="${ref##*/}"; \
      case "$owner" in \
        coilysiren) base="https://github.com" ;; \
        *) base="https://forgejo.coilysiren.me" ;; \
      esac; \
      git clone --mirror "$base/$owner/$name.git" "/opt/substrate-seed/${owner}__${name}.git"; \
    done < /tmp/substrate-image-repos.txt; \
    rm /tmp/substrate-image-repos.txt; \
    chmod -R a+rX /opt/substrate-seed

# CARGO_HOME/RUSTUP_HOME and the cargo PATH entry stay in core's ENV, but the
# Rust toolchain itself ships in lang-rust and grafts into full (aos#610).
COPY --from=dev-base-ward-builder /usr/local/bin/ward /usr/local/bin/ward
# Keep the per-commit config ref below the expensive tool layers so ordinary
# aos commits invalidate only the real-bundle validation gate.
ARG WARD_CONFIG_REF_COMMIT
ENV WARD_CONFIG_REF=forgejo.coilysiren.me/coilyco-flight-deck/agentic-os@${WARD_CONFIG_REF_COMMIT}//.ward
RUN set -eux; \
    ward --version; \
    CLIGUARD_NO_SANDBOX=1 ward doctor; \
    mkdir -p /home/ubuntu/.ward/audit; \
    chown -R 1000:1000 /home/ubuntu/.ward; \
    chmod 0700 /home/ubuntu/.ward /home/ubuntu/.ward/audit

WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]
