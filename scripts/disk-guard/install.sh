#!/bin/sh
# Install the disk guard script and its launchd job.
# usage: install.sh <bin-dir> <launch-agents-dir> <log-file> [--load]
# Copies the script rather than linking it, so the job survives the checkout
# moving. --load also bootstraps the job into the current user's launchd.
set -eu

here=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
bin=${1:?usage: install.sh <bin-dir> <launch-agents-dir> <log-file> [--load]}
agents=${2:?missing launch-agents-dir}
log=${3:?missing log-file}
label=dev.coilyco.disk-guard

mkdir -p "$bin" "$agents" "$(dirname "$log")"
cp "$here/disk-guard.sh" "$bin/disk-guard.sh"
chmod 0755 "$bin/disk-guard.sh"
sed -e "s|@SCRIPT@|$bin/disk-guard.sh|" -e "s|@LOG@|$log|" \
    "$here/disk-guard.plist.tmpl" > "$agents/$label.plist"
echo "installed $bin/disk-guard.sh and $agents/$label.plist"

if [ "${4:-}" = "--load" ]; then
    uid=$(id -u)
    launchctl bootout "gui/$uid/$label" 2>/dev/null || true
    launchctl bootstrap "gui/$uid" "$agents/$label.plist"
    echo "loaded $label"
fi
