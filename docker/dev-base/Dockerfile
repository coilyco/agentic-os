# syntax=docker/dockerfile:1
# Language targets are build-only payloads. Forgejo builds them in parallel,
# keeps stable registry caches across runs, and feeds their commit-scoped draft
# manifests into the one released full image.

ARG UV_VERSION=0.12.0
ARG GO_VERSION=1.26.5
ARG DOTNET_VERSION=10.0.302
ARG TRUNK_VERSION=0.21.14
ARG NODE_VERSION=22.23.2
ARG WASM_PACK_VERSION=0.15.0
ARG WASM_BINDGEN_VERSION=0.2.118
ARG BINARYEN_VERSION=119
# Toolchains consumers pin in rust-toolchain.toml, baked so a pinned repo
# never fetches a channel at build time. See docs/dev-base-image.md.
ARG RUST_PINNED_VERSIONS="1.90.0"

FROM ubuntu:24.04 AS dev-base-lang-node
ARG NODE_VERSION
ARG TARGETARCH
ENV PATH=/usr/local/node/bin:/usr/local/bin:/usr/bin:/bin
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os Node build payload"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=prepare-build-stage.sh,target=/tmp/prepare-build-stage.sh \
    TARGETARCH="${TARGETARCH}" bash /tmp/prepare-build-stage.sh
RUN set -eux; . /opt/agentic-os/arch.env; \
    apt-get update; apt-get install -y --no-install-recommends ca-certificates curl xz-utils; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
      -o /tmp/node.tar.xz; \
    install -d /usr/local/node; \
    tar -xJf /tmp/node.tar.xz -C /usr/local/node --strip-components=1 \
      --exclude='*/CHANGELOG.md' --exclude='*/README.md' --exclude='*/LICENSE'; \
    rm /tmp/node.tar.xz; node --version; npm --version

FROM ubuntu:24.04 AS dev-base-lang-go
ARG GO_VERSION
ARG TARGETARCH
ENV PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os Go build payload"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=prepare-build-stage.sh,target=/tmp/prepare-build-stage.sh \
    TARGETARCH="${TARGETARCH}" bash /tmp/prepare-build-stage.sh
RUN set -eux; . /opt/agentic-os/arch.env; \
    apt-get update; apt-get install -y --no-install-recommends ca-certificates curl; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz; \
    tar -xzf /tmp/go.tar.gz -C /usr/local; rm /tmp/go.tar.gz; go version

FROM ubuntu:24.04 AS dev-base-lang-dotnet
ARG DOTNET_VERSION
ARG TARGETARCH
ENV PATH=/usr/local/dotnet:/usr/local/bin:/usr/bin:/bin \
    DOTNET_ROOT=/usr/local/dotnet DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os .NET build payload"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=prepare-build-stage.sh,target=/tmp/prepare-build-stage.sh \
    TARGETARCH="${TARGETARCH}" bash /tmp/prepare-build-stage.sh
RUN set -eux; . /opt/agentic-os/arch.env; \
    apt-get update; apt-get install -y --no-install-recommends ca-certificates curl; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh; \
    chmod 0755 /tmp/dotnet-install.sh; \
    /tmp/dotnet-install.sh --version "${DOTNET_VERSION}" --architecture "${DOTNET_ARCH}" \
      --install-dir /usr/local/dotnet --no-path; \
    rm /tmp/dotnet-install.sh; chmod -R a+rX /usr/local/dotnet; dotnet --list-sdks

FROM ubuntu:24.04 AS dev-base-lang-rust
ARG TRUNK_VERSION
ARG WASM_PACK_VERSION
ARG WASM_BINDGEN_VERSION
ARG BINARYEN_VERSION
ARG RUST_PINNED_VERSIONS
ARG TARGETARCH
ENV PATH=/usr/local/cargo/bin:/usr/local/bin:/usr/bin:/bin \
    CARGO_HOME=/usr/local/cargo RUSTUP_HOME=/usr/local/rustup
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os Rust build payload"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=prepare-build-stage.sh,target=/tmp/prepare-build-stage.sh \
    TARGETARCH="${TARGETARCH}" bash /tmp/prepare-build-stage.sh
RUN set -eux; apt-get update; apt-get install -y --no-install-recommends \
      build-essential ca-certificates curl libasound2-dev libudev-dev \
      libwayland-dev libxkbcommon-dev pkg-config; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      https://sh.rustup.rs -o /tmp/rustup-init.sh; chmod 0755 /tmp/rustup-init.sh; \
    /tmp/rustup-init.sh -y --no-modify-path --profile minimal --default-toolchain stable; \
    rm /tmp/rustup-init.sh; rustup target add wasm32-unknown-unknown; \
    rustup component add clippy rustfmt; \
    for pin in ${RUST_PINNED_VERSIONS}; do \
      rustup toolchain install "${pin}" --profile minimal \
        --component clippy --component rustfmt; \
      rustup target add wasm32-unknown-unknown --toolchain "${pin}"; \
    done; \
    chmod -R a+rwX /usr/local/cargo /usr/local/rustup; cargo --version
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/trunk-rs/trunk/releases/download/v${TRUNK_VERSION}/trunk-${KDL_ARCH}-unknown-linux-gnu.tar.gz" \
      -o /tmp/trunk.tar.gz; \
    tar -xzf /tmp/trunk.tar.gz -C /usr/local/cargo/bin trunk; \
    chmod 0755 /usr/local/cargo/bin/trunk; rm /tmp/trunk.tar.gz; trunk --version
