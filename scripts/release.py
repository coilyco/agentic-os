#!/usr/bin/env python3
"""Local release helper: bump version, sign tag, push.

Bumps the latest semver tag (minor by default), rewrites `pyproject.toml`
version and the `FALLBACK_REV` floor in
`scripts/apply-agentic-os-hooks.py`, commits the bump, creates a signed
annotated tag, and pushes both.

This is the HAND-CUT path for the `aos-precommit-v*` train. The automatic
release workflow normally cuts that train only when installed hook inputs
change. The
version-bump commit here carries a `[skip ci]` marker so it does not start
the main-push workflows. The consumer pin resolves from package tags at read
time, with no per-push pin commit. This path additionally
reconciles pyproject `version` + `uv.lock` and refreshes the `FALLBACK_REV`
floor, so run it for a hand-driven package major or whenever those drift.

Usage:
    python3 scripts/release.py [--bump {major|minor|patch}] [--dry-run]

Bump policy: every release is a minor bump unless --bump says otherwise.
Major is hand-driven only (--bump major) - commit messages are never
parsed to decide the bump.
"""
from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
PYPROJECT = REPO_ROOT / "pyproject.toml"
APPLY_HOOKS = REPO_ROOT / "scripts" / "apply-agentic-os-hooks.py"
TAG_PREFIX = "aos-precommit-v"
VERSION_RE = re.compile(rf"^{TAG_PREFIX}(\d+)\.(\d+)\.(\d+)$")
PYPROJECT_VERSION_RE = re.compile(r'^version = "(\d+\.\d+\.\d+)"$', re.MULTILINE)
FALLBACK_REV_RE = re.compile(
    r'^FALLBACK_REV = "aos-precommit-v\d+\.\d+\.\d+"$',
    re.MULTILINE,
)
DEFAULT_BUMP = "minor"


def run(cmd: list[str], **kwargs) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, cwd=REPO_ROOT, check=True, text=True, capture_output=True, **kwargs)


def latest_tag() -> str | None:
    """Most recent aos-precommit semver tag, sorted by version."""
    out = run(["git", "tag", "--list", f"{TAG_PREFIX}[0-9]*.[0-9]*.[0-9]*", "--sort=-v:refname"])
    for line in out.stdout.splitlines():
        if VERSION_RE.match(line.strip()):
            return line.strip()
    return None


def commits_since(tag: str | None) -> list[str]:
    """Return commit subjects since `tag` (or all if None), for the changelog
    and the release-is-empty check. Subjects are not parsed for bump signals."""
    rng = f"{tag}..HEAD" if tag else "HEAD"
    out = run(["git", "log", rng, "--pretty=format:%s"])
    return [line.strip() for line in out.stdout.splitlines() if line.strip()]


def next_version(prev: str | None, bump: str) -> str:
    if prev is None:
        major, minor, patch = 0, 0, 0
    else:
        m = VERSION_RE.match(prev)
        if not m:
            raise SystemExit(f"latest tag {prev!r} does not parse as semver")
        major, minor, patch = (int(x) for x in m.groups())
    if bump == "major":
        major, minor, patch = major + 1, 0, 0
    elif bump == "minor":
        minor, patch = minor + 1, 0
    elif bump == "patch":
        patch += 1
    else:
        raise SystemExit(f"unknown bump {bump!r}")
    return f"{major}.{minor}.{patch}"


def bump_pyproject(new_ver: str) -> bool:
    """Rewrite pyproject.toml version. Returns True if changed."""
    text = PYPROJECT.read_text(encoding="utf-8")
    new_text, n = PYPROJECT_VERSION_RE.subn(f'version = "{new_ver}"', text, count=1)
    if n == 0:
        raise SystemExit("could not find version = \"...\" line in pyproject.toml")
    if new_text == text:
        return False
    PYPROJECT.write_text(new_text, encoding="utf-8")
    return True


def relock() -> bool:
    """Re-resolve uv.lock so its editable self-version tracks pyproject.

    uv.lock pins this package's own version, so a pyproject bump leaves the
    lock a version behind. Untouched, the next `uv run` re-syncs and dirties
    the tree (and `uv run python scripts/release.py` then refuses on its own
    dirty check). Relocking here folds the lock bump into the release commit.
    Returns True when uv.lock changed.
    """
    run(["uv", "lock"])
    out = run(["git", "status", "--porcelain", "uv.lock"])
    return out.stdout.strip() != ""


def bump_fallback_rev(new_tag: str) -> bool:
    """Rewrite FALLBACK_REV in apply-agentic-os-hooks.py. Returns True if changed.

    The consumer pin resolves from git tags at read time (agentic-os#238), so
    no per-release bump commit is needed. FALLBACK_REV is just the floor for
    tag-less checkouts; refreshing it on each hand-cut release keeps that floor
    from drifting far behind the latest tag.
    """
    text = APPLY_HOOKS.read_text(encoding="utf-8")
    new_text, n = FALLBACK_REV_RE.subn(f'FALLBACK_REV = "{new_tag}"', text, count=1)
    if n == 0:
        raise SystemExit("could not find FALLBACK_REV line in apply-agentic-os-hooks.py")
    if new_text == text:
        return False
    APPLY_HOOKS.write_text(new_text, encoding="utf-8")
    return True


def working_tree_clean() -> bool:
    out = run(["git", "status", "--porcelain"])
    return out.stdout.strip() == ""


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bump", choices=("major", "minor", "patch"), help="Bump to apply. Defaults to minor; pass major for a hand-driven major bump.")
    parser.add_argument("--dry-run", action="store_true", help="Compute and report, do not write.")
    parser.add_argument("--allow-dirty", action="store_true", help="Allow uncommitted changes (risky).")
    args = parser.parse_args()

    if not args.allow_dirty and not working_tree_clean():
        print("FAIL: working tree is dirty. Commit or stash first, or pass --allow-dirty.", file=sys.stderr)
        return 1

    prev = latest_tag()
    subjects = commits_since(prev)
    if not subjects and not args.bump:
        print(f"No commits since {prev or 'beginning'}. Nothing to release.")
        return 0

    bump = args.bump or DEFAULT_BUMP

    new_ver = next_version(prev, bump)
    new_tag = f"{TAG_PREFIX}{new_ver}"

    print(f"Previous tag: {prev or '(none)'}")
    print(f"Commits since: {len(subjects)}")
    print(f"Bump: {bump}{'' if args.bump else ' (default)'}")
    print(f"Next version:  {new_tag}")

    if args.dry_run:
        print("Dry run; no changes written.")
        return 0

    changed_files: list[str] = []
    if bump_pyproject(new_ver):
        changed_files.append("pyproject.toml")
    if bump_fallback_rev(new_tag):
        changed_files.append("scripts/apply-agentic-os-hooks.py")
    if relock():
        changed_files.append("uv.lock")

    if changed_files:
        run(["git", "add", *changed_files])
        # [skip ci] keeps the hand-cut package release out of main workflows.
        run(["git", "commit", "-m", f"chore: bump version to {new_tag} [skip ci]"])
        print(f"Committed version bump touching: {', '.join(changed_files)}")

    summary = subjects[0] if subjects else f"release {new_tag}"
    tag_msg = f"Release {new_tag}: {summary}"
    run(["git", "tag", "-a", "-s", new_tag, "-m", tag_msg])
    print(f"Created signed tag {new_tag}")

    run(["git", "push"])
    run(["git", "push", "--tags"])
    print(f"Pushed commit and tag {new_tag}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
