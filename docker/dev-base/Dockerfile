# Canonical agent dev environment, published per release as the dev-base image. See docs/dev-base-image.md.
FROM ubuntu:24.04

# Pinned toolchain. Auto-bumped against upstream by .forgejo/workflows/dep-bump.yml
# (one commit per tool); edit an ARG by hand to pin or roll back. See docs/dev-base-image.md.
ARG UV_VERSION=0.11.26
ARG NODE_VERSION=22.23.1
ARG GO_VERSION=1.26.4
ARG AWSCLI_VERSION=2.35.15
ARG CLAUDE_VERSION=2.1.200
ARG MCPORTER_VERSION=0.12.3
ARG CODEX_VERSION=0.142.5
ARG GOOSE_VERSION=1.41.0
ARG DOCKER_VERSION=28.5.2
ARG TAILSCALE_VERSION=1.98.8
ARG WARD_VERSION=0.396.0
# .NET SDK for in-container C# compilation (see docs/dev-base-image.md). Pinned to the .NET 10
# LTS line eco-app's Eco mods build against; auto-bump stays on the 10.x channel
# (like NODE_VERSION stays on its major) so a mod never faces a surprise major jump.
ARG DOTNET_VERSION=10.0.301

# Pre-commit / CI gate tools (see docs/dev-base-image.md). golangci-lint and kdlfmt are
# hand-pinned to the versions the consuming repos' CI pins - cli-guard and ward
# both pin golangci-lint v2.12.2, ward pins kdlfmt v0.1.7 - because their lint /
# format output is version-sensitive: an image bumping them independently would
# re-drift the in-container gate from CI, the exact failure this closes. They are
# deliberately left out of dep-bump's auto-bump for that reason (no resolver in
# scripts/dep-bump.py). trufflehog tracks upstream latest, since CI installs it
# unpinned (cli-guard's secrets job curls trufflehog's `main` install.sh), so
# latest is the match - it stays in the auto-bump set.
ARG GOLANGCI_LINT_VERSION=2.12.2
ARG TRUFFLEHOG_VERSION=3.95.8
ARG KDLFMT_VERSION=0.1.7
# Default runtime user for the surface path. ward can still override these when
# it builds a different surface image, but the published dev-base seeds the
# home app dir for the standard agent user so the first audit preflight does not
# race a root-owned mkdir.
ARG WARD_AGENT_UID=1000
ARG WARD_AGENT_GID=1000

# buildx populates TARGETARCH (amd64|arm64) per platform in the manifest list.
ARG TARGETARCH

# uv tools land on the global PATH so any run-as-uid finds pre-commit.
ENV DEBIAN_FRONTEND=noninteractive \
    PATH=/usr/local/go/bin:/usr/local/dotnet:/usr/local/bin:/usr/bin:/bin \
    UV_TOOL_DIR=/opt/uv/tools \
    UV_TOOL_BIN_DIR=/usr/local/bin \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python \
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
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl wget git gnupg jq ripgrep less unzip xz-utils socat \
      build-essential openssh-client procps shellcheck \
      python3 python3-pip python3-venv \
      libicu74 \
 && rm -rf /var/lib/apt/lists/*

# Map Docker's TARGETARCH onto each upstream's arch spelling once.
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) NODE_ARCH=x64;   GO_ARCH=amd64; AWS_ARCH=x86_64; CODEX_ARCH=x86_64; GOOSE_ARCH=x86_64; DOCKER_ARCH=x86_64; TS_ARCH=amd64; GOLANGCI_ARCH=amd64; TRUFFLEHOG_ARCH=amd64; KDL_ARCH=x86_64; DOTNET_ARCH=x64 ;; \
      arm64) NODE_ARCH=arm64; GO_ARCH=arm64; AWS_ARCH=aarch64; CODEX_ARCH=aarch64; GOOSE_ARCH=aarch64; DOCKER_ARCH=aarch64; TS_ARCH=arm64; GOLANGCI_ARCH=arm64; TRUFFLEHOG_ARCH=arm64; KDL_ARCH=aarch64; DOTNET_ARCH=arm64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    echo "NODE_ARCH=${NODE_ARCH} GO_ARCH=${GO_ARCH} AWS_ARCH=${AWS_ARCH} CODEX_ARCH=${CODEX_ARCH} GOOSE_ARCH=${GOOSE_ARCH} DOCKER_ARCH=${DOCKER_ARCH} TS_ARCH=${TS_ARCH} GOLANGCI_ARCH=${GOLANGCI_ARCH} TRUFFLEHOG_ARCH=${TRUFFLEHOG_ARCH} KDL_ARCH=${KDL_ARCH} DOTNET_ARCH=${DOTNET_ARCH}" > /tmp/arch.env

# uv (Python project + tool manager).
RUN set -eux; \
    curl -fsSL "https://astral.sh/uv/${UV_VERSION}/install.sh" \
      | env INSTALLER_NO_MODIFY_PATH=1 UV_INSTALL_DIR=/usr/local/bin sh; \
    uv --version

# pre-commit, as a uv tool on the global bin dir.
RUN set -eux; \
    uv tool install "pre-commit"; \
    chmod -R a+rX /opt/uv; \
    pre-commit --version

# Seed the default surface home path before ward starts. ward's audit preflight
# runs against $HOME/.ward, so the path has to exist with the runtime agent's
# ownership before the root bootstrap reaches `MkdirAll` and freezes it that way.
RUN set -eux; \
    mkdir -p /home/ubuntu; \
    chmod 0755 /home/ubuntu; \
    install -d -o "${WARD_AGENT_UID}" -g "${WARD_AGENT_GID}" -m 0700 /home/ubuntu/.ward

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

# Node + npm (Claude Code's runtime).
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
      -o /tmp/node.tar.xz; \
    tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 \
      --exclude='*/CHANGELOG.md' --exclude='*/README.md' --exclude='*/LICENSE'; \
    rm /tmp/node.tar.xz; \
    node --version; npm --version

