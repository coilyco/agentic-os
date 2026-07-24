# syntax=docker/dockerfile:1
# AOS publishes one development image. The full target contains every supported
# language toolchain and every agent/operator binary.

ARG UV_VERSION=0.11.26
ARG GO_VERSION=1.26.5
ARG DOTNET_VERSION=10.0.301
ARG TRUNK_VERSION=0.21.14
ARG AGENT_COMPOSE_VERSION=0.23.0
ARG WARD_VERSION=0.775.0-tmp
ARG SPECGEN_VERSION=0.118.0
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
ARG GOLANGCI_LINT_VERSION=2.12.2
ARG TRUFFLEHOG_VERSION=3.95.8
ARG KDLFMT_VERSION=0.1.7
ARG TARGETARCH

FROM ubuntu:24.04 AS dev-base-tool-builder
ARG GO_VERSION
ARG WARD_VERSION
ARG SPECGEN_VERSION
ARG TARGETARCH
ARG DEBIAN_FRONTEND=noninteractive

ENV PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN set -eux; \
    for attempt in 1 2 3; do \
      if apt-get update -o Acquire::Retries=3 \
        && apt-get install -y --no-install-recommends \
          ca-certificates \
          curl \
          git \
          openssh-client; then \
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
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
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
      sleep 5; \
    done; \
    cd /tmp/ward-src; \
    for attempt in 1 2 3; do \
      if timeout 240 go mod download; then break; fi; \
      [ "$attempt" -eq 3 ] && { echo "go mod download failed after 3 attempts" >&2; exit 1; }; \
      sleep 5; \
    done; \
    go build -trimpath -ldflags "-s -w -X main.Version=v${WARD_VERSION}" \
      -o /usr/local/bin/ward ./cmd/ward; \
    chmod 0755 /usr/local/bin/ward; \
    cd /; \
    rm -rf /tmp/ward-src /tmp/gobuild /tmp/gocache; \
    ward --version

WORKDIR /tmp/aos-src
COPY --from=aos-cli go.mod go.sum ./
RUN set -eux; \
    export GOPATH=/tmp/aos-gopath GOCACHE=/tmp/aos-gocache; \
    go mod download
COPY --from=aos-cli . .
RUN set -eux; \
    export GOPATH=/tmp/aos-gopath GOCACHE=/tmp/aos-gocache CGO_ENABLED=0; \
    go build -trimpath -ldflags "-s -w -X main.version=dev-base" \
      -o /usr/local/bin/aos .; \
    chmod 0755 /usr/local/bin/aos; \
    rm -rf /tmp/aos-gopath /tmp/aos-gocache; \
    /usr/local/bin/aos --version

RUN set -eux; \
    specgen_asset="specgen-linux-${TARGETARCH}"; \
    specgen_base="https://forgejo.coilysiren.me/coilyco-flight-deck/cli-guard/releases/download/v${SPECGEN_VERSION}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${specgen_base}/${specgen_asset}" -o "/tmp/${specgen_asset}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${specgen_base}/SHA256SUMS" -o /tmp/specgen-SHA256SUMS; \
    grep "[[:space:]]${specgen_asset}$" /tmp/specgen-SHA256SUMS \
      | (cd /tmp && sha256sum -c -); \
    install -m 0755 "/tmp/${specgen_asset}" /usr/local/bin/specgen; \
    rm "/tmp/${specgen_asset}" /tmp/specgen-SHA256SUMS; \
    specgen --version

COPY --from=aguard-spec . /tmp/aguard-spec
RUN set -eux; \
    specgen --project-root /tmp/aguard-spec build \
      --set-version "dev-base" \
      --out /usr/local/bin/aguard; \
    chmod 0755 /usr/local/bin/aguard; \
    rm -rf /tmp/aguard-spec; \
    aguard --version

FROM ubuntu:24.04 AS dev-base-full
ARG UV_VERSION
ARG GO_VERSION
ARG DOTNET_VERSION
ARG TRUNK_VERSION
ARG AGENT_COMPOSE_VERSION
ARG NODE_VERSION
ARG CLAUDE_VERSION
ARG MCPORTER_VERSION
ARG CODEX_VERSION
ARG GOOSE_VERSION
ARG OPENCODE_VERSION
ARG AWSCLI_VERSION
ARG GH_VERSION
ARG DOCKER_VERSION
ARG HELM_VERSION
ARG KUBECTL_VERSION
ARG YQ_VERSION
ARG TAILSCALE_VERSION
ARG GOLANGCI_LINT_VERSION
ARG TRUFFLEHOG_VERSION
ARG KDLFMT_VERSION
ARG TARGETARCH

ENV PATH=/usr/local/go/bin:/usr/local/node/bin:/usr/local/dotnet:/usr/local/cargo/bin:/usr/local/bin:/usr/bin:/bin:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin \
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
    DOTNET_NOLOGO=1 \
    PYTHONPATH=/opt/agentic-os/python

LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os full"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=type=bind,source=substrate-image-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt

RUN set -eux; \
    . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
      -o /tmp/go.tar.gz; \
    tar -xzf /tmp/go.tar.gz -C /usr/local; \
    rm /tmp/go.tar.gz; \
    go version

