"""List Forgejo Actions runs or tasks through the guarded operator surface."""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request


DEFAULT_BASE_URL = "https://forgejo.coilysiren.me"


def _positive_int(value: str) -> int:
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("must be a positive integer")
    return parsed


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(prog="actions")
    parser.add_argument("kind", choices=("runs", "tasks"))
    parser.add_argument("owner")
    parser.add_argument("repo")
    parser.add_argument("--page", type=_positive_int, default=1)
    parser.add_argument("--limit", type=_positive_int)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    token = os.environ.get("FORGEJO_TOKEN")
    if not token:
        print("FORGEJO_TOKEN is required", file=sys.stderr)
        return 1

    query = {"page": args.page}
    if args.limit is not None:
        query["limit"] = args.limit
    base_url = os.environ.get("FORGEJO_BASE_URL", DEFAULT_BASE_URL).rstrip("/")
    url = (
        f"{base_url}/api/v1/repos/{args.owner}/{args.repo}/actions/{args.kind}"
        f"?{urllib.parse.urlencode(query)}"
    )
    request = urllib.request.Request(
        url,
        headers={
            "Authorization": f"token {token}",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(request) as response:
            payload = json.load(response)
    except (OSError, urllib.error.HTTPError, json.JSONDecodeError) as exc:
        print(f"Forgejo Actions list failed. target_url={url}. {exc}", file=sys.stderr)
        return 65

    json.dump(payload, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
