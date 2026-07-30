"""Decide whether a repository change requires a published release artifact."""

from __future__ import annotations

import argparse
from collections.abc import Iterable
import json
from pathlib import Path
import subprocess

from agentic_os.dev_base import affected_tiers


_AOS_CLI_EXACT_INPUTS = frozenset(
    {
        "agentic_os/__init__.py",
        "agentic_os/forgejo_actions_list.py",
        "agentic_os/forgejo_actions_logs.py",
        "agentic_os/forgejo_actions_rerun.py",
        "agentic_os/forgejo_actions_web.py",
        "docker/dev-base/Dockerfile",
        "scripts/aos-release-build.sh",
        "scripts/render-aos-packaging.sh",
    }
)
_AOS_EMBEDDED_INPUTS = frozenset(
    {
        "aos/harness_launch_profiles.json",
        "aos/layout-model-classes.json",
        "aos/release-targets.txt",
        "aos/role-harnesses.json",
    }
)
_AOS_PRECOMMIT_EXACT_INPUTS = frozenset(
    {
        ".pre-commit-hooks.yaml",
        "agentic_os/__init__.py",
        "agentic_os/config.py",
        "agentic_os/generators/__init__.py",
        "agentic_os/generators/generate_agent_compose.py",
        "agentic_os/generators/generate_agents_pointer.py",
        "agentic_os/generators/generate_repo_pointer_skill.py",
        "agentic_os/generators/generate_seed_skills.py",
        "agentic_os/seed_skills_data.py",
        "pyproject.toml",
        "scripts/apply-agentic-os-hooks.py",
        "scripts/trufflehog-scan.sh",
    }
)


def _normalize_repo_path(path: str | Path) -> str:
    normalized = str(path).replace("\\", "/")
    while normalized.startswith("./"):
        normalized = normalized[2:]
    return normalized.rstrip("/")


def _production_go_input(path: str, root: str) -> bool:
    prefix = f"{root}/"
    if not path.startswith(prefix):
        return False
    relative = path.removeprefix(prefix)
    if "/" in relative:
        return False
    return relative in {"go.mod", "go.sum"} or (
        relative.endswith(".go") and not relative.endswith("_test.go")
    )


def is_aos_cli_release_input(path: str | Path) -> bool:
    """Return whether a path can change shipped CLI bytes or package metadata."""

    normalized = _normalize_repo_path(path)
    if normalized in _AOS_CLI_EXACT_INPUTS or normalized in _AOS_EMBEDDED_INPUTS:
        return True
    if normalized.startswith(".specgen/guardfiles/"):
        return True
    return any(
        _production_go_input(normalized, root)
        for root in ("aos", "agent-terminal", "aosguard-release")
    )


def is_aos_precommit_release_input(path: str | Path) -> bool:
    """Return whether a path changes an installed aos-precommit hook."""

    normalized = _normalize_repo_path(path)
    return (
        normalized in _AOS_PRECOMMIT_EXACT_INPUTS
        or normalized.startswith("agentic_os/pre_commit/")
    )


def release_required(surface: str, changed_paths: Iterable[str | Path]) -> bool:
    """Return whether the selected artifact surface has a relevant change."""

    paths = tuple(_normalize_repo_path(path) for path in changed_paths)
    if surface == "aos-cli":
        return any(is_aos_cli_release_input(path) for path in paths)
    if surface == "aos-precommit":
        return any(is_aos_precommit_release_input(path) for path in paths)
    if surface == "dev-base":
        return bool(affected_tiers(paths))
    raise ValueError(f"unknown release surface: {surface}")


def _changed_paths(base: str, head: str) -> tuple[str, ...]:
    probe = subprocess.run(
        [
            "git",
            "diff",
            "--name-only",
            "--no-renames",
            "--diff-filter=ACDMR",
            base,
            head,
            "--",
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    return tuple(line for line in probe.stdout.splitlines() if line)


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--surface",
        choices=("aos-cli", "aos-precommit", "dev-base"),
        required=True,
    )
    parser.add_argument("--base", default="", help="Base revision for the change.")
    parser.add_argument("--head", default="HEAD", help="Head revision for the change.")
    parser.add_argument(
        "--changed-path",
        action="append",
        default=[],
        help="Explicit changed path. May be repeated instead of --base.",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Require the release regardless of changed paths.",
    )
    parser.add_argument(
        "--github-output",
        default="",
        help="Optional GitHub or Forgejo Actions output file.",
    )
    args = parser.parse_args(argv)
    if args.base and args.changed_path:
        parser.error("--base and --changed-path cannot be combined")
    if not args.force and not args.base and not args.changed_path:
        parser.error("one of --base, --changed-path, or --force is required")
    return args


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    if args.changed_path:
        paths = tuple(args.changed_path)
    elif args.base:
        paths = _changed_paths(args.base, args.head)
    else:
        paths = ()
    required = args.force or release_required(args.surface, paths)
    matching_paths = (
        list(paths)
        if args.force
        else [
            path
            for path in paths
            if release_required(args.surface, (path,))
        ]
    )
    payload = {
        "surface": args.surface,
        "release_required": required,
        "changed_paths": list(paths),
        "matching_paths": matching_paths,
        "forced": args.force,
    }
    print(json.dumps(payload))
    if args.github_output:
        with open(args.github_output, "a", encoding="utf-8") as output:
            output.write(f"release_required={'true' if required else 'false'}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
