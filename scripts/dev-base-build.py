#!/usr/bin/env python3
"""Build or plan the tiered dev-base image family from the folder layout."""
from __future__ import annotations

import argparse
import json
import platform
import subprocess
import sys
from pathlib import Path

from agentic_os.dev_base import PUBLISHED_TIER_NAMES, REGISTRY_BASE, publish_plan


def _docker_base_command(push: bool, platforms: str | None) -> list[str]:
    if push:
        cmd = ["docker", "buildx", "build", "--progress=plain", "--push"]
        if platforms:
            cmd.extend(["--platform", platforms])
        return cmd
    return ["docker", "build"]


def _run(cmd: list[str]) -> None:
    subprocess.run(cmd, check=True)


def _probe_cache_write(buildcache_ref: str) -> None:
    # cache-to runs with ignore-error=true so a registry hiccup cannot fail the
    # push; probe the cache manifest and warn loudly so a cold cache is visible.
    probe = subprocess.run(
        ["docker", "buildx", "imagetools", "inspect", buildcache_ref],
        check=False,
        capture_output=True,
        text=True,
    )
    if probe.returncode != 0:
        print(
            f"::warning::buildcache write to {buildcache_ref} failed or is "
            "missing - the next build of this tier starts cold. "
            f"imagetools inspect said: {probe.stderr.strip()}",
            file=sys.stderr,
        )


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
    alias: str | None = None,
    only_tier: str | None = None,
) -> None:
    # only_tier serves the per-tier CI jobs: base/graft refs resolve to the
    # sibling jobs' already-pushed images under the same tag.
    plan = publish_plan(registry_base, tag, alias)
    if only_tier is not None:
        plan = [entry for entry in plan if entry["tier"] == only_tier]
        if not plan:
            raise SystemExit(f"unknown tier: {only_tier}")
    ward_config_ref_commit = _ward_config_ref_commit()
    for entry in plan:
        dockerfile = Path(entry["dockerfile"])
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
        if push and entry.get("alias_image"):
            # The pipeline passes its branch name as the moving alias, so
            # ward's default agentic-os:release surface stays pullable.
            cmd.extend(["-t", entry["alias_image"]])
        cmd.extend(["-f", str(dockerfile), str(dockerfile.parent.parent)])
        _run(cmd)
        if push:
            _run(["docker", "buildx", "imagetools", "inspect", entry["image"]])
            if entry.get("alias_image"):
                _run(["docker", "buildx", "imagetools", "inspect", entry["alias_image"]])
            if entry.get("legacy_alias_image"):
                _run(
                    [
                        "docker",
                        "buildx",
                        "imagetools",
                        "create",
                        "-t",
                        entry["legacy_alias_image"],
                        entry["alias_image"] if entry.get("alias_image") else entry["image"],
                    ]
                )
                _run(["docker", "buildx", "imagetools", "inspect", entry["legacy_alias_image"]])
            _probe_cache_write(entry["cache_image"])


def _cmd_plan(args: argparse.Namespace) -> int:
    print(json.dumps({"registry_base": args.registry, "tag": args.tag, "tiers": publish_plan(args.registry, args.tag, args.alias)}))
    return 0


def _cmd_build(args: argparse.Namespace) -> int:
    _build_plan(args.registry, args.tag, args.push, args.platforms, args.alias, args.tier)
    return 0


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
        default=None,
        help="Moving alias tag pushed alongside the release tag (the CI pipeline passes its branch name).",
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
        help="Build only this tier (per-tier CI jobs); base and graft tiers must already exist under the same tag.",
    )
    p_build.add_argument(
        "--platforms",
        default="linux/amd64,linux/arm64",
        help="Platforms for buildx publishes (ignored for local builds).",
    )
    p_build.set_defaults(func=_cmd_build)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
