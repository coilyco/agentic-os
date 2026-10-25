#!/bin/sh
# Stage the guard shim sources that the agent dev container builds in place.
# usage: stage-shim-src.sh <dest>. Called by the release bundle build.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dest=${1:?usage: stage-shim-src.sh <dest>}

mkdir -p "$dest/shims" "$dest/fs-shims"
cp -R "$repo_root/.umbra/shims/." "$dest/shims/"
cp "$repo_root/.umbra/guardfiles/specverb.lock" "$dest/shims/specverb.lock"
cp "$repo_root/scripts/fs-shims/fs_guard.py" "$repo_root/scripts/fs-shims/git_guard.py" \
    "$repo_root/scripts/fs-shims/install.sh" "$dest/fs-shims/"
echo "staged shim sources into $dest"
