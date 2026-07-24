#!/usr/bin/env bash

set -euo pipefail

docker_cli_ver=$(curl -fsSL https://download.docker.com/linux/static/stable/x86_64/ \
  | grep -oE 'docker-[0-9]+\.[0-9]+\.[0-9]+\.tgz' | sort -V | tail -1 \
  | sed -E 's/^docker-(.*)\.tgz$/\1/')
curl -fsSL "https://download.docker.com/linux/static/stable/x86_64/docker-${docker_cli_ver}.tgz" \
  | tar -xz -C /tmp
install /tmp/docker/docker /usr/local/bin/docker
mkdir -p ~/.docker/cli-plugins
curl -fsSL "https://github.com/docker/buildx/releases/download/${BUILDX_VERSION}/buildx-${BUILDX_VERSION}.linux-amd64" \
  -o ~/.docker/cli-plugins/docker-buildx
chmod +x ~/.docker/cli-plugins/docker-buildx
docker buildx version
docker --version
