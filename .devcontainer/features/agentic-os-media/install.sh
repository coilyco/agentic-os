#!/usr/bin/env bash
# Media tools for the fleet dev container. See docs/dev-container.md.
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update -o Acquire::Retries=3
apt-get install -y --no-install-recommends ffmpeg imagemagick
rm -rf /var/lib/apt/lists/*
convert -version
ffmpeg -version
