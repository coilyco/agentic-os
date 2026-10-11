#!/bin/sh
# Install the fs guard and its rm, find and mv stubs into a shim directory.
# usage: install.sh <shim-dir>. Copies rather than links: the stubs find the
# guard beside themselves, and a link would resolve to the checkout.
set -eu

here=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
dest=${1:?usage: install.sh <shim-dir>}

mkdir -p "$dest"
cp "$here/fs_guard.py" "$dest/fs_guard.py"
for tool in rm find mv; do
    printf '#!/bin/sh\nexec python3 -I "$(dirname "$0")/fs_guard.py" %s "$@"\n' "$tool" > "$dest/$tool"
    chmod 0755 "$dest/$tool"
done
echo "installed fs guard stubs (rm find mv) into $dest"
