#!/usr/bin/env python3
"""Build or plan the tiered dev-base image family from the folder layout."""
from __future__ import annotations

import argparse
import json
import platform
import re
import subprocess
import sys
from collections.abc import Sequence
from pathlib import Path
from time import sleep
from typing import Callable

from agentic_os.dev_base import PUBLISHED_TIER_NAMES, REGISTRY_BASE, publish_plan


_DIGEST_RE = re.compile(r"^Digest:\s+(sha256:[0-9a-f]+)$", re.MULTILINE)


def _docker_base_command(push: bool, platforms: str | None) -> list[str]:
    if push:
        cmd = ["docker", "buildx", "build", "--progress=plain", "--push"]
        if platforms:
            cmd.extend(["--platform", platforms])
        return cmd
    return ["docker", "build"]


def _run(cmd: list[str]) -> None:
    subprocess.run(cmd, check=True)


def _retry(action: str, func: Callable[[], object], attempts: int = 4, initial_delay: int = 2) -> object:
    delay = initial_delay
    last_exc: Exception | None = None
    for attempt in range(1, attempts + 1):
        try:
            return func()
        except subprocess.CalledProcessError as exc:
            last_exc = exc
            if attempt == attempts:
                break
            print(
                f"retrying {action} after attempt {attempt} failed; sleeping {delay}s",
                file=sys.stderr,
            )
            sleep(delay)
            delay = min(delay * 2, 30)
    assert last_exc is not None
    raise last_exc


def _inspect_manifest(ref: str) -> str | None:
    probe = subprocess.run(
        ["docker", "buildx", "imagetools", "inspect", ref],
        check=False,
        capture_output=True,
        text=True,
    )
    if probe.returncode != 0:
        return None
    match = _DIGEST_RE.search(probe.stdout) or _DIGEST_RE.search(probe.stderr)
    if match is None:
        return None
    return match.group(1)


def _probe_cache_write(buildcache_ref: str) -> None:
    # cache-to runs with ignore-error=true so a registry hiccup cannot fail the
    # push. Probe the cache manifest and warn loudly so a cold cache is visible.
    def _inspect_once() -> subprocess.CompletedProcess[str]:
        cmd = ["docker", "buildx", "imagetools", "inspect", buildcache_ref]
        probe = subprocess.run(
            cmd,
            check=False,
            capture_output=True,
            text=True,
        )
        if probe.returncode != 0:
            raise subprocess.CalledProcessError(
                probe.returncode,
                cmd,
                output=getattr(probe, "stdout", ""),
                stderr=getattr(probe, "stderr", ""),
            )
        return probe

    try:
        _retry(f"inspect cache {buildcache_ref}", _inspect_once)
    except subprocess.CalledProcessError:
        print(
            f"::warning::buildcache write to {buildcache_ref} failed or is "
            "missing - the next build of this tier starts cold. "
            "imagetools inspect could not resolve the cache manifest.",
            file=sys.stderr,
        )


def _has_target_checkpoint(target_ref: str, alias_refs: Sequence[str]) -> bool:
    target = _inspect_manifest(target_ref)
    if target is None:
        return False
    if not alias_refs:
        return True
    aliases = [_inspect_manifest(alias_ref) for alias_ref in alias_refs]
    return all(alias is not None and alias == target for alias in aliases)


def _target_matches_source(target_ref: str, source_ref: str, alias_refs: Sequence[str]) -> bool:
    target = _inspect_manifest(target_ref)
    source = _inspect_manifest(source_ref)
    if target is None or source is None:
        return False
    if target != source:
        return False
    aliases = [_inspect_manifest(alias_ref) for alias_ref in alias_refs]
    return all(alias is not None and alias == source for alias in aliases)


def _ward_config_ref_commit() -> str:
    return (
        subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    )


def _host_targetarch() -> str:
    machine = platform.machine().lower()
    if machine in {"x86_64", "amd64"}:
        return "amd64"
    if machine in {"aarch64", "arm64"}:
        return "arm64"
    raise SystemExit(f"unsupported host architecture: {machine}")