# wasm-pack is the second Rust-to-WASM front end alongside trunk: trunk serves
# whole-app Rust bundles, wasm-pack emits an npm package a JS bundler consumes.
# Pinned rather than `cargo install wasm-pack`, which resolves to whatever is
# newest that day and made every consumer's bundle a different artifact.
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/rustwasm/wasm-pack/releases/download/v${WASM_PACK_VERSION}/wasm-pack-v${WASM_PACK_VERSION}-${WASM_PACK_ARCH}-unknown-linux-musl.tar.gz" \
      -o /tmp/wasm-pack.tar.gz; \
    tar -xzf /tmp/wasm-pack.tar.gz -C /usr/local/cargo/bin --strip-components=1 \
      "wasm-pack-v${WASM_PACK_VERSION}-${WASM_PACK_ARCH}-unknown-linux-musl/wasm-pack"; \
    chmod 0755 /usr/local/cargo/bin/wasm-pack; rm /tmp/wasm-pack.tar.gz; \
    wasm-pack --version
# wasm-pack shells out to the wasm-bindgen CLI and downloads its own copy when
# PATH has none, the same hole the wasm-opt block below closes. This pin must
# track the wasm-bindgen CRATE consumers lock, or wasm-pack fetches the
# matching CLI anyway: docs/dev-base-wasm-toolchain.md carries that rule.
# Only the binaries wasm-pack invokes are kept; wasm2es6js it never calls.
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/wasm-bindgen/wasm-bindgen/releases/download/${WASM_BINDGEN_VERSION}/wasm-bindgen-${WASM_BINDGEN_VERSION}-${WASM_BINDGEN_ARCH}-unknown-linux-musl.tar.gz" \
      -o /tmp/wasm-bindgen.tar.gz; \
    tar -xzf /tmp/wasm-bindgen.tar.gz -C /usr/local/cargo/bin --strip-components=1 \
      "wasm-bindgen-${WASM_BINDGEN_VERSION}-${WASM_BINDGEN_ARCH}-unknown-linux-musl/wasm-bindgen" \
      "wasm-bindgen-${WASM_BINDGEN_VERSION}-${WASM_BINDGEN_ARCH}-unknown-linux-musl/wasm-bindgen-test-runner"; \
    chmod 0755 /usr/local/cargo/bin/wasm-bindgen \
      /usr/local/cargo/bin/wasm-bindgen-test-runner; \
    rm /tmp/wasm-bindgen.tar.gz; wasm-bindgen --version
# binaryen supplies wasm-opt, which wasm-pack runs as its optimization pass.
# wasm-pack takes wasm-opt from PATH when it finds one and otherwise downloads
# its own floating "latest" build, so an unpinned image silently reoptimizes
# every bundle differently. Pinning it here is what makes the wasm output a
# function of the source rather than of the build date. Only bin/wasm-opt is
# kept: the tarball's lib/libbinaryen.a is a static archive the binary has
# already absorbed, and include/ is for C API consumers this image has none of.
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/WebAssembly/binaryen/releases/download/version_${BINARYEN_VERSION}/binaryen-version_${BINARYEN_VERSION}-${BINARYEN_ARCH}-linux.tar.gz" \
      -o /tmp/binaryen.tar.gz; \
    tar -xzf /tmp/binaryen.tar.gz -C /usr/local/bin --strip-components=2 \
      "binaryen-version_${BINARYEN_VERSION}/bin/wasm-opt"; \
    chmod 0755 /usr/local/bin/wasm-opt; rm /tmp/binaryen.tar.gz; wasm-opt --version
RUN printf '%s\n' '#include <wayland-client.h>' '#include <xkbcommon/xkbcommon.h>' \
      'int main(void) { return 0; }' > /tmp/rust-native-smoke.c; \
    cc /tmp/rust-native-smoke.c $(pkg-config --cflags --libs wayland-client xkbcommon) \
      -o /tmp/rust-native-smoke; /tmp/rust-native-smoke; \
    rm /tmp/rust-native-smoke /tmp/rust-native-smoke.c

FROM ubuntu:24.04 AS dev-base-lang-python
ARG UV_VERSION
ARG TARGETARCH
ENV PATH=/usr/local/bin:/usr/bin:/bin \
    UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os Python build payload"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN --mount=type=bind,source=prepare-build-stage.sh,target=/tmp/prepare-build-stage.sh \
    TARGETARCH="${TARGETARCH}" bash /tmp/prepare-build-stage.sh
RUN set -eux; apt-get update; apt-get install -y --no-install-recommends \
      ca-certificates curl python3 python3-pip python3-venv; \
    rm -rf /var/lib/apt/lists/*; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://astral.sh/uv/${UV_VERSION}/install.sh" \
      | env INSTALLER_NO_MODIFY_PATH=1 UV_INSTALL_DIR=/usr/local/bin sh; \
    uv tool install pipenv; uv python install 3.13 3.12; \
    chmod -R a+rwX /opt/uv; uv --version; pipenv --version; uv python list