# Go (builds the warp module's pre-commit hooks and, later, ward).
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" \
      -o /tmp/go.tar.gz; \
    tar -xzf /tmp/go.tar.gz -C /usr/local; \
    rm /tmp/go.tar.gz; \
    go version

# .NET SDK, pinned. dev-base shipped no C# toolchain, so any repo
# with C# to compile (eco-app's Eco mods, `dotnet build mods/*`) had to hand-install
# the SDK mid-run - the same absent-toolchain class as the uv/Python fix. Bake
# it in with Microsoft's official dotnet-install.sh at the exact pinned SDK version so
# `dotnet build` just works, no per-run install. --no-path since ENV owns DOTNET_ROOT
# and PATH; chmod a+rX so any run-as-uid runs it like every other tool here.
# Globalization: full ICU (libicu74 in the base apt block above), NOT invariant mode,
# so C# culture/string behaviour matches a normal host - no DOTNET_SYSTEM_GLOBALIZATION_INVARIANT.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://dot.net/v1/dotnet-install.sh" -o /tmp/dotnet-install.sh; \
    chmod 0755 /tmp/dotnet-install.sh; \
    /tmp/dotnet-install.sh --version "${DOTNET_VERSION}" --architecture "${DOTNET_ARCH}" \
      --install-dir /usr/local/dotnet --no-path; \
    rm /tmp/dotnet-install.sh; \
    chmod -R a+rX /usr/local/dotnet; \
    dotnet --version; dotnet --list-sdks

# golangci-lint (Go linter for ward + cli-guard). Closes the missing-binary gap
# that made the in-container `pre-commit run --all-files` / CI lint job unrunnable
# for agents. Release tag is v-prefixed; the tarball name and its
# top-level dir are not. Pinned to the consuming repos' CI version (see the ARG).
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${GOLANGCI_ARCH}.tar.gz" \
      -o /tmp/golangci-lint.tar.gz; \
    tar -xzf /tmp/golangci-lint.tar.gz -C /usr/local/bin --strip-components=1 \
      "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${GOLANGCI_ARCH}/golangci-lint"; \
    chmod 0755 /usr/local/bin/golangci-lint; \
    rm /tmp/golangci-lint.tar.gz; \
    golangci-lint --version

# trufflehog (secret scanner for the catalog suite's offline trufflehog hook).
# The static Go binary sits at the tarball root; extract just it.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/trufflesecurity/trufflehog/releases/download/v${TRUFFLEHOG_VERSION}/trufflehog_${TRUFFLEHOG_VERSION}_linux_${TRUFFLEHOG_ARCH}.tar.gz" \
      -o /tmp/trufflehog.tar.gz; \
    tar -xzf /tmp/trufflehog.tar.gz -C /usr/local/bin trufflehog; \
    chmod 0755 /usr/local/bin/trufflehog; \
    rm /tmp/trufflehog.tar.gz; \
    trufflehog --version