def _build_plan(
    registry_base: str,
    tag: str,
    push: bool,
    platforms: str | None,
    aliases: str | Sequence[str] | None = None,
    only_tier: str | None = None,
) -> None:
    # only_tier serves the per-tier CI jobs: base/graft refs resolve to the
    # sibling jobs' already-pushed images under the same tag.
    plan = publish_plan(registry_base, tag, aliases)
    if only_tier is not None:
        plan = [entry for entry in plan if entry["tier"] == only_tier]
        if not plan:
            raise SystemExit(f"unknown tier: {only_tier}")
    ward_config_ref_commit = _ward_config_ref_commit()
    for entry in plan:
        dockerfile = Path(entry["dockerfile"])
        alias_images = tuple(entry.get("alias_images", ()))
        if push and _has_target_checkpoint(entry["image"], alias_images):
            print(f"{entry['tier']} already published at {entry['image']}; skipping tier")
            continue
        cmd = _docker_base_command(push, platforms)
        if entry["tier"] == "core" and not push:
            cmd.extend(["--build-arg", f"TARGETARCH={_host_targetarch()}"])
        if entry["tier"] == "core":
            cmd.extend(["--build-arg", f"WARD_CONFIG_REF_COMMIT={ward_config_ref_commit}"])
        elif entry["tier"] != "core":
            cmd.extend(["--build-arg", f"BASE_IMAGE={entry['base_image']}"])
        for arg_name, graft_ref in entry["graft_images"].items():
            cmd.extend(["--build-arg", f"{arg_name}={graft_ref}"])
        if push:
            buildcache_ref = entry["cache_image"]
            cmd.extend(
                [
                    "--cache-from",
                    f"type=registry,ref={buildcache_ref}",
                    "--cache-to",
                    f"type=registry,ref={buildcache_ref},mode=max,ignore-error=true",
                ]
            )
        cmd.extend(["-t", entry["image"]])
        if push:
            for alias_image in entry.get("alias_images", []):
                cmd.extend(["-t", alias_image])
        cmd.extend(["-f", str(dockerfile), str(dockerfile.parent.parent)])
        if push:
            _retry(f"build {entry['tier']}", lambda: _run(cmd))
        else:
            _run(cmd)
        if push:
            _retry(f"inspect built image {entry['image']}", lambda: _run(["docker", "buildx", "imagetools", "inspect", entry["image"]]))
            for alias_image in alias_images:
                _retry(
                    f"inspect built alias {alias_image}",
                    lambda alias_image=alias_image: _run(["docker", "buildx", "imagetools", "inspect", alias_image]),
                )
            _probe_cache_write(entry["cache_image"])


def _promote_plan(
    registry_base: str,
    source_tag: str,
    target_tag: str,
    aliases: str | Sequence[str] | None = None,
    only_tier: str | None = None,
) -> None:
    source_plan = publish_plan(registry_base, source_tag)
    target_plan = {entry["tier"]: entry for entry in publish_plan(registry_base, target_tag, aliases)}
    if only_tier is not None:
        source_plan = [entry for entry in source_plan if entry["tier"] == only_tier]
        if not source_plan:
            raise SystemExit(f"unknown tier: {only_tier}")
    for source_entry in source_plan:
        target_entry = target_plan[source_entry["tier"]]
        target_images = [target_entry["image"], *target_entry.get("alias_images", [])]
        if _target_matches_source(target_entry["image"], source_entry["image"], target_entry.get("alias_images", [])):
            print(
                f"{source_entry['tier']} already promoted to {target_entry['image']}; skipping tier"
            )
            continue
        cmd = ["docker", "buildx", "imagetools", "create"]
        for target_image in target_images:
            cmd.extend(["-t", target_image])
        cmd.append(source_entry["image"])
        _retry(f"promote {source_entry['tier']} -> {target_entry['image']}", lambda: _run(cmd))
        for target_image in target_images:
            _retry(
                f"inspect promoted image {target_image}",
                lambda target_image=target_image: _run(["docker", "buildx", "imagetools", "inspect", target_image]),
            )


