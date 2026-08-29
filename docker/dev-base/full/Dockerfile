# syntax=docker/dockerfile:1
ARG BASE_IMAGE=ubuntu:24.04
ARG LANG_NODE_IMAGE=${BASE_IMAGE}
ARG LANG_GO_IMAGE=${BASE_IMAGE}
ARG LANG_DOTNET_IMAGE=${BASE_IMAGE}
ARG LANG_PYTHON_IMAGE=${BASE_IMAGE}
FROM ${LANG_NODE_IMAGE} AS dev-base-lang-node-graft
FROM ${LANG_GO_IMAGE} AS dev-base-lang-go-graft
FROM ${LANG_DOTNET_IMAGE} AS dev-base-lang-dotnet-graft
FROM ${LANG_PYTHON_IMAGE} AS dev-base-lang-python-graft

FROM ubuntu:24.04 AS dev-base-tool-builder
ARG WARD_VERSION=0.887.0
ARG SPECGEN_VERSION=0.184.0
ARG AOS_VERSION=0.213.0
ARG TARGETARCH
ARG DEBIAN_FRONTEND=noninteractive
ENV PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go
RUN set -eux; \
    apt-get update; apt-get install -y --no-install-recommends \
      ca-certificates curl git openssh-client python3 python3-yaml; \
    rm -rf /var/lib/apt/lists/*
RUN set -eux; \
    ward_asset="ward-linux-${TARGETARCH}"; \
    ward_base="https://forgejo.coilysiren.me/coilyco-flight-deck/ward/releases/download/v${WARD_VERSION}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${ward_base}/${ward_asset}" -o "/tmp/${ward_asset}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${ward_base}/SHA256SUMS" -o /tmp/ward-SHA256SUMS; \
    grep "[[:space:]]${ward_asset}$" /tmp/ward-SHA256SUMS \
      | (cd /tmp && sha256sum -c -); \
    install -m 0755 "/tmp/${ward_asset}" /usr/local/bin/ward; \
    rm "/tmp/${ward_asset}" /tmp/ward-SHA256SUMS; ward --version
RUN set -eux; \
    aos_asset="aos-linux-${TARGETARCH}"; \
    aos_base="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/releases/download/aos-v${AOS_VERSION}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${aos_base}/${aos_asset}" -o "/tmp/${aos_asset}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${aos_base}/SHA256SUMS" -o /tmp/aos-SHA256SUMS; \
    grep "[[:space:]]${aos_asset}$" /tmp/aos-SHA256SUMS \
      | (cd /tmp && sha256sum -c -); \
    install -m 0755 "/tmp/${aos_asset}" /usr/local/bin/aos; \
    rm "/tmp/${aos_asset}" /tmp/aos-SHA256SUMS; aos --version
RUN set -eux; \
    specgen_asset="specgen-linux-${TARGETARCH}"; \
    specgen_base="https://forgejo.coilysiren.me/coilyco-flight-deck/umbra/releases/download/v${SPECGEN_VERSION}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${specgen_base}/${specgen_asset}" -o "/tmp/${specgen_asset}"; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "${specgen_base}/SHA256SUMS" -o /tmp/specgen-SHA256SUMS; \
    grep "[[:space:]]${specgen_asset}$" /tmp/specgen-SHA256SUMS \
      | (cd /tmp && sha256sum -c -); \
    install -m 0755 "/tmp/${specgen_asset}" /usr/local/bin/specgen; \
    rm "/tmp/${specgen_asset}" /tmp/specgen-SHA256SUMS; specgen --version
COPY --from=aosguard-spec . /tmp/aosguard-spec
COPY --from=aosguard-python . /tmp/aosguard-python/agentic_os
RUN set -eux; \
    specgen --project-root /tmp/aosguard-spec/guardfiles \
      --skills-out /opt/agentic-os/aosguard-skill build \
      --set-version "dev-base" --out /usr/local/bin/aosguard; \
    chmod 0755 /usr/local/bin/aosguard; \
    test -s /opt/agentic-os/aosguard-skill/aosguard/SKILL.md; \
    test -s /opt/agentic-os/aosguard-skill/aosguard/references/commands.yaml; \
    PYTHONPATH=/tmp/aosguard-python python3 -m agentic_os.generators.generate_aosguard_skills \
      --skills-root /opt/agentic-os/aosguard-skill; \
    test -s /opt/agentic-os/aosguard-skill/aosguard-forgejo/SKILL.md; \
    rm -rf /tmp/aosguard-spec /tmp/aosguard-python; aosguard --version

FROM ${BASE_IMAGE} AS dev-base-full
ARG AGENT_COMPOSE_VERSION=2.51.0
ARG CLAUDE_VERSION=2.1.221
ARG MCPORTER_VERSION=0.12.3
ARG CODEX_VERSION=0.146.0
ARG GOOSE_VERSION=1.45.0
ARG OPENCODE_VERSION=1.18.12
ARG AWSCLI_VERSION=2.36.15
ARG GH_VERSION=2.97.0
ARG DOCKER_VERSION=28.5.2
ARG HELM_VERSION=4.2.3
ARG KUBECTL_VERSION=1.36.3
ARG YQ_VERSION=4.53.3
ARG JUST_VERSION=1.58.0
ARG TAILSCALE_VERSION=1.98.10
ARG GIT_LFS_VERSION=3.7.1
ARG GIT_LFS_SHA256_AMD64=1c0b6ee5200ca708c5cebebb18fdeb0e1c98f1af5c1a9cba205a4c0ab5a5ec08
ARG GIT_LFS_SHA256_ARM64=73a9c90eeb4312133a63c3eaee0c38c019ea7bfa0953d174809d25b18588dd8d
ARG TARGETARCH
ENV PATH=/usr/local/go/bin:/usr/local/dotnet:/usr/local/node/bin:/usr/local/cargo/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/root UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python AWS_DEFAULT_REGION=us-east-1 \
    AWS_REGION=us-east-1 DOTNET_ROOT=/usr/local/dotnet \
    DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1 \
    PRE_COMMIT_HOME=/opt/pre-commit \
    AOS_GIT_NAME=coilyco-ops AOS_GIT_EMAIL=coilyco-ops@coilysiren.me
LABEL org.opencontainers.image.source="https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.title="agentic-os full"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

COPY --from=dev-base-lang-node-graft /usr/local/node /usr/local/node
COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go
COPY --from=dev-base-lang-dotnet-graft /usr/local/dotnet /usr/local/dotnet
COPY --from=dev-base-lang-python-graft /usr/local/bin/uv /usr/local/bin/uv
COPY --from=dev-base-lang-python-graft /usr/local/bin/pipenv /usr/local/bin/pipenv
COPY --from=dev-base-lang-python-graft /opt/uv /opt/uv
RUN --mount=type=bind,source=install-common.sh,target=/tmp/install-common.sh \
    --mount=from=repo-lists,source=substrate-repos.txt,target=/tmp/substrate-repos.txt \
    bash /tmp/install-common.sh /tmp/substrate-repos.txt "${GIT_LFS_VERSION}" \
      "${GIT_LFS_SHA256_AMD64}" "${GIT_LFS_SHA256_ARM64}"
RUN set -eux; ln -sf /usr/bin/python3 /usr/local/bin/python; \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip; \
    chmod -R a+rX /opt/uv; python --version; pip --version; pipenv --version
COPY --chmod=0755 session-name.sh git-identity.sh statusline.sh ward-shell-entrypoint.sh /opt/agentic-os/
COPY --chmod=0755 statusline.d/ /opt/agentic-os/statusline.d/
COPY --chmod=0644 claude-managed-settings.json /etc/claude-code/managed-settings.json
COPY --from=repo-lists --chmod=0644 substrate-repos.txt /opt/agentic-os/substrate-repos.txt
COPY --from=dev-base-tool-builder /usr/local/bin/ward /usr/local/bin/ward
COPY --from=dev-base-tool-builder /usr/local/bin/aos /usr/local/bin/aos
COPY --from=dev-base-tool-builder /usr/local/bin/aosguard /usr/local/bin/aosguard
COPY --from=dev-base-tool-builder /opt/agentic-os/aosguard-skill /opt/agentic-os/aosguard-skill
# aos#771: the system interpreter carries the operator modules. A global
# PYTHONPATH would also reach every isolated venv and void a pinned hook rev.
COPY --from=aosguard-python . /tmp/agentic-os-python/agentic_os/
# Ask site where this interpreter looks rather than hardcoding a distribution
# path, then prove the import in the same layer.
RUN set -eux; \
    site="$(python3 -c 'import site; print(site.getsitepackages()[0])')"; \
    mkdir -p "${site}"; \
    mv /tmp/agentic-os-python/agentic_os "${site}/agentic_os"; \
    rmdir /tmp/agentic-os-python; \
    python3 -c 'import agentic_os; print(agentic_os.__file__)'
RUN --mount=type=bind,source=verify-common.sh,target=/tmp/verify-common.sh \
    bash /tmp/verify-common.sh

# install-hooks needs a work tree, so it gets a throwaway one. Consumer repos
# read this cache through PRE_COMMIT_HOME and never write a pin back into it.
RUN --mount=type=bind,source=fleet-precommit-hooks.yaml,target=/tmp/fleet-precommit-hooks.yaml \
    set -eux; \
    warm="$(mktemp -d)"; cd "${warm}"; \
    git init -q .; \
    cp /tmp/fleet-precommit-hooks.yaml .pre-commit-config.yaml; \
    pre-commit install-hooks; \
    cd /; rm -rf "${warm}"; \
    chmod -R a+rX "${PRE_COMMIT_HOME}"; \
    test -d "${PRE_COMMIT_HOME}"

ARG GOLANGCI_LINT_VERSION=2.12.2
ARG TRUFFLEHOG_VERSION=3.96.0
ARG KDLFMT_VERSION=0.1.7
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${GOLANGCI_ARCH}.tar.gz" \
      -o /tmp/golangci-lint.tar.gz; \
    tar -xzf /tmp/golangci-lint.tar.gz -C /usr/local/bin --strip-components=1 \
      "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${GOLANGCI_ARCH}/golangci-lint"; \
    chmod 0755 /usr/local/bin/golangci-lint; \
    rm /tmp/golangci-lint.tar.gz; golangci-lint --version
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/trufflesecurity/trufflehog/releases/download/v${TRUFFLEHOG_VERSION}/trufflehog_${TRUFFLEHOG_VERSION}_linux_${TRUFFLEHOG_ARCH}.tar.gz" \
      -o /tmp/trufflehog.tar.gz; \
    tar -xzf /tmp/trufflehog.tar.gz -C /usr/local/bin trufflehog; \
    chmod 0755 /usr/local/bin/trufflehog; \
    rm /tmp/trufflehog.tar.gz; trufflehog --version
RUN set -eux; . /opt/agentic-os/arch.env; \
    curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      "https://github.com/hougesen/kdlfmt/releases/download/v${KDLFMT_VERSION}/kdlfmt-${KDL_ARCH}-unknown-linux-gnu.tar.xz" \
      -o /tmp/kdlfmt.tar.xz; \
    tar -xJf /tmp/kdlfmt.tar.xz -C /usr/local/bin --strip-components=1 \
      "kdlfmt-${KDL_ARCH}-unknown-linux-gnu/kdlfmt"; \
    chmod 0755 /usr/local/bin/kdlfmt; \
    rm /tmp/kdlfmt.tar.xz; kdlfmt --version

ARG RUST_PINNED_VERSIONS
# Prove each pinned toolchain is resident with the components and target a
# pinned consumer asks for - without the components rustup fetches them at
# first use, which is the download this bake exists to remove. The empty
# guard matters: an unplumbed arg would make the loop below a silent pass.
RUN node --version && go version && dotnet --list-sdks && cargo --version && \
    trunk --version && wasm-pack --version && wasm-opt --version && \
    python --version && pipenv --version && aosguard --version && \
    test -n "${RUST_PINNED_VERSIONS}" && \
    for pin in ${RUST_PINNED_VERSIONS}; do \
      cargo "+${pin}" --version && cargo "+${pin}" clippy --version && \
      cargo "+${pin}" fmt --version && \
      rustup target list --installed --toolchain "${pin}" | grep -qx wasm32-unknown-unknown; \
    done
WORKDIR /workspace
ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
CMD ["bash"]
