#!/usr/bin/env bash

set -euo pipefail

docker buildx build \
  --check \
  --build-arg WARD_CONFIG_REF_COMMIT=0000000000000000000000000000000000000000 \
  --build-context aos-cli=aos \
  --build-context aguard-spec=.specgen \
  --build-context aguard-python=agentic_os \
  --target dev-base-full \
  --file docker/dev-base/Dockerfile \
  docker/dev-base