def _cmd_plan(args: argparse.Namespace) -> int:
    print(json.dumps({"registry_base": args.registry, "tag": args.tag, "tiers": publish_plan(args.registry, args.tag, args.alias)}))
    return 0


def _cmd_build(args: argparse.Namespace) -> int:
    _build_plan(args.registry, args.tag, args.push, args.platforms, args.alias, args.tier)
    return 0


def _cmd_promote(args: argparse.Namespace) -> int:
    _promote_plan(args.registry, args.source_tag, args.tag, args.alias, args.tier)
    return 0


def _cmd_check(args: argparse.Namespace) -> int:
    plan = publish_plan(args.registry, args.tag, args.alias)
    if args.tier is not None:
        plan = [entry for entry in plan if entry["tier"] == args.tier]
        if not plan:
            raise SystemExit(f"unknown tier: {args.tier}")
    if len(plan) != 1:
        raise SystemExit("check requires exactly one tier")

    target_entry = plan[0]
    alias_images = tuple(target_entry.get("alias_images", ()))
    if args.mode == "build":
        return 0 if _has_target_checkpoint(target_entry["image"], alias_images) else 1

    if not args.source_tag:
        raise SystemExit("source-tag is required in promote mode")
    source_plan = publish_plan(args.registry, args.source_tag)
    if args.tier is not None:
        source_plan = [entry for entry in source_plan if entry["tier"] == args.tier]
    if len(source_plan) != 1:
        raise SystemExit("promote check requires exactly one tier")
    source_entry = source_plan[0]
    return 0 if _target_matches_source(target_entry["image"], source_entry["image"], alias_images) else 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--registry",
        default=REGISTRY_BASE,
        help="Registry/image family base (default: Forgejo release registry).",
    )
    parser.add_argument(
        "--tag",
        required=True,
        help="Release tag or local tag to stamp onto each tier image.",
    )
    parser.add_argument(
        "--alias",
        action="append",
        default=[],
        help="Moving alias tag pushed alongside the stamped tag. May be repeated.",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    p_plan = sub.add_parser("plan", help="Print the derived tier plan as JSON.")
    p_plan.set_defaults(func=_cmd_plan)

    p_build = sub.add_parser("build", help="Build the tier family in order.")
    p_build.add_argument("--push", action="store_true", help="Push each published tier instead of loading locally.")
    p_build.add_argument(
        "--tier",
        default=None,
        choices=PUBLISHED_TIER_NAMES,
        help="Build only this tier (per-tier CI jobs). Base and graft tiers must already exist under the same tag.",
    )
    p_build.add_argument(
        "--platforms",
        default="linux/amd64,linux/arm64",
        help="Platforms for buildx publishes (ignored for local builds).",
    )
    p_build.set_defaults(func=_cmd_build)

    p_promote = sub.add_parser("promote", help="Retag an already-pushed image family.")
    p_promote.add_argument(
        "--source-tag",
        required=True,
        help="Existing tag to retag from, usually the draft tag built on main.",
    )
    p_promote.add_argument(
        "--tier",
        default=None,
        choices=PUBLISHED_TIER_NAMES,
        help="Promote only this tier (per-tier CI jobs).",
    )
    p_promote.set_defaults(func=_cmd_promote)

    p_check = sub.add_parser("check", help="Report whether the requested tier already matches its target checkpoint.")
    p_check.add_argument("--mode", required=True, choices=("build", "promote"))
    p_check.add_argument(
        "--tier",
        default=None,
        choices=PUBLISHED_TIER_NAMES,
        help="Check only this tier (per-tier CI jobs).",
    )
    p_check.add_argument(
        "--source-tag",
        default="",
        help="Existing source tag used by promote mode.",
    )
    p_check.set_defaults(func=_cmd_check)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
