#!/usr/bin/env bash
# Install uv for the repository's Python-backed publication helper.

set -euo pipefail

curl --retry 5 --retry-all-errors --retry-delay 2 -LsSf \
  "https://astral.sh/uv/${UV_VERSION}/install.sh" | sh
echo "$HOME/.local/bin" >> "$GITHUB_PATH"
export PATH="$HOME/.local/bin:$PATH"
uv --version