# kdlfmt (KDL formatter for ward-kdl guardfiles, via ward's language:system hook).
# cargo-dist ships a .tar.xz with the binary under a target-triple dir; only the
# gnu build covers both arches (no aarch64 musl), and ubuntu's glibc runs it.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/hougesen/kdlfmt/releases/download/v${KDLFMT_VERSION}/kdlfmt-${KDL_ARCH}-unknown-linux-gnu.tar.xz" \
      -o /tmp/kdlfmt.tar.xz; \
    tar -xJf /tmp/kdlfmt.tar.xz -C /usr/local/bin --strip-components=1 \
      "kdlfmt-${KDL_ARCH}-unknown-linux-gnu/kdlfmt"; \
    chmod 0755 /usr/local/bin/kdlfmt; \
    rm /tmp/kdlfmt.tar.xz; \
    kdlfmt --version

# AWS CLI v2 (SSM secret loader + ~/.aws passthrough).
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${AWS_ARCH}-${AWSCLI_VERSION}.zip" \
      -o /tmp/awscli.zip; \
    unzip -q /tmp/awscli.zip -d /tmp; \
    /tmp/aws/install --bin-dir /usr/local/bin --install-dir /usr/local/aws-cli; \
    rm -rf /tmp/awscli.zip /tmp/aws; \
    aws --version

# Claude Code, pinned.
RUN set -eux; \
    npm install -g "@anthropic-ai/claude-code@${CLAUDE_VERSION}"; \
    claude --version

# MCPorter, pinned. Warded Codex containers shell out to this on demand for
# one-off MCP calls and schema refreshes, so it has to be baked into the image
# rather than pulled ad hoc at runtime.
RUN set -eux; \
    timeout 5m npm install -g "mcporter@${MCPORTER_VERSION}"; \
    timeout 1m mcporter --help >/dev/null

# Codex CLI, pinned. ward injects host ~/.codex/auth.json at container launch.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION}/codex-${CODEX_ARCH}-unknown-linux-musl.tar.gz" \
      -o /tmp/codex.tar.gz; \
    tar -xzf /tmp/codex.tar.gz -C /usr/local/bin; \
    mv "/usr/local/bin/codex-${CODEX_ARCH}-unknown-linux-musl" /usr/local/bin/codex; \
    chmod 0755 /usr/local/bin/codex; \
    rm /tmp/codex.tar.gz; \
    codex --version

# Goose CLI (block/goose), pinned. Statically linked musl build, no runtime deps.
# Unblocks the `ward agent goose work|headless|task|reply` surface the ward
# entrypoint already wires (it dropped to a shell while the binary was absent).
# goose floats over a model provider it reads from ~/.config/goose; wiring that
# into the container is ward's concern (the adapter manifest's goose `auth` slot).
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/block/goose/releases/download/v${GOOSE_VERSION}/goose-${GOOSE_ARCH}-unknown-linux-musl.tar.gz" \
      -o /tmp/goose.tar.gz; \
    tar -xzf /tmp/goose.tar.gz -C /usr/local/bin ./goose; \
    chmod 0755 /usr/local/bin/goose; \
    rm /tmp/goose.tar.gz; \
    goose --version

# Docker CLI (client only, no daemon). ward's read-only `explore` session binds the
# host docker socket so the in-container agent can dispatch a sibling `warded #N`
# headless run; that needs a `docker` client in the image. The static
# tarball ships the full engine - extract only the client binary.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
      -o /tmp/docker.tgz; \
    tar -xzf /tmp/docker.tgz -C /usr/local/bin --strip-components=1 docker/docker; \
    chmod 0755 /usr/local/bin/docker; \
    rm /tmp/docker.tgz; \
    docker --version

# Tailscale CLI (client only, no daemon). ward containers reach the tower
# (kai-tower-3026 ollama on the tailnet) over the tailnet; the image shipped no
# tailnet client, so an agent could not query tailnet status or route to the
# tower. The static tarball ships the full engine - extract only
# the `tailscale` client binary. This makes the CLI *exist*; bringing up the
# tailnet (auth) is a separate axis ward owns, not baked into the image.
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://pkgs.tailscale.com/stable/tailscale_${TAILSCALE_VERSION}_${TS_ARCH}.tgz" \
      -o /tmp/tailscale.tgz; \
    tar -xzf /tmp/tailscale.tgz -C /usr/local/bin --strip-components=1 \
      "tailscale_${TAILSCALE_VERSION}_${TS_ARCH}/tailscale"; \
    chmod 0755 /usr/local/bin/tailscale; \
    rm /tmp/tailscale.tgz; \
    tailscale version

