#!/usr/bin/env python3
"""Apply this repo's host shell entry-point symlinks and git wire-up.

A local repair path for the shell half of the ansible shell role. It does not
replace fleet convergence, it fixes one host when links drift.
"""
from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple


REPO_ROOT = Path(__file__).resolve().parent.parent


class LinkSpec(NamedTuple):
    name: str
    source: Path
    dest: Path


class ConfigSpec(NamedTuple):
    key: str
    value: str
    config_path: Path


def native_session_root(path: Path) -> Path | None:
    """Return the `.../aos/native` ancestor of path, when it has one.

    A native session home and its shadow worktrees live under a temporary root
    that outlives no session. Absolute paths taken from one are durable enough
    to write into git config and not durable enough to resolve later, which is
    the whole failure this script guards (agentic-os#1137).
    """
    parts = path.parts
    for index in range(len(parts) - 1):
        if parts[index] == "aos" and parts[index + 1] == "native":
            return Path(*parts[: index + 2])
    return None


def canonical_home(home: Path) -> Path:
    """Resolve a session home to the durable host home it links back to.

    A native session home mirrors the real one entry by entry as symlinks, so
    any of them names the durable parent.
    """
    if native_session_root(home) is None:
        return home
    for probe in (".gitconfig", ".local", ".zshrc"):
        entry = home / probe
        if not entry.is_symlink():
            continue
        parent = entry.resolve().parent
        if native_session_root(parent) is None:
            return parent
    return home


def _target_for_gpg_ssm(repo_root: Path) -> Path:
    if os.name == "nt":
        return repo_root / "scripts" / "gpg-ssm.cmd"
    return repo_root / "scripts" / "gpg-ssm"


def _target_for_git_credential(repo_root: Path) -> Path:
    if os.name == "nt":
        return repo_root / "scripts" / "git-credential-forgejo-ssm.cmd"
    return repo_root / "scripts" / "git-credential-forgejo-ssm.sh"


def _target_for_docker_credential(repo_root: Path) -> Path:
    if os.name == "nt":
        return repo_root / "scripts" / "docker-credential-forgejo-ssm.cmd"
    return repo_root / "scripts" / "docker-credential-forgejo-ssm"


def link_specs(home: Path, repo_root: Path = REPO_ROOT) -> list[LinkSpec]:
    specs = [
        LinkSpec("zshrc", repo_root / "shell" / "zshrc", home / ".zshrc"),
        LinkSpec(
            "gpg-ssm",
            _target_for_gpg_ssm(repo_root),
            home / ".local" / "bin" / _target_for_gpg_ssm(repo_root).name,
        ),
        LinkSpec(
            "ssm-get",
            repo_root / "scripts" / "ssm-get",
            home / ".local" / "bin" / "ssm-get",
        ),
        LinkSpec(
            "git-credential-forgejo-ssm",
            _target_for_git_credential(repo_root),
            home / ".local" / "bin" / _target_for_git_credential(repo_root).name,
        ),
        LinkSpec(
            "docker-credential-forgejo-ssm",
            _target_for_docker_credential(repo_root),
            home / ".local" / "bin" / _target_for_docker_credential(repo_root).name,
        ),
    ]
    if os.name == "nt":
        specs.extend(
            [
                LinkSpec(
                    "gpg-ssm-bash",
                    repo_root / "scripts" / "gpg-ssm",
                    home / ".local" / "bin" / "gpg-ssm",
                ),
                LinkSpec(
                    "git-credential-forgejo-ssm-bash",
                    repo_root / "scripts" / "git-credential-forgejo-ssm.sh",
                    home / ".local" / "bin" / "git-credential-forgejo-ssm.sh",
                ),
                LinkSpec(
                    "docker-credential-forgejo-ssm-bash",
                    repo_root / "scripts" / "docker-credential-forgejo-ssm",
                    home / ".local" / "bin" / "docker-credential-forgejo-ssm",
                ),
            ]
        )
    else:
        specs.insert(1, LinkSpec("bashrc", repo_root / "shell" / "bashrc", home / ".bashrc"))
        # kitty does not ship on Windows, and the ansible shell role links
        # alacritty only, so this is the one place the kitty config installs.
        specs.append(
            LinkSpec(
                "kitty",
                repo_root / "kitty" / "kitty.conf",
                home / ".config" / "kitty" / "kitty.conf",
            )
        )
    return specs


def config_specs(home: Path, repo_root: Path = REPO_ROOT) -> list[ConfigSpec]:
    """Git settings whose value is a path this script also owns.

    The value names the durable home rather than `$HOME`, so wiring this up
    from inside a native session cannot record a path the session takes with
    it when it is purged.
    """
    durable = canonical_home(home)
    signer = durable / ".local" / "bin" / _target_for_gpg_ssm(repo_root).name
    return [
        ConfigSpec("gpg.program", str(signer), home / ".gitconfig"),
    ]


