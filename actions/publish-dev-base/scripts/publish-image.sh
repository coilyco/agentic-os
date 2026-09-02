#!/usr/bin/env bash
# Build one explicit image tier or promote one existing manifest.

set -euo pipefail

tier_tag() {
  if [ "$1" = full ]; then
    printf '%s' "$2"
  else
    printf '%s-%s' "$1" "$2"
  fi
}

# Read from the payload Dockerfile's own default rather than restating it, so
# the full image can only ever verify the pins the rust payload actually baked.
rust_pins="$(sed -n 's/^ARG RUST_PINNED_VERSIONS="\(.*\)"$/\1/p' docker/dev-base/Dockerfile)"
if [ -z "$rust_pins" ]; then
  echo "::error::no RUST_PINNED_VERSIONS default in docker/dev-base/Dockerfile" >&2
  exit 2
fi

target_ref="${IMAGE_BASE}:$(tier_tag "$TIER" "$TAG")"
read -r -a aliases <<< "${ALIAS:-} ${EXTRA_ALIASES:-}"
target_args=(-t "$target_ref")
for moving_tag in "${aliases[@]}"; do
  [ -n "$moving_tag" ] || continue
  target_args+=(-t "${IMAGE_BASE}:$(tier_tag "$TIER" "$moving_tag")")
done

if [ "$MODE" = build ]; then
  cache_ref="${IMAGE_BASE}:$(tier_tag "$TIER" buildcache)"
  command=(
    docker buildx build
    --progress=plain
    --push
    --platform "$PLATFORMS"
    --cache-from "type=registry,ref=${cache_ref}"
    --cache-to "type=registry,ref=${cache_ref},mode=max,ignore-error=true"
    "${target_args[@]}"
  )
  if [[ "$TIER" = lang-* ]]; then
    command+=(
      --target "dev-base-${TIER}"
      --file docker/dev-base/Dockerfile
      docker/dev-base
    )
  elif [ "$TIER" = full ]; then
    command+=(
      --build-arg "RUST_PINNED_VERSIONS=${rust_pins}"
      --build-arg "BASE_IMAGE=${IMAGE_BASE}:lang-rust-${TAG}"
      --build-arg "LANG_NODE_IMAGE=${IMAGE_BASE}:lang-node-${TAG}"
      --build-arg "LANG_GO_IMAGE=${IMAGE_BASE}:lang-go-${TAG}"
      --build-arg "LANG_DOTNET_IMAGE=${IMAGE_BASE}:lang-dotnet-${TAG}"
      --build-arg "LANG_PYTHON_IMAGE=${IMAGE_BASE}:lang-python-${TAG}"
      --build-context aosguard-spec=.umbra
      --build-context aosguard-python=agentic_os
      --build-context repo-lists=aos-cli/repositories
      --target dev-base-full
      --file docker/dev-base/full/Dockerfile
      docker/dev-base
    )
  else
    echo "::error::unknown dev-base tier: ${TIER}" >&2
    exit 2
  fi
elif [ "$MODE" = promote ]; then
  if [ "$TIER" != full ]; then
    echo "::error::only the full dev-base image is a promotable release product" >&2
    exit 2
  fi
  if [ -z "${SOURCE_TAG:-}" ]; then
    echo "::error::source-tag is required in promote mode" >&2
    exit 2
  fi
  source_ref="${IMAGE_BASE}:$(tier_tag "$TIER" "$SOURCE_TAG")"
  command=(docker buildx imagetools create "${target_args[@]}" "$source_ref")
else
  echo "::error::unknown publish mode: ${MODE}" >&2
  exit 2
fi

timeout --preserve-status --kill-after=5m "$BUILD_TIMEOUT" "${command[@]}"