# ward, built from source at a pinned tag. ward is the whole
# dev-command surface agents route through (`ward <verb>`); baking it in makes the
# image batteries-included instead of leaving every warded run to `go install` it.
# Go is already in the base, so this compiles in place - one RUN, source and build
# caches cleaned in the same layer like every other tool here.
#
# NO build token. ward was deferred because it was assumed to need a cross-repo
# clone secret, but coilyco-flight-deck/ward and its one private-domain module
# dep (coilyco-flight-deck/cli-guard) are both public now, so an anonymous clone
# plus GOPROXY=direct resolves everything. GOPRIVATE keeps the cli-guard fetch off
# the public proxy (it lives on forgejo, not proxy.golang.org); GOSUMDB=off since
# that domain has no sumdb entry. The only network-flaky step is the direct git
# fetch of cli-guard's module, so `go mod download` gets ward's own bounded retry
# (its release.yml does the same) to keep a transient hang from shipping a
# ward-less image. -X main.Version stamps the tag so `ward --version` is truthful.
#
# coily is deliberately NOT shipped. The title paired ward + coily, but coily
# was retired: its `coily ops` surface moved into ward-kdl (`ward
# ops <area>`) and coilyco-bridge/coily is archived. Baking a dead binary in would
# reintroduce a retired command, so ward is the whole deliverable. Pinned as an
# ARG like the other agent CLIs and auto-bumped against ward's latest release by
# scripts/dep-bump.py (see docs/dev-base-auto-bump.md).
RUN set -eux; \
    export GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache \
      GOPROXY=direct GOSUMDB=off GOPRIVATE=forgejo.coilysiren.me CGO_ENABLED=0; \
    git clone --depth 1 --branch "v${WARD_VERSION}" \
      https://forgejo.coilysiren.me/coilyco-flight-deck/ward.git /tmp/ward-src; \
    cd /tmp/ward-src; \
    for attempt in 1 2 3; do \
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

# Agent self-name + status-line composer. Warded
# explorers / headless agents run in this image and otherwise get no name - host
# sessions self-name via a Claude Code SessionStart + statusLine hook the
# claude-hooks ansible role wires into ~/.claude/settings.json, but that role
# never runs in a container. Bake the slice in instead: the trimmed agent-name.sh
# under /opt (world-readable, so it runs as any uid like every other tool here),
# the statusline.sh composer plus its statusline.d/ provider dir, and a
# policy-tier managed-settings.json at the fixed /etc/claude-code path. Policy
# settings are the one Claude Code surface whose location does not depend on the
# run-as-uid, HOME, or CLAUDE_CONFIG_DIR ward picks - exactly right for an image
# that bakes in no user. The statusLine points at the composer, which runs every
# provider in statusline.d/ (10-agent-name, 20-repos, plus any user/repo overlay)
# so a warded container shows the full segment-composed line and new providers
# ride the next image build to all containers at once. See docs/statusline.md and
# docs/dev-base-self-name.md.
COPY agent-name.sh /opt/agentic-os/agent-name.sh
COPY statusline.sh /opt/agentic-os/statusline.sh
COPY statusline.d/ /opt/agentic-os/statusline.d/
COPY claude-managed-settings.json /etc/claude-code/managed-settings.json
RUN set -eux; \
    chmod 0755 /opt/agentic-os/agent-name.sh /opt/agentic-os/statusline.sh; \
    chmod 0755 /opt/agentic-os/statusline.d/*.sh; \
    chmod 0644 /etc/claude-code/managed-settings.json

# Substrate seed: bare mirrors of the public (image-tier) reference repos a ward
# container warms its gitcache from on a cold volume. List is a denormalized copy
# of the image-tier subset of ward's cmd/ward/containerassets/preclone-repos.txt.
# All public read (anonymous clone). Bridge/private repos are deliberately not
# baked into this shareable image. Mirror dir name matches ward's owner__name.git.
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

WORKDIR /workspace
CMD ["bash"]
