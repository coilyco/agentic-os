#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 ]]; then
    echo "usage: actions <runs|tasks> <owner> <repo> [--page N] [--limit N]" >&2
    exit 64
fi

kind=$1
owner=$2
repo=$3
shift 3

case "$kind" in
    runs|tasks)
        ;;
    *)
        echo "actions list kind must be runs or tasks" >&2
        exit 64
        ;;
esac

if [[ -z "${FORGEJO_TOKEN:-}" ]]; then
    echo "FORGEJO_TOKEN is required" >&2
    exit 1
fi

page=1
limit=
while [[ $# -gt 0 ]]; do
    case "$1" in
        --page)
            shift
            if [[ $# -eq 0 ]]; then
                echo "--page requires a value" >&2
                exit 64
            fi
            page=$1
            ;;
        --limit)
            shift
            if [[ $# -eq 0 ]]; then
                echo "--limit requires a value" >&2
                exit 64
            fi
            limit=$1
            ;;
        *)
            echo "unknown flag: $1" >&2
            exit 64
            ;;
    esac
    shift
done

if ! [[ $page =~ ^[1-9][0-9]*$ ]]; then
    echo "--page must be a positive integer" >&2
    exit 64
fi
if [[ -n "$limit" && ! "$limit" =~ ^[1-9][0-9]*$ ]]; then
    echo "--limit must be a positive integer" >&2
    exit 64
fi

base_url="${FORGEJO_BASE_URL:-https://forgejo.coilysiren.me}"
url="${base_url}/api/v1/repos/${owner}/${repo}/actions/${kind}?page=${page}"
if [[ -n "$limit" ]]; then
    url="${url}&limit=${limit}"
fi

curl -fsSL \
    -H "Authorization: token ${FORGEJO_TOKEN}" \
    -H "Accept: application/json" \
    "$url"
