#!/usr/bin/env bash

set -euo pipefail

uv run pytest
pre-commit run --all-files
