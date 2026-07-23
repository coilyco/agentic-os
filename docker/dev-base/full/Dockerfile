ARG BASE_IMAGE
ARG LANG_GO_IMAGE
ARG LANG_DOTNET_IMAGE
ARG LANG_PYTHON_IMAGE
FROM ${LANG_GO_IMAGE} AS dev-base-lang-go-graft
FROM ${LANG_DOTNET_IMAGE} AS dev-base-lang-dotnet-graft
FROM ${LANG_PYTHON_IMAGE} AS dev-base-lang-python-graft
FROM ${BASE_IMAGE} AS dev-base-full
ARG GOLANGCI_LINT_VERSION=2.12.2
ARG TRUFFLEHOG_VERSION=3.95.8
ARG KDLFMT_VERSION=0.1.7

# Toolchain grafts. full inherits lang-rust (and core's Node/shared tools),
# then adds the remaining parallel language specialists.
COPY --from=dev-base-lang-go-graft /usr/local/go /usr/local/go
COPY --from=dev-base-lang-dotnet-graft /usr/local/dotnet /usr/local/dotnet
COPY --from=dev-base-lang-python-graft /opt/uv/tools/pipenv /opt/uv/tools/pipenv
COPY --from=dev-base-lang-python-graft /usr/local/bin/python /usr/local/bin/python
COPY --from=dev-base-lang-python-graft /usr/local/bin/pip /usr/local/bin/pip
COPY --from=dev-base-lang-python-graft /usr/local/bin/pipenv /usr/local/bin/pipenv
RUN set -eux; \
    node --version; \
    go version; \
    dotnet --list-sdks; \
    cargo --version; \
    trunk --version; \
    rustup target list --installed | grep -q wasm32-unknown-unknown; \
    python --version; \
    pipenv --version

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

WORKDIR /workspace
CMD ["bash"]
