# syntax=docker/dockerfile:1
# Every published language target starts directly from Ubuntu. The full image
# fans the independently built language images back into one compatibility
# surface.

ARG UV_VERSION=0.12.0
ARG GO_VERSION=1.26.5
ARG DOTNET_VERSION=10.0.301
ARG TRUNK_VERSION=0.21.14
ARG AGENT_COMPOSE_VERSION=1.36.0
ARG WARD_VERSION=0.856.0
ARG SPECGEN_VERSION=0.127.0
ARG NODE_VERSION=22.23.1
ARG CLAUDE_VERSION=2.1.220
ARG MCPORTER_VERSION=0.12.3
ARG CODEX_VERSION=0.146.0
ARG GOOSE_VERSION=1.44.0
ARG OPENCODE_VERSION=1.17.18
ARG AWSCLI_VERSION=2.36.10
ARG GH_VERSION=2.96.0
ARG DOCKER_VERSION=28.5.2
ARG HELM_VERSION=4.2.2
ARG KUBECTL_VERSION=1.36.2
ARG YQ_VERSION=4.53.3
ARG TAILSCALE_VERSION=1.98.10
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

COPY --from=aosguard-spec . /tmp/aosguard-spec
RUN set -eux; \
    specgen --project-root /tmp/aosguard-spec \
      --skills-out /opt/agentic-os/aosguard-skill build \
      --set-version "dev-base" \
      --out /usr/local/bin/aosguard; \
    chmod 0755 /usr/local/bin/aosguard; \
    test -s /opt/agentic-os/aosguard-skill/aosguard/SKILL.md; \
    test -s /opt/agentic-os/aosguard-skill/aosguard/references/commands.yaml; \
    rm -rf /tmp/aosguard-spec; \
    aosguard --version

FROM ubuntu:24.04 AS dev-base-lang-node
ARG UV_VERSION
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
ARG TARGETARCH
ENV PATH=/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/root UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 AOS_GIT_NAME=coilyco-ops \
    AOS_GIT_EMAIL=coilyco-ops@coilysiren.me PYTHONPATH=/opt/agentic-os/python
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os lang-node"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=type=bind,source=substrate-image-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt
COPY --chmod=0755 agent-name.sh statusline.sh ward-shell-entrypoint.sh /opt/agentic-os/
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --chmod=0644 substrate-image-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aosguard /usr/local/bin/aosguard
COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill /opt/agentic-os/aosguard-skill
COPY --from=aosguard-python . /opt/agentic-os/python/agentic_os/
RUN --mount=type=bind,source=verify-common.sh,target=/tmp/verify-common.sh \
    node --version && bash /tmp/verify-common.sh
WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]

FROM ubuntu:24.04 AS dev-base-lang-go
ARG UV_VERSION
ARG GO_VERSION
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
ARG TARGETARCH
ENV PATH=/usr/local/go/bin:/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/root UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 AOS_GIT_NAME=coilyco-ops \
    AOS_GIT_EMAIL=coilyco-ops@coilysiren.me PYTHONPATH=/opt/agentic-os/python
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os lang-go"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=type=bind,source=substrate-image-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz; \
    tar -xzf /tmp/go.tar.gz -C /usr/local; rm /tmp/go.tar.gz; go version
COPY --chmod=0755 agent-name.sh statusline.sh ward-shell-entrypoint.sh /opt/agentic-os/
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --chmod=0644 substrate-image-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aosguard /usr/local/bin/aosguard
COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill /opt/agentic-os/aosguard-skill
COPY --from=aosguard-python . /opt/agentic-os/python/agentic_os/
RUN --mount=type=bind,source=verify-common.sh,target=/tmp/verify-common.sh \
    go version && bash /tmp/verify-common.sh
WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]

FROM ubuntu:24.04 AS dev-base-lang-dotnet
ARG UV_VERSION
ARG DOTNET_VERSION
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
ARG TARGETARCH
ENV PATH=/usr/local/dotnet:/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/root UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 DOTNET_ROOT=/usr/local/dotnet \
    DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1 \
    AOS_GIT_NAME=coilyco-ops AOS_GIT_EMAIL=coilyco-ops@coilysiren.me \
    PYTHONPATH=/opt/agentic-os/python
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os lang-dotnet"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=type=bind,source=substrate-image-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh; \
    chmod 0755 /tmp/dotnet-install.sh; \
    /tmp/dotnet-install.sh --version "${DOTNET_VERSION}" --architecture "${DOTNET_ARCH}" \
      --install-dir /usr/local/dotnet --no-path; rm /tmp/dotnet-install.sh; \
    chmod -R a+rX /usr/local/dotnet; dotnet --list-sdks
