#!/bin/sh
# Install the script guards and their git, rm, find and mv stubs into a shim
# directory. usage: install.sh <shim-dir>. Copies rather than links: the stubs
# find the guard beside themselves, and a link would resolve to the checkout.
set -eu

here=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
dest=${1:?usage: install.sh <shim-dir>}

mkdir -p "$dest"
cp "$here/fs_guard.py" "$here/git_guard.py" "$dest/"
stub() {
    printf '#!/bin/sh\nexec python3 -I "$(dirname "$0")/%s" %s "$@"\n' "$2" "$1" > "$dest/$1"
    chmod 0755 "$dest/$1"
}
stub git git_guard.py
for tool in rm find mv; do stub "$tool" fs_guard.py; done
echo "installed guard stubs (git rm find mv) into $dest"
