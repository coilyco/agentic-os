#!/usr/bin/env bash

set -euo pipefail

ubuntu_base="$(
  awk '$1 == "FROM" && $2 ~ /^ubuntu:/ { print $2; exit }' \
    docker/dev-base/Dockerfile
)"
if [ -z "$ubuntu_base" ]; then
  echo "dev-base Dockerfile has no Ubuntu parent" >&2
  exit 1
fi

language_targets="$(
  awk \
    '$1 == "FROM" && $2 ~ /^ubuntu:/ && $3 == "AS" && $4 ~ /^dev-base-lang-/ { print $4 }' \
    docker/dev-base/Dockerfile
)"
if [ -z "$language_targets" ]; then
  echo "dev-base Dockerfile has no language targets" >&2
  exit 1
fi

while IFS= read -r language_target; do
  docker buildx build \
    --check \
    --build-context aos-cli=aos \
    --target "$language_target" \
    --file docker/dev-base/Dockerfile \
    docker/dev-base
done <<<"$language_targets"

docker buildx build \
  --check \
  --build-arg "BASE_IMAGE=${ubuntu_base}" \
  --build-arg "LANG_GO_IMAGE=${ubuntu_base}" \
  --build-arg "LANG_DOTNET_IMAGE=${ubuntu_base}" \
  --build-arg "LANG_PYTHON_IMAGE=${ubuntu_base}" \
  --target dev-base-full \
  --file docker/dev-base/full/Dockerfile \
  docker/dev-base/full
