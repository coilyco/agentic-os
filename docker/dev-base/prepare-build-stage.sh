#!/usr/bin/env bash
#
# Per-stage build setup: the architecture map, and apt's retry policy.
# Runs first in every language stage, before that stage's apt-get.

set -euo pipefail

# Every curl in the Dockerfile retries; apt-get did not, and an apt mirror blip
# failed one payload and skipped the whole release. See agentic-os#987.
install -d /etc/apt/apt.conf.d
printf '%s\n' \
  'Acquire::Retries "5";' \
  'Acquire::http::Timeout "30";' \
  'Acquire::https::Timeout "30";' \
  > /etc/apt/apt.conf.d/80-agentic-os-retries

case "${TARGETARCH:?TARGETARCH is required}" in
  amd64)
    AWS_ARCH=x86_64
    BINARYEN_ARCH=x86_64
    CODEX_ARCH=x86_64
    DOCKER_ARCH=x86_64
    DOTNET_ARCH=x64
    GH_ARCH=amd64
    GO_ARCH=amd64
    GOLANGCI_ARCH=amd64
    GOOSE_ARCH=x86_64
    HELM_ARCH=amd64
    JUST_ARCH=x86_64
    KDL_ARCH=x86_64
    KUBECTL_ARCH=amd64
    NODE_ARCH=x64
    TRUFFLEHOG_ARCH=amd64
    TS_ARCH=amd64
    WASM_BINDGEN_ARCH=x86_64
    WASM_PACK_ARCH=x86_64
    YQ_ARCH=amd64
    ;;
  arm64)
    AWS_ARCH=aarch64
    BINARYEN_ARCH=aarch64
    CODEX_ARCH=aarch64
    DOCKER_ARCH=aarch64
    DOTNET_ARCH=arm64
    GH_ARCH=arm64
    GO_ARCH=arm64
    GOLANGCI_ARCH=arm64
    GOOSE_ARCH=aarch64
    HELM_ARCH=arm64
    JUST_ARCH=aarch64
    KDL_ARCH=aarch64
    KUBECTL_ARCH=arm64
    NODE_ARCH=arm64
    TRUFFLEHOG_ARCH=arm64
    TS_ARCH=arm64
    WASM_BINDGEN_ARCH=aarch64
    WASM_PACK_ARCH=aarch64
    YQ_ARCH=arm64
    ;;
  *)
    echo "unsupported TARGETARCH: ${TARGETARCH}" >&2
    exit 1
    ;;
esac

install -d /opt/agentic-os
printf '%s\n' \
  "AWS_ARCH=${AWS_ARCH}" \
  "BINARYEN_ARCH=${BINARYEN_ARCH}" \
  "CODEX_ARCH=${CODEX_ARCH}" \
  "DOCKER_ARCH=${DOCKER_ARCH}" \
  "DOTNET_ARCH=${DOTNET_ARCH}" \
  "GH_ARCH=${GH_ARCH}" \
  "GO_ARCH=${GO_ARCH}" \
  "GOLANGCI_ARCH=${GOLANGCI_ARCH}" \
  "GOOSE_ARCH=${GOOSE_ARCH}" \
  "HELM_ARCH=${HELM_ARCH}" \
  "JUST_ARCH=${JUST_ARCH}" \
  "KDL_ARCH=${KDL_ARCH}" \
  "KUBECTL_ARCH=${KUBECTL_ARCH}" \
  "NODE_ARCH=${NODE_ARCH}" \
  "TRUFFLEHOG_ARCH=${TRUFFLEHOG_ARCH}" \
  "TS_ARCH=${TS_ARCH}" \
  "WASM_BINDGEN_ARCH=${WASM_BINDGEN_ARCH}" \
  "WASM_PACK_ARCH=${WASM_PACK_ARCH}" \
  "YQ_ARCH=${YQ_ARCH}" \
  > /opt/agentic-os/arch.env
