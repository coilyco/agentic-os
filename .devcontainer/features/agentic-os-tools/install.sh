#!/usr/bin/env bash
# Fleet tools for CI and the agent dev container. See docs/dev-container.md.
set -euo pipefail

DEFAULT_TOOLS="just uv pre-commit trufflehog golangci-lint kdlfmt yq umbra"
PRE_COMMIT_VERSION=${PRE_COMMIT_VERSION:-4.6.0}

pins() {
  cat <<'PINS'
just|1.58.0|amd64|https://github.com/casey/just/releases/download/1.58.0/just-1.58.0-x86_64-unknown-linux-musl.tar.gz|4a5cc2f53e6f0f8c59092a6cc38291eb729d46a7dd95d3ae582008881b84931d|just
just|1.58.0|arm64|https://github.com/casey/just/releases/download/1.58.0/just-1.58.0-aarch64-unknown-linux-musl.tar.gz|748237128c4c40cbdabc65e841d05ceba13cc23a91eaba395495894c1d9764df|just
uv|0.12.0|amd64|https://github.com/astral-sh/uv/releases/download/0.12.0/uv-x86_64-unknown-linux-gnu.tar.gz|eaf842262aa1c418d8ecc5605f02ee1ebfd369124fa48548e85f9481a47831a9|uv-x86_64-unknown-linux-gnu/uv uv-x86_64-unknown-linux-gnu/uvx
uv|0.12.0|arm64|https://github.com/astral-sh/uv/releases/download/0.12.0/uv-aarch64-unknown-linux-gnu.tar.gz|2c5d6e3092cc5223b10ff403880cc75121bf64e84644e7a0c69f643b0d89ac95|uv-aarch64-unknown-linux-gnu/uv uv-aarch64-unknown-linux-gnu/uvx
trufflehog|3.96.0|amd64|https://github.com/trufflesecurity/trufflehog/releases/download/v3.96.0/trufflehog_3.96.0_linux_amd64.tar.gz|7105f1cd6577f058a9e39d0578f1a99c8a1e481e4d3512cd8a09acfe22a0fdc0|trufflehog
trufflehog|3.96.0|arm64|https://github.com/trufflesecurity/trufflehog/releases/download/v3.96.0/trufflehog_3.96.0_linux_arm64.tar.gz|50acd4c7a3b8ebfe5083d8350956057030c44be3515dedd55b45263495c490b2|trufflehog
golangci-lint|2.12.2|amd64|https://github.com/golangci/golangci-lint/releases/download/v2.12.2/golangci-lint-2.12.2-linux-amd64.tar.gz|8df580d2670fed8fa984aac0507099af8df275e665215f5c7a2ae3943893a553|golangci-lint-2.12.2-linux-amd64/golangci-lint
golangci-lint|2.12.2|arm64|https://github.com/golangci/golangci-lint/releases/download/v2.12.2/golangci-lint-2.12.2-linux-arm64.tar.gz|44cd40a8c76c86755375adfeea52cfd3533cb43d7bd647771e0ae065e166df3a|golangci-lint-2.12.2-linux-arm64/golangci-lint
kdlfmt|0.1.7|amd64|https://github.com/hougesen/kdlfmt/releases/download/v0.1.7/kdlfmt-x86_64-unknown-linux-gnu.tar.xz|b1308797646f4e4cbe254c5eb27779bf6450e5386e2644acc46c5b7316783ab9|kdlfmt-x86_64-unknown-linux-gnu/kdlfmt
kdlfmt|0.1.7|arm64|https://github.com/hougesen/kdlfmt/releases/download/v0.1.7/kdlfmt-aarch64-unknown-linux-gnu.tar.xz|2ddbcc630e6a8d93b900fb5252b5182736064fc03f3876fbf8544b9d94f0fb70|kdlfmt-aarch64-unknown-linux-gnu/kdlfmt
yq|4.53.3|amd64|https://github.com/mikefarah/yq/releases/download/v4.53.3/yq_linux_amd64|fa52a4e758c63d38299163fbdd1edfb4c4963247918bf9c1c5d31d84789eded4|-
yq|4.53.3|arm64|https://github.com/mikefarah/yq/releases/download/v4.53.3/yq_linux_arm64|578648e463a11c1b6db6010cbf41eafed6bee79466fcffa1bb446672cf7945ea|-
umbra|0.256.0|amd64|https://github.com/coilyco/umbra/releases/download/v0.256.0/umbra-linux-amd64|6e85573e2f5bb61031e600523c5b2fc3f1afcd98cc5edce8a3a424486db3b668|-
umbra|0.256.0|arm64|https://github.com/coilyco/umbra/releases/download/v0.256.0/umbra-linux-arm64|9a4776415783f23879e1d3027619fc3f907879cfb155fe5be7f423986ffb464c|-
codex|0.146.0|amd64|https://github.com/openai/codex/releases/download/rust-v0.146.0/codex-x86_64-unknown-linux-musl.tar.gz|5ba3b9405543953081f661d0854d266f76e2abbe51d41349355a36de7673776a|codex-x86_64-unknown-linux-musl>codex
codex|0.146.0|arm64|https://github.com/openai/codex/releases/download/rust-v0.146.0/codex-aarch64-unknown-linux-musl.tar.gz|975bac91562abeedeb8f79636d51a86649b31f34a9de6a3bcb059565b6cf1f87|codex-aarch64-unknown-linux-musl>codex
goose|1.45.0|amd64|https://github.com/block/goose/releases/download/v1.45.0/goose-x86_64-unknown-linux-musl.tar.gz|d7aac29ae244ac5a16ea97b205ff463d63a5525f5791d3581d2cea2fc9840d59|goose
goose|1.45.0|arm64|https://github.com/block/goose/releases/download/v1.45.0/goose-aarch64-unknown-linux-musl.tar.gz|91d56b48abbf6869c6c2e2a47491e085dbde537a3c39b04d97f07291cdaec18c|goose
PINS
}

