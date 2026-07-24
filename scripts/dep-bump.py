#!/usr/bin/env python3
"""Resolve the dev-base image's pinned tool versions against upstream and bump them.

The dev-base Dockerfile hand-pins every tool as an `ARG` (`UV_VERSION`,
`NODE_VERSION`, ...). `release.yml`
republishes the full image on every push to main, but nothing keeps those
pins current, so the published image drifts behind upstream until a human edits
an `ARG` (agentic-os#272, ward#301).

This script is the auto-bump: `plan` resolves each pin's latest upstream release
and reports the drift; `apply` rewrites a single `ARG` in place. The scheduled
`.forgejo/workflows/dep-bump.yml` runs `plan`, commits one bump per stale tool,
and pushes to main, which republishes via the existing `publish-full` job. The
bump logic lives here (the publish pipeline is this repo's per AGENTS.md); the
fleet rollout is not this script's concern.

Stdlib only (urllib, no `requests`) so it runs on a bare Forgejo runner with just
`python3`. Each resolver is isolated: an upstream that is flaky or reshapes its
API drops that one tool from the plan, it never blocks the others.

Version policy: bumps stay on the pinned line where crossing it is risky. Node
tracks the latest release of its currently-pinned major (no surprise major jump);
uv/go/aws-cli/claude/mcporter/codex/goose/gh/helm/kubectl/yq track the latest
stable upstream release (mcporter off npm, everything else off a GitHub
Releases/tags feed). Ward is manual until raw ward releases become trustworthy
staging inputs and aos owns the prod/N-1 promotion.

Usage:
    python3 scripts/dep-bump.py plan            # TSV: NAME<TAB>CURRENT<TAB>LATEST
    python3 scripts/dep-bump.py plan --json      # same, as JSON (machine/tests)
    python3 scripts/dep-bump.py check            # human summary, exit 1 on drift
    python3 scripts/dep-bump.py apply --arg UV_VERSION --version 0.12.0
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.request
from pathlib import Path
from typing import Callable

from agentic_os.dev_base import DEV_BASE_ROOT, TIER_SPECS

DOCKERFILE = DEV_BASE_ROOT

GITHUB_API = "https://api.github.com"
_TIMEOUT = 30


# --- Pure helpers (Dockerfile parsing + semver), unit-tested without network. --

_ARG_RE = re.compile(r"^ARG (?P<name>[A-Z0-9_]+)=(?P<value>\S+)\s*$", re.MULTILINE)


def parse_args_block(text: str) -> dict[str, str]:
    """Map every `ARG NAME=value` default in a Dockerfile to its value."""
    return {m.group("name"): m.group("value") for m in _ARG_RE.finditer(text)}


def load_pins(dockerfile: Path) -> dict[str, str]:
    """Collect ARG defaults from one Dockerfile or the dev-base tree."""
    if dockerfile.is_file():
        return parse_args_block(dockerfile.read_text(encoding="utf-8"))
    pins: dict[str, str] = {}
    dockerfiles = dict.fromkeys(tier.dockerfile for tier in TIER_SPECS)
    for path in dockerfiles:
        pins.update(parse_args_block(path.read_text(encoding="utf-8")))
    return pins


def set_arg(text: str, name: str, version: str) -> str:
    """Rewrite a single `ARG NAME=...` default, leaving everything else byte-identical."""
    pattern = re.compile(rf"^ARG {re.escape(name)}=\S+", re.MULTILINE)
    new_text, count = pattern.subn(f"ARG {name}={version}", text)
    if count != 1:
        raise SystemExit(f"expected exactly one 'ARG {name}=' line, found {count}")
    return new_text


def set_arg_in_file(path: Path, name: str, version: str) -> None:
    """Rewrite one ARG default in a single file in place."""
    path.write_text(set_arg(path.read_text(encoding="utf-8"), name, version), encoding="utf-8")


def set_arg_in_tree(root: Path, name: str, version: str) -> None:
    """Rewrite one ARG default everywhere it appears in a Dockerfile tree."""
    dockerfiles = sorted(root.rglob("Dockerfile"))
    touched = 0
    needle = re.compile(rf"^ARG {re.escape(name)}=\S+", re.MULTILINE)
    for dockerfile in dockerfiles:
        text = dockerfile.read_text(encoding="utf-8")
        if not needle.search(text):
            continue
        dockerfile.write_text(set_arg(text, name, version), encoding="utf-8")
        touched += 1
    if touched == 0:
        raise SystemExit(f"expected at least one 'ARG {name}=' line, found 0")


def parse_semver(value: str) -> tuple[int, ...]:
    """Numeric (major, minor, patch) for sorting. Non-numeric tails are dropped."""
    parts = value.split(".")[:3]
    out: list[int] = []
    for part in parts:
        digits = re.match(r"\d+", part)
        out.append(int(digits.group()) if digits else 0)
    while len(out) < 3:
        out.append(0)
    return tuple(out)


def pick_highest(versions: list[str]) -> str | None:
    """Highest version by numeric semver order, or None if the list is empty."""
    return max(versions, key=parse_semver) if versions else None


# --- Network resolvers (one per pinned tool). -------------------------------

def _get_json(url: str) -> object:
    headers = {"User-Agent": "aos-dep-bump", "Accept": "application/json"}
    # An optional token only lifts github.com's 60/hr anonymous rate limit; the
    # script works without it (one daily run makes a handful of calls).
    token = os.environ.get("UPSTREAM_GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if token and url.startswith(GITHUB_API):
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
        return json.load(resp)


def _gh_release(repo: str, tag_re: re.Pattern[str]) -> str | None:
    """Highest stable (non-draft, non-prerelease) release tag matching `tag_re` group 1."""
    data = _get_json(f"{GITHUB_API}/repos/{repo}/releases?per_page=100")
    cands = []
    for rel in data:  # type: ignore[union-attr]
        if rel.get("draft") or rel.get("prerelease"):
            continue
        match = tag_re.match(rel.get("tag_name", ""))
        if match:
            cands.append(match.group(1))
    return pick_highest(cands)


def _gh_tag(repo: str, tag_re: re.Pattern[str]) -> str | None:
    """Highest tag matching `tag_re` group 1, for repos that ship tags but no Releases."""
    data = _get_json(f"{GITHUB_API}/repos/{repo}/tags?per_page=100")
    cands = []
    for tag in data:  # type: ignore[union-attr]
        match = tag_re.match(tag.get("name", ""))
        if match:
            cands.append(match.group(1))
    return pick_highest(cands)


def _resolve_uv() -> str | None:
    return _gh_release("astral-sh/uv", re.compile(r"^v?(\d+\.\d+\.\d+)$"))


def _resolve_node(current: str) -> str | None:
    # Stay on the pinned major (e.g. 22): an auto major jump can break the runtime.
    major = current.split(".")[0]
    data = _get_json("https://nodejs.org/dist/index.json")
    cands = [
        rel["version"].lstrip("v")
        for rel in data  # type: ignore[union-attr]
        if rel["version"].lstrip("v").split(".")[0] == major
    ]
    return pick_highest(cands)


def _resolve_go() -> str | None:
    data = _get_json("https://go.dev/dl/?mode=json")
    cands = [rel["version"].removeprefix("go") for rel in data if rel.get("stable")]  # type: ignore[union-attr]
    return pick_highest(cands)


def _resolve_awscli() -> str | None:
    # aws-cli ships v1 and v2 lines as tags and no Releases; track latest v2.
    return _gh_tag("aws/aws-cli", re.compile(r"^(2\.\d+\.\d+)$"))


def _resolve_claude() -> str | None:
    data = _get_json("https://registry.npmjs.org/@anthropic-ai/claude-code/latest")
    return data.get("version")  # type: ignore[union-attr]


def _resolve_mcporter() -> str | None:
    data = _get_json("https://registry.npmjs.org/mcporter/latest")
    return data.get("version")  # type: ignore[union-attr]


def _resolve_codex() -> str | None:
    # Tags are rust-vX.Y.Z with rust-vX.Y.Z-alpha.N prereleases interleaved.
    return _gh_release("openai/codex", re.compile(r"^rust-v(\d+\.\d+\.\d+)$"))


def _resolve_goose() -> str | None:
    return _gh_release("block/goose", re.compile(r"^v(\d+\.\d+\.\d+)$"))


def _resolve_gh() -> str | None:
    return _gh_release("cli/cli", re.compile(r"^v?(\d+\.\d+\.\d+)$"))


def _resolve_docker() -> str | None:
    # The static client tarball tracks the moby engine release; moby/moby tags as
    # vX.Y.Z. The strict regex drops the -rc/-beta prereleases _gh_tag would see.
    return _gh_tag("moby/moby", re.compile(r"^v(\d+\.\d+\.\d+)$"))


def _resolve_helm() -> str | None:
    return _gh_release("helm/helm", re.compile(r"^v(\d+\.\d+\.\d+)$"))


def _resolve_kubectl() -> str | None:
    return _gh_release("kubernetes/kubernetes", re.compile(r"^v(\d+\.\d+\.\d+)$"))


def _resolve_tailscale() -> str | None:
    # Track the stable feed, not github tags (which interleave unstable odd-minor
    # releases); TarballsVersion is the version baked into the static tarball URL.
    data = _get_json("https://pkgs.tailscale.com/stable/?mode=json")
    return data.get("TarballsVersion")  # type: ignore[union-attr]


def _resolve_trufflehog() -> str | None:
    return _gh_release("trufflesecurity/trufflehog", re.compile(r"^v(\d+\.\d+\.\d+)$"))


def _resolve_yq() -> str | None:
    return _gh_release("mikefarah/yq", re.compile(r"^v(\d+\.\d+\.\d+)$"))


def _resolve_dotnet(current: str) -> str | None:
    # Stay on the pinned .NET major (like NODE_VERSION): the per-channel release
    # metadata carries `latest-sdk`, so an auto major jump never blindsides a consumer.
    major = current.split(".")[0]
    data = _get_json(
        f"https://builds.dotnet.microsoft.com/dotnet/release-metadata/{major}.0/releases.json"
    )
    return data.get("latest-sdk")  # type: ignore[union-attr]


# Resolver per ARG. No resolver means never auto-bumped.
# Node stays on major; ward opts out until aos validates prod/N-1.
RESOLVERS: dict[str, Callable[..., str | None]] = {
    "UV_VERSION": _resolve_uv,
    "NODE_VERSION": _resolve_node,
    "GO_VERSION": _resolve_go,
    "AWSCLI_VERSION": _resolve_awscli,
    "CLAUDE_VERSION": _resolve_claude,
    "MCPORTER_VERSION": _resolve_mcporter,
    "CODEX_VERSION": _resolve_codex,
    "GOOSE_VERSION": _resolve_goose,
    "GH_VERSION": _resolve_gh,
    "DOCKER_VERSION": _resolve_docker,
    "HELM_VERSION": _resolve_helm,
    "KUBECTL_VERSION": _resolve_kubectl,
    "YQ_VERSION": _resolve_yq,
    "TAILSCALE_VERSION": _resolve_tailscale,
    "TRUFFLEHOG_VERSION": _resolve_trufflehog,
    "DOTNET_VERSION": _resolve_dotnet,
}
_NEEDS_CURRENT = {"NODE_VERSION", "DOTNET_VERSION"}

def compute_plan(text: str) -> list[dict[str, str]]:
    """Resolve every pinned ARG and return the ones whose upstream latest differs.

    A resolver that raises or returns None is skipped with a stderr warning, so one
    unreachable upstream never blocks the rest of the plan.
    """
    pins = parse_args_block(text)
    plan: list[dict[str, str]] = []
    for name, resolver in RESOLVERS.items():
        current = pins.get(name)
        if current is None:
            continue
        try:
            latest = resolver(current) if name in _NEEDS_CURRENT else resolver()
        except Exception as exc:  # noqa: BLE001 - one flaky upstream must not sink the run
            print(f"warning: resolving {name} failed: {exc}", file=sys.stderr)
            continue
        if not latest:
            print(f"warning: no upstream version resolved for {name}", file=sys.stderr)
            continue
        if latest != current:
            plan.append({"arg": name, "current": current, "latest": latest})
    return plan


def _compute_tree_plan(root: Path) -> list[dict[str, str]]:
    pins = load_pins(root)
    plan: list[dict[str, str]] = []
    for name, resolver in RESOLVERS.items():
        current = pins.get(name)
        if current is None:
            continue
        try:
            latest = resolver(current) if name in _NEEDS_CURRENT else resolver()
        except Exception as exc:  # noqa: BLE001 - one flaky upstream must not sink the run
            print(f"warning: resolving {name} failed: {exc}", file=sys.stderr)
            continue
        if not latest:
            print(f"warning: no upstream version resolved for {name}", file=sys.stderr)
            continue
        if latest != current:
            plan.append({"arg": name, "current": current, "latest": latest})
    return plan


# --- CLI --------------------------------------------------------------------

def _cmd_plan(args: argparse.Namespace) -> int:
    plan = _compute_tree_plan(args.dockerfile)
    if args.json:
        print(json.dumps(plan))
    else:
        for entry in plan:
            print(f"{entry['arg']}\t{entry['current']}\t{entry['latest']}")
    return 0


def _cmd_check(args: argparse.Namespace) -> int:
    plan = _compute_tree_plan(args.dockerfile)
    if not plan:
        print("dev-base pins are current with upstream.")
        return 0
    print("dev-base pins drifting behind upstream:")
    for entry in plan:
        print(f"  {entry['arg']}: {entry['current']} -> {entry['latest']}")
    return 1


def _cmd_apply(args: argparse.Namespace) -> int:
    if args.dockerfile.is_file():
        set_arg_in_file(args.dockerfile, args.arg, args.version)
    else:
        set_arg_in_tree(args.dockerfile, args.arg, args.version)
    print(f"set {args.arg}={args.version}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dockerfile",
        type=Path,
        default=DOCKERFILE,
        help="Dockerfile or dev-base Dockerfile tree root to read/rewrite (default: docker/dev-base/).",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    p_plan = sub.add_parser("plan", help="Report pins drifting behind upstream.")
    p_plan.add_argument("--json", action="store_true", help="Emit JSON instead of TSV.")
    p_plan.set_defaults(func=_cmd_plan)

    p_check = sub.add_parser("check", help="Human summary; exit 1 if any pin is stale.")
    p_check.set_defaults(func=_cmd_check)

    p_apply = sub.add_parser("apply", help="Rewrite one ARG default in place.")
    p_apply.add_argument("--arg", required=True, help="ARG name, e.g. UV_VERSION.")
    p_apply.add_argument("--version", required=True, help="New version value.")
    p_apply.set_defaults(func=_cmd_apply)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