def _git_config_get(spec: ConfigSpec) -> str | None:
    result = subprocess.run(
        ["git", "config", "--file", str(spec.config_path), "--get", spec.key],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        return None
    return result.stdout.strip() or None


def apply_config(spec: ConfigSpec, *, dry_run: bool) -> tuple[str, str]:
    target = Path(spec.value)
    if not target.exists():
        return "failed", f"missing signer {target}; link it first"

    current = _git_config_get(spec)
    if current == spec.value:
        return "ok", "already current"
    if not dry_run:
        spec.config_path.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            ["git", "config", "--file", str(spec.config_path), spec.key, spec.value],
            check=True,
        )
    if current is None:
        return ("would-set" if dry_run else "set"), spec.value
    detail = f"{current} -> {spec.value}"
    if native_session_root(Path(current)) is not None:
        detail = f"purged session path {detail}"
    return ("would-repoint" if dry_run else "repointed"), detail


def _backup_path(path: Path) -> Path:
    base = path.with_name(path.name + ".bak")
    if not base.exists() and not base.is_symlink():
        return base
    n = 2
    while True:
        candidate = path.with_name(f"{path.name}.bak.{n}")
        if not candidate.exists() and not candidate.is_symlink():
            return candidate
        n += 1


def _link_target(dest: Path) -> Path:
    # Windows readlink() can return a \\?\-prefixed extended-length path,
    # which never compares equal to the plain source path.
    target = dest.readlink()
    text = os.fspath(target)
    if text.startswith("\\\\?\\"):
        return Path(text[4:])
    return target


def apply_link(spec: LinkSpec, *, dry_run: bool) -> tuple[str, str]:
    if not spec.source.exists():
        return "failed", f"missing source {spec.source}"

    if spec.dest.is_symlink():
        current = _link_target(spec.dest)
        if not current.is_absolute():
            current = (spec.dest.parent / current).resolve()
        if current == spec.source:
            return "ok", "already current"
        if not dry_run:
            spec.dest.unlink()
            spec.dest.symlink_to(spec.source)
        return ("would-repoint" if dry_run else "repointed"), str(spec.source)

    if spec.dest.exists():
        if spec.dest.is_dir():
            return "failed", f"destination is a directory: {spec.dest}"
        backup = _backup_path(spec.dest)
        if not dry_run:
            backup.parent.mkdir(parents=True, exist_ok=True)
            spec.dest.rename(backup)
            spec.dest.parent.mkdir(parents=True, exist_ok=True)
            spec.dest.symlink_to(spec.source)
        return (
            "would-backup" if dry_run else "backed-up",
            f"{spec.dest} -> {backup}; linked {spec.source}",
        )

    if not dry_run:
        spec.dest.parent.mkdir(parents=True, exist_ok=True)
        spec.dest.symlink_to(spec.source)
    return ("would-link" if dry_run else "linked"), str(spec.source)


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--check", action="store_true", help="fail if any link would change")
    ap.add_argument("--home", type=Path, default=Path.home(), help=argparse.SUPPRESS)
    ap.add_argument("--repo-root", type=Path, default=REPO_ROOT, help=argparse.SUPPRESS)
    args = ap.parse_args(argv)

    dry_run = args.dry_run or args.check
    session = native_session_root(args.repo_root)
    if session is not None:
        print(
            f"Refusing to apply from a native-session checkout under {session}.\n"
            "Every link and setting would name a path the session takes with it.\n"
            "Re-run from the canonical checkout.",
            file=sys.stderr,
        )
        return 1

    specs = link_specs(args.home, args.repo_root)
    settings = config_specs(args.home, args.repo_root)
    print(f"Applying shell links for {args.home}")
    if args.dry_run:
        print("(dry run)")
    if args.check:
        print("(check)")
    print()

    counts: dict[str, int] = {}
    changed = False
    failed = False
    for spec in specs:
        action, detail = apply_link(spec, dry_run=dry_run)
        counts[action] = counts.get(action, 0) + 1
        print(f"  {spec.name:8} {action:14} {detail}")
        failed = failed or action == "failed"
        changed = changed or action.startswith("would-")
    for setting in settings:
        action, detail = apply_config(setting, dry_run=dry_run)
        counts[action] = counts.get(action, 0) + 1
        print(f"  {setting.key:8} {action:14} {detail}")
        failed = failed or action == "failed"
        changed = changed or action.startswith("would-")

    print()
    print("Summary:", ", ".join(f"{k}={v}" for k, v in sorted(counts.items())))
    if failed:
        return 1
    if args.check and changed:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
