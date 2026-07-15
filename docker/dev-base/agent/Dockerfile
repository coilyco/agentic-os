ARG BASE_IMAGE
ARG LANG_NODE_IMAGE
FROM ${LANG_NODE_IMAGE} AS dev-base-lang-node-graft
FROM ${BASE_IMAGE} AS dev-base-agent
ARG CLAUDE_VERSION=2.1.206
ARG MCPORTER_VERSION=0.12.3
ARG CODEX_VERSION=0.144.1
ARG GOOSE_VERSION=1.41.0
ARG OPENCODE_VERSION=1.17.18

# Node toolchain graft. agent sits on ops (core + ops CLIs, no language
# tiers) since the aos#491 fan-out, and Claude Code / mcporter / opencode all
# ride Node, so graft the sibling lang-node tier's self-contained
# /usr/local/node prefix in. core's ENV PATH already carries its bin dir, and
# npm's global prefix follows the node binary, so the npm installs below land
# inside the grafted directory.
COPY --from=dev-base-lang-node-graft /usr/local/node /usr/local/node
RUN set -eux; \
    node --version; npm --version

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

# OpenCode, pinned. ward's `--harness opencode` path expects the binary to be
# present already, so bake it into the image instead of downloading it during
# engineer startup. The npm package owns the platform split for the optional
# native payloads, and the global install lands the `opencode` command on PATH.
RUN set -eux; \
    npm_config_cache=/tmp/npm-cache timeout 10m npm install -g "opencode-ai@${OPENCODE_VERSION}"; \
    rm -rf /tmp/npm-cache; \
    opencode --version

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

# Bake the deployment bot identity into git's system config so every user on
# the image inherits it without a SessionStart write. The runtime hook keeps a
# fallback path for older or custom images that do not carry this baked config.
RUN set -eux; \
    git config --system user.name "${WARD_GIT_NAME:-coilyco-ops}"; \
    git config --system user.email "${WARD_GIT_EMAIL:-coilyco-ops@coilysiren.me}"; \
    git config --system --list --show-origin | grep -E 'file:/etc/gitconfig[[:space:]]+user\.(name|email)='

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
#
# Seed the ward app dir with the non-root surface ownership up front. The first
# root bootstrap in a fresh director/read-only surface preflights `~/.ward/audit`,
# so this directory must already belong to the uid 1000 surface user or the audit
# mkdir lands root-owned and blocks later `warded` runs.
RUN set -eux; \
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

# Substrate seed: bare mirrors of the public (image-tier) reference repos a ward
# container warms its gitcache from on a cold volume. The list is canonical in
# docker/dev-base/substrate-image-repos.txt and the runtime ward manifest
# denormalizes that image-tier subset into cmd/ward/containerassets/preclone-repos.txt.
# All public read (anonymous clone). Non-profile bridge/private repos are
# deliberately not baked into this shareable image. Mirror dir name matches
# ward's owner__name.git.
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

ENTRYPOINT ["/opt/agentic-os/ward-shell-entrypoint.sh"]
