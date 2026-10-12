#!/bin/sh
# Prune unused Docker build cache when free space on the data volume drops
# below a floor. Build cache is the only target: never images, never volumes.
# usage: disk-guard.sh. Tunables are env vars, see docs/install.md#disk-pressure-guard.
set -eu

min_kib=${DISK_GUARD_MIN_KIB:-15728640}
docker=${DISK_GUARD_DOCKER:-docker}
volume=${DISK_GUARD_VOLUME:-}
if [ -z "$volume" ]; then
    # The data volume holds the user's files and Docker's disk on macOS.
    if [ -d /System/Volumes/Data ]; then volume=/System/Volumes/Data; else volume=/; fi
fi

free_kib() {
    df -Pk "$volume" | awk 'NR == 2 { print $4 }'
}

before=$(free_kib)
if [ "$before" -ge "$min_kib" ]; then
    exit 0
fi

stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }
echo "$(stamp) free ${before} KiB on $volume is below ${min_kib} KiB, pruning build cache"
if ! "$docker" builder prune -f; then
    echo "$(stamp) docker builder prune failed, free space unchanged at ${before} KiB" >&2
    exit 1
fi
after=$(free_kib)
echo "$(stamp) free before=${before} KiB after=${after} KiB"
