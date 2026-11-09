#!/usr/bin/env python3
"""Resolve the immutable tag for a product's promoted release branch.

Exits non-zero when it cannot: a silent fallback to the literal `release` ref
left callers unpinned without anyone noticing (COI-3206).
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.request
from collections.abc import Callable, Sequence
from dataclasses import dataclass

from agentic_os import shared_ssl_context


GITHUB_API = "https://api.github.com"
_TIMEOUT = 30
_PEEL_DEPTH = 3


class ReleaseRefError(Exception):
    """The release branch has no immutable tag to resolve to."""


@dataclass(frozen=True)
class Product:
    repository: str
    tag_prefix: str
    tag_pattern: re.Pattern[str]


_GUARD = Product(
    repository="umbra",
    tag_prefix="v",
    tag_pattern=re.compile(r"^v(\d+\.\d+\.\d+)$"),
)
_AOS = Product(
    repository="agentic-os",
    tag_prefix="aos-v",
    tag_pattern=re.compile(r"^aos-v(\d+\.\d+\.\d+)$"),
)
PRODUCTS = {
    "guard": _GUARD,
    "umbra": _GUARD,
    "cli-guard": _GUARD,  # retained alias
    "specgen": _GUARD,  # retained alias: the driver's name until umbra v0.192.0
    "aos": _AOS,
    "agentic-os": _AOS,
}


def _get_json(url: str) -> object:
    headers = {"User-Agent": "aos-prod-install-ref", "Accept": "application/json"}
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(
        request, timeout=_TIMEOUT, context=shared_ssl_context()
    ) as response:
        return json.load(response)


def _semver(value: str) -> tuple[int, int, int]:
    major, minor, patch = value.split(".")
    return int(major), int(minor), int(patch)


def _commit_of(
    fetch: Callable[[str], object], base: str, target: dict[str, str]
) -> str:
    """The commit a matching ref points at, peeling annotated tag objects."""
    for _ in range(_PEEL_DEPTH):
        if target["type"] != "tag":
            return target["sha"]
        tag = fetch(f"{base}/git/tags/{target['sha']}")
        target = tag["object"]  # type: ignore[index]
    raise ValueError("annotated tag chain is too deep")


def resolve_release_ref(
    product_name: str,
    *,
    fetch_json: Callable[[str], object] | None = None,
) -> str:
    """Return the product tag at the head of `release`, or raise ReleaseRefError.

    The tags listing is not ordered by recency, so this reads the branch head
    and asks git/matching-refs for the tags under the product prefix. That call
    returns every match in one response and ignores `page`.
    """
    product = PRODUCTS[product_name]
    fetch = fetch_json or _get_json
    base = f"{GITHUB_API}/repos/coilyco/{product.repository}"
    try:
        branch = fetch(f"{base}/branches/release")
        release_sha = branch["commit"]["sha"]  # type: ignore[index]
        refs = fetch(f"{base}/git/matching-refs/tags/{product.tag_prefix}")
        if not isinstance(refs, list):
            raise TypeError("git/matching-refs did not return a list")
        matches: list[tuple[tuple[int, int, int], str]] = []
        for ref in refs:
            name = str(ref["ref"]).removeprefix("refs/tags/")
            match = product.tag_pattern.fullmatch(name)
            if match is None:
                continue
            if _commit_of(fetch, base, ref["object"]) == release_sha:
                matches.append((_semver(match.group(1)), name))
    except (KeyError, TypeError, OSError, ValueError) as exc:
        raise ReleaseRefError(
            f"cannot read {product.repository} release refs: {exc!r}"
        ) from exc
    if not matches:
        raise ReleaseRefError(
            f"{product.repository} release is at {release_sha}, which carries "
            f"no {product.tag_prefix}* tag"
        )
    return max(matches)[1]


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("product", choices=sorted(PRODUCTS))
    args = parser.parse_args(argv)
    try:
        print(resolve_release_ref(args.product))
    except ReleaseRefError as exc:
        print(f"prod-install-ref: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