host_arch() {
  case "${ARCH:-$(uname -m)}" in
    x86_64 | amd64) echo amd64 ;;
    aarch64 | arm64) echo arm64 ;;
    *) echo "unsupported architecture: ${ARCH:-$(uname -m)}" >&2; return 1 ;;
  esac
}

pin_for() {
  local tool=$1 arch=$2 line
  line=$(pins | awk -F'|' -v t="$tool" -v a="$arch" '$1 == t && $3 == a { print; exit }')
  [ -n "$line" ] || { echo "no pin for $tool on $arch" >&2; return 1; }
  echo "$line"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

fetch_verified() {
  local url=$1 want=$2 dest=$3 got
  curl --retry 5 --retry-all-errors --retry-delay 2 -fsSL "$url" -o "$dest"
  got=$(sha256_of "$dest")
  if [ "$got" != "$want" ]; then
    rm -f "$dest"
    echo "checksum mismatch for $url: want $want, got $got" >&2
    return 1
  fi
}

already_installed() {
  local tool=$1 version=$2
  command -v "$INSTALL_DIR/$tool" >/dev/null 2>&1 || return 1
  "$INSTALL_DIR/$tool" --version 2>&1 | grep -Fq "$version"
}

install_pinned() {
  local tool=$1 arch version url sha members work member
  arch=$(host_arch)
  IFS='|' read -r _ version _ url sha members < <(pin_for "$tool" "$arch")
  if [ -n "${PLAN:-}" ]; then
    printf '%s\t%s\t%s\t%s\t%s\n' "$tool" "$version" "$arch" "$url" "$sha"
    return 0
  fi
  if [ -z "${VERIFY_ONLY:-}" ] && already_installed "$tool" "$version"; then
    echo "$tool $version already installed"
    return 0
  fi
  work=$(mktemp -d "${TMPDIR:-/tmp}/fleet-tools.XXXXXX")
  fetch_verified "$url" "$sha" "$work/asset"
  if [ -n "${VERIFY_ONLY:-}" ]; then
    echo "$tool $version $arch verified"
    rm -rf "$work"
    return 0
  fi
  mkdir -p "$INSTALL_DIR"
  if [ "$members" = "-" ]; then
    install -m 0755 "$work/asset" "$INSTALL_DIR/$tool"
  else
    case "$url" in
      *.tar.xz) tar -xJf "$work/asset" -C "$work" ;;
      *) tar -xzf "$work/asset" -C "$work" ;;
    esac
    for member in $members; do
      case "$member" in
        *'>'*) install -m 0755 "$work/${member%%>*}" "$INSTALL_DIR/${member#*>}" ;;
        *) install -m 0755 "$work/$member" "$INSTALL_DIR/${member##*/}" ;;
      esac
    done
  fi
  rm -rf "$work"
  echo "installed $tool $version"
}

install_pre_commit() {
  if [ -n "${PLAN:-}" ]; then
    printf 'pre-commit\t%s\t%s\tpypi\t-\n' "$PRE_COMMIT_VERSION" "$(host_arch)"
    return 0
  fi
  [ -z "${VERIFY_ONLY:-}" ] || { echo "pre-commit $PRE_COMMIT_VERSION skipped (PyPI)"; return 0; }
  if [ "$(PATH="$INSTALL_DIR:$PATH" pre-commit --version 2>/dev/null | cut -d' ' -f2)" = "$PRE_COMMIT_VERSION" ]; then
    echo "pre-commit $PRE_COMMIT_VERSION already installed"
    return 0
  fi
  PATH="$INSTALL_DIR:$PATH" UV_TOOL_BIN_DIR="${UV_TOOL_BIN_DIR:-$INSTALL_DIR}" \
    uv tool install --force "pre-commit==$PRE_COMMIT_VERSION"
  echo "installed pre-commit $PRE_COMMIT_VERSION"
}

share_uv_dirs() {
  export UV_TOOL_DIR=/opt/uv/tools UV_TOOL_BIN_DIR=$INSTALL_DIR UV_PYTHON_INSTALL_DIR=/opt/uv/python
  mkdir -p "$UV_TOOL_DIR" "$UV_PYTHON_INSTALL_DIR"
}

require_linux() {
  [ "$(uname -s)" = Linux ] || { echo "only Linux installs are supported, not $(uname -s)" >&2; return 1; }
}

main() {
  local tools tool
  [ -n "${PLAN:-}${VERIFY_ONLY:-}" ] || require_linux
  INSTALL_DIR=${INSTALL_DIR:-/usr/local/bin}
  tools=$(echo "${TOOLS:-$DEFAULT_TOOLS}" | tr ',' ' ')
  [ "${SHAREDUV:-false}" != "true" ] || share_uv_dirs
  for tool in $tools; do
    case "$tool" in
      pre-commit) install_pre_commit ;;
      *) install_pinned "$tool" ;;
    esac
  done
  [ "${SHAREDUV:-false}" != "true" ] || [ -n "${PLAN:-}${VERIFY_ONLY:-}" ] || chmod -R a+rwX /opt/uv
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