COPY --chmod=0755 agent-name.sh statusline.sh ward-shell-entrypoint.sh /opt/agentic-os/
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --chmod=0644 substrate-image-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aosguard /usr/local/bin/aosguard
COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill /opt/agentic-os/aosguard-skill
COPY --from=aosguard-python . /opt/agentic-os/python/agentic_os/
RUN --mount=type=bind,source=verify-common.sh,target=/tmp/verify-common.sh \
    dotnet --list-sdks && bash /tmp/verify-common.sh
WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]

FROM ubuntu:24.04 AS dev-base-lang-rust
ARG UV_VERSION
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
ARG TARGETARCH
ENV PATH=/usr/local/node/bin:/usr/local/cargo/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/root UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python CARGO_HOME=/usr/local/cargo \
    RUSTUP_HOME=/usr/local/rustup AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 AOS_GIT_NAME=coilyco-ops \
    AOS_GIT_EMAIL=coilyco-ops@coilysiren.me PYTHONPATH=/opt/agentic-os/python
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os lang-rust"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=type=bind,source=substrate-image-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt
RUN set -eux; apt-get update; apt-get install -y --no-install-recommends \
      libasound2-dev libudev-dev libwayland-dev libxkbcommon-dev pkg-config; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      https://sh.rustup.rs -o /tmp/rustup-init.sh; chmod 0755 /tmp/rustup-init.sh; \
    /tmp/rustup-init.sh -y --no-modify-path --profile minimal --default-toolchain stable; \
    rm /tmp/rustup-init.sh; rustup target add wasm32-unknown-unknown; \
    chmod -R a+rwX /usr/local/cargo /usr/local/rustup; cargo --version
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/trunk-rs/trunk/releases/download/v${TRUNK_VERSION}/trunk-${KDL_ARCH}-unknown-linux-gnu.tar.gz" \
      -o /tmp/trunk.tar.gz; tar -xzf /tmp/trunk.tar.gz -C /usr/local/cargo/bin trunk; \
    chmod 0755 /usr/local/cargo/bin/trunk; rm /tmp/trunk.tar.gz; trunk --version
RUN printf '%s\n' '#include <wayland-client.h>' '#include <xkbcommon/xkbcommon.h>' \
      'int main(void) { return 0; }' > /tmp/rust-native-smoke.c && \
    cc /tmp/rust-native-smoke.c $(pkg-config --cflags --libs wayland-client xkbcommon) \
      -o /tmp/rust-native-smoke && /tmp/rust-native-smoke && \
    rm /tmp/rust-native-smoke /tmp/rust-native-smoke.c
COPY --chmod=0755 agent-name.sh statusline.sh ward-shell-entrypoint.sh /opt/agentic-os/
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --chmod=0644 substrate-image-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aosguard /usr/local/bin/aosguard
COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill /opt/agentic-os/aosguard-skill
COPY --from=aosguard-python . /opt/agentic-os/python/agentic_os/
RUN --mount=type=bind,source=verify-common.sh,target=/tmp/verify-common.sh \
    cargo --version && trunk --version && bash /tmp/verify-common.sh
WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]

FROM ubuntu:24.04 AS dev-base-lang-python
ARG UV_VERSION
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
ARG TARGETARCH
ENV PATH=/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/root UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 AOS_GIT_NAME=coilyco-ops \
    AOS_GIT_EMAIL=coilyco-ops@coilysiren.me PYTHONPATH=/opt/agentic-os/python
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os lang-python"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=type=bind,source=substrate-image-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt
RUN set -eux; ln -sf /usr/bin/python3 /usr/local/bin/python; \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip; uv tool install pipenv; \
    chmod -R a+rX /opt/uv; python --version; pip --version; pipenv --version
COPY --chmod=0755 agent-name.sh statusline.sh ward-shell-entrypoint.sh /opt/agentic-os/
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --chmod=0644 substrate-image-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aosguard /usr/local/bin/aosguard
COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill /opt/agentic-os/aosguard-skill
COPY --from=aosguard-python . /opt/agentic-os/python/agentic_os/
RUN --mount=type=bind,source=verify-common.sh,target=/tmp/verify-common.sh \
    python --version && pipenv --version && bash /tmp/verify-common.sh
WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]
