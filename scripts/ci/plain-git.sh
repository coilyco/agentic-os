#!/usr/bin/env bash
# Put the system git ahead of the image's shim directory for the steps after this one.
# The :release image published with the umbra git shim, which remaps git's exit
# status to 3 and refuses "<" in an argument (COI-2738), so every gate run in it
# failed, including the gate that would publish the fix. Remove this step, and its
# three callers, once :release carries the script guard.
set -euo pipefail

shim_dir=/opt/agentic-os/shims
clean=$(printf '%s' "$PATH" | tr ':' '\n' | grep -vx "$shim_dir" | paste -sd: -)
real=$(PATH="$clean" command -v git)
mkdir -p /tmp/plain-git
ln -sf "$real" /tmp/plain-git/git
echo /tmp/plain-git >> "$GITHUB_PATH"
echo "plain git: $real"