RUN set -eux; \
    . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh; \
    chmod 0755 /tmp/dotnet-install.sh; \
    /tmp/dotnet-install.sh --version "${DOTNET_VERSION}" --architecture "${DOTNET_ARCH}" \
      --install-dir /usr/local/dotnet --no-path; \
    rm /tmp/dotnet-install.sh; \
    chmod -R a+rX /usr/local/dotnet; \
    dotnet --list-sdks

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      libasound2-dev \
      libudev-dev \
      libwayland-dev \
      libxkbcommon-dev \
      pkg-config; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      https://sh.rustup.rs -o /tmp/rustup-init.sh; \
    chmod 0755 /tmp/rustup-init.sh; \
    /tmp/rustup-init.sh -y --no-modify-path --profile minimal --default-toolchain stable; \
    rm /tmp/rustup-init.sh; \
    rustup target add wasm32-unknown-unknown; \
    chmod -R a+rwX /usr/local/cargo /usr/local/rustup; \
    cargo --version; \
    rustc --version

RUN set -eux; \
    . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/trunk-rs/trunk/releases/download/v${TRUNK_VERSION}/trunk-${KDL_ARCH}-unknown-linux-gnu.tar.gz" \
      -o /tmp/trunk.tar.gz; \
    tar -xzf /tmp/trunk.tar.gz -C /usr/local/cargo/bin trunk; \
    chmod 0755 /usr/local/cargo/bin/trunk; \
    rm /tmp/trunk.tar.gz; \
    trunk --version

RUN set -eux; \
    printf '%s\n' \
      '#include <wayland-client.h>' \
      '#include <xkbcommon/xkbcommon.h>' \
      'int main(void) { return 0; }' \
      > /tmp/rust-native-smoke.c; \
    cc /tmp/rust-native-smoke.c \
      $(pkg-config --cflags --libs wayland-client xkbcommon) \
      -o /tmp/rust-native-smoke; \
    /tmp/rust-native-smoke; \
    rm /tmp/rust-native-smoke /tmp/rust-native-smoke.c

RUN set -eux; \
    ln -sf /usr/bin/python3 /usr/local/bin/python; \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip; \
    uv tool install pipenv; \
    chmod -R a+rX /opt/uv; \
    python --version; \
    pip --version; \
    pipenv --version

RUN set -eux; \
    . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${GOLANGCI_ARCH}.tar.gz" \
      -o /tmp/golangci-lint.tar.gz; \
    tar -xzf /tmp/golangci-lint.tar.gz -C /usr/local/bin --strip-components=1 \
      "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${GOLANGCI_ARCH}/golangci-lint"; \
    chmod 0755 /usr/local/bin/golangci-lint; \
    rm /tmp/golangci-lint.tar.gz; \
    golangci-lint --version

RUN set -eux; \
    . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/trufflesecurity/trufflehog/releases/download/v${TRUFFLEHOG_VERSION}/trufflehog_${TRUFFLEHOG_VERSION}_linux_${TRUFFLEHOG_ARCH}.tar.gz" \
      -o /tmp/trufflehog.tar.gz; \
    tar -xzf /tmp/trufflehog.tar.gz -C /usr/local/bin trufflehog; \
    chmod 0755 /usr/local/bin/trufflehog; \
    rm /tmp/trufflehog.tar.gz; \
    trufflehog --version

RUN set -eux; \
    . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/hougesen/kdlfmt/releases/download/v${KDLFMT_VERSION}/kdlfmt-${KDL_ARCH}-unknown-linux-gnu.tar.xz" \
      -o /tmp/kdlfmt.tar.xz; \
    tar -xJf /tmp/kdlfmt.tar.xz -C /usr/local/bin --strip-components=1 \
      "kdlfmt-${KDL_ARCH}-unknown-linux-gnu/kdlfmt"; \
    chmod 0755 /usr/local/bin/kdlfmt; \
    rm /tmp/kdlfmt.tar.xz; \
    kdlfmt --version

COPY --chmod=0755 agent-name.sh /opt/agentic-os/agent-name.sh
COPY --chmod=0755 statusline.sh /opt/agentic-os/statusline.sh
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0755 ward-shell-entrypoint.sh /opt/agentic-os/ward-shell-entrypoint.sh
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --chmod=0644 substrate-image-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aguard /usr/local/bin/aguard
COPY --from=aguard-python . /opt/agentic-os/python/agentic_os/

ARG WARD_CONFIG_REF_COMMIT
ENV WARD_CONFIG_REF=forgejo.coilysiren.me/coilyco-flight-deck/agentic-os@${WARD_CONFIG_REF_COMMIT}//.ward

RUN set -eux; \
    node --version; \
    go version; \
    dotnet --list-sdks; \
    cargo --version; \
    trunk --version; \
    rustup target list --installed | grep -q wasm32-unknown-unknown; \
    python --version; \
    pipenv --version; \
    aguard --version; \
    python3 -m agentic_os.forgejo_actions_list --help >/dev/null; \
    ward --version; \
    CLIGUARD_NO_SANDBOX=1 ward doctor

WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]
