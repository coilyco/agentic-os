#!/usr/bin/env python3
"""Roll out the coilyco-flight-deck/agentic-os pre-commit hook suite to every catalog repo.

Inserts (or refreshes) a single managed `repo: <forgejo>/coilyco-flight-deck/agentic-os`
block in each consumer's `.pre-commit-config.yaml`. Block is delimited by marker
comments so re-runs are idempotent. Replaces the older per-hook stamping
rollouts that lived in coilyco-bridge/agentic-os-kai/scripts/.

For each repo checked out under ~/projects/<org>/<name> across every org dir
(coilysiren, coilyco-bridge, coilyco-flight-deck, post org-migration):
  1. Read or create `.pre-commit-config.yaml`.
  2. Strip legacy stamped `repo: local` blocks for the hooks now centralized
     here (catalog-block-present, catalog-doc-size, catalog-trifecta,
     documentation-layout, code-comments, check-skills, dead-cross-links,
     skill-discipline).
  3. Insert/refresh the managed block: the agentic-os hook set plus the
     standard hygiene hooks, actionlint, Forgejo Runner validation, shellcheck,
     and typos.
  4. Run `pre-commit install --hook-type pre-commit --hook-type commit-msg
     --hook-type prepare-commit-msg`.

Pin a package tag with `--rev`. Default tracks the latest aos-precommit release.

Usage:
    python3 scripts/apply-agentic-os-hooks.py             # apply to all
    python3 scripts/apply-agentic-os-hooks.py --dry-run   # show plan
    python3 scripts/apply-agentic-os-hooks.py --repo X    # one repo
    python3 scripts/apply-agentic-os-hooks.py --skip X Y  # exclude
    python3 scripts/apply-agentic-os-hooks.py --rev aos-precommit-v0.2.0

A repo carrying a .agentic-os-ignore file at its root is skipped entirely
(declarative, repo-owned opt-out). Use --skip for one-off exclusions, the
marker for durable ones. Honored fail-closed: presence skips, no override.

Drives off the on-disk checkout set via agentic_os.config.iter_workspace_repos
(every git working tree under ~/projects/<org>/*), so it is owner-agnostic:
the org migration of active repos to coilyco-bridge / coilyco-flight-deck no
longer strands them the way a single hardcoded root did. Override the root
with $PROJECTS_ROOT (e.g. PROJECTS_ROOT=X:/projects on Windows, where the
workspace lives off the home drive). See the workspace-root rollout notes and
the convention design in docs/features-release-tooling.md.
"""
from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from agentic_os import config as cfg  # noqa: E402

# Consumer pin is tag-derived at read time (see default_rev), not committed.
# FALLBACK_REV is the floor for tag-less checkouts. See docs/release.md.
FALLBACK_REV = "aos-precommit-v0.1.0"

REPO_ROOT = Path(__file__).resolve().parent.parent
VERSION_TAG_GLOB = "aos-precommit-v[0-9]*.[0-9]*.[0-9]*"
_VERSION_TAG_RE = re.compile(r"^aos-precommit-v\d+\.\d+\.\d+$")


def latest_release_tag() -> str | None:
    """Most recent aos-precommit-v<MAJOR>.<MINOR>.<PATCH> tag, or None.

    Reads git tags from the agentic-os checkout (REPO_ROOT), independent of the
    caller's cwd. Returns None when git is unavailable or no release tag is
    fetched (a shallow clone), letting default_rev() fall back to FALLBACK_REV.
    """
    try:
        out = subprocess.run(
            ["git", "tag", "--list", VERSION_TAG_GLOB, "--sort=-v:refname"],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            check=True,
        )
    except (subprocess.CalledProcessError, OSError):
        return None
    for line in out.stdout.splitlines():
        candidate = line.strip()
        if _VERSION_TAG_RE.match(candidate):
            return candidate
    return None


def default_rev() -> str:
    """The release tag consumers pin, resolved from git at runtime.

    Latest fetched tag, else the FALLBACK_REV floor. Derived rather than
    committed so the auto release pipeline cuts only a tag, never a per-push
    DEFAULT_REV bump commit.
    """
    return latest_release_tag() or FALLBACK_REV


# A repo carrying this marker at its root opts out of all baseline
# normalization, fail-closed. Remove the file to re-enroll.
IGNORE_MARKER = ".agentic-os-ignore"

BEGIN_MARKER = "# BEGIN managed by agentic-os/scripts/apply-agentic-os-hooks.py"
END_MARKER = "# END managed by agentic-os/scripts/apply-agentic-os-hooks.py"

# Canonical source for the hook suite. Forgejo, not the GitHub mirror: it is
# the source of truth and lands release tags first.
AGENTIC_OS_REPO_URL = "https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os"

# Upstream check-merge-conflict, displaced from the agentic-os catalog
# ; --assume-in-merge keeps the old always-scan behavior.
PRECOMMIT_HOOKS_REPO_URL = "https://github.com/pre-commit/pre-commit-hooks"
PRECOMMIT_HOOKS_REV = "v6.0.0"

PRECOMMIT_HOOKS = [
    {"id": "trailing-whitespace"},
    {"id": "end-of-file-fixer"},
    {"id": "check-added-large-files", "args": ["--maxkb=2048"]},
    {"id": "check-merge-conflict", "args": ["--assume-in-merge"]},
    {"id": "check-case-conflict"},
    {"id": "check-illegal-windows-names"},
    {"id": "mixed-line-ending"},
    {"id": "check-json"},
    {"id": "check-toml"},
]

ACTIONLINT_REPO_URL = "https://github.com/rhysd/actionlint"
ACTIONLINT_REV = "v1.7.12"
FORGEJO_WORKFLOW_FILES = r"^\.forgejo/workflows/.*\.(ya?ml)$"

FORGEJO_RUNNER_REPO_URL = "https://code.forgejo.org/forgejo/runner"
FORGEJO_RUNNER_REV = "v12.10.1"

SHELLCHECK_REPO_URL = "https://github.com/shellcheck-py/shellcheck-py"
SHELLCHECK_REV = "v0.11.0.1"
SHELLCHECK_EXCLUDE = r"^shell/(zshrc|warp\.zsh)$"

TYPOS_REPO_URL = "https://github.com/crate-ci/typos"
TYPOS_REV = "v1.48.0"

MANAGED_REPO_URLS = [
    PRECOMMIT_HOOKS_REPO_URL,
    ACTIONLINT_REPO_URL,
    FORGEJO_RUNNER_REPO_URL,
    SHELLCHECK_REPO_URL,
    TYPOS_REPO_URL,
]

# Legacy managed-block markers from the prior per-hook stamping rollouts.
# Strip these when present so consumers end up with one upstream-ref block.
LEGACY_BLOCK_MARKERS = [
    ("# BEGIN managed by agentic-os-kai/scripts/apply-catalog-block-hook.py",
     "# END managed by agentic-os-kai/scripts/apply-catalog-block-hook.py"),
    ("# BEGIN managed by agentic-os-kai/scripts/apply-catalog-doc-size-hook.py",
     "# END managed by agentic-os-kai/scripts/apply-catalog-doc-size-hook.py"),
    ("# BEGIN managed by agentic-os-kai/scripts/apply-catalog-trifecta-hook.py",
     "# END managed by agentic-os-kai/scripts/apply-catalog-trifecta-hook.py"),
    ("# BEGIN managed by agentic-os-kai/scripts/apply-skill-discipline-hooks.py",
     "# END managed by agentic-os-kai/scripts/apply-skill-discipline-hooks.py"),
    ("# BEGIN managed by agentic-os-kai/scripts/apply-commit-msg-hook.py",
     "# END managed by agentic-os-kai/scripts/apply-commit-msg-hook.py"),
]

# Legacy stamped scripts to delete; validators ship from aos-precommit now.
LEGACY_STAMPED_SCRIPTS = [
    "scripts/check-catalog-block.py",
    "scripts/check-catalog-doc-size.py",
    "scripts/check-catalog-trifecta.py",
    "scripts/check-dead-links.py",
    "scripts/check-skills.py",
]

# Default hook IDs per repo (hand-editable).
DEFAULT_HOOK_IDS = [
    "catalog-doc-size",
    "catalog-trifecta",
    "documentation-layout",
    "context-load-points",
    "code-comments",
    "actions-run-one-line",
    "source-doc-refs",
    "catalog-block-present",
    "check-skills",
    "dead-cross-links",
    "repo-pointer-skills",
    "misplaced-skills",
    "agent-compose-size",
    "agent-compose-dedup",
    "trufflehog",
]

# Per-repo hook opt-outs. eco-* repos skip code-comments (Unity / C# conventions).
# lore: docs-only / no-skills slice, subtracted set reproduces it.
PER_REPO_HOOK_SKIPS: dict[str, set[str]] = {
    "lore": {
        "catalog-doc-size",
        "check-skills",
        "repo-pointer-skills",
        "misplaced-skills",
        "agent-compose-size",
        "agent-compose-dedup",
    },
}
ECO_HOOK_SKIPS = {"code-comments"}


def hook_ids_for(repo: str) -> list[str]:
    skips: set[str] = set(PER_REPO_HOOK_SKIPS.get(repo, set()))
    if repo.startswith("eco"):
        skips |= ECO_HOOK_SKIPS
    return [h for h in DEFAULT_HOOK_IDS if h not in skips]


def managed_block(rev: str, hook_ids: list[str] | None = None) -> str:
    ids = hook_ids if hook_ids is not None else DEFAULT_HOOK_IDS
    hook_lines = "\n".join(f"      - id: {h}" for h in ids)
    precommit_hook_lines = "\n".join(
        "      - id: {id}{args}".format(
            id=hook["id"],
            args=(
                f"\n        args: [{', '.join(hook['args'])}]"
                if "args" in hook
                else ""
            ),
        )
        for hook in PRECOMMIT_HOOKS
    )
    return f"""\
  {BEGIN_MARKER}
  - repo: {AGENTIC_OS_REPO_URL}
    rev: {rev}
    hooks:
{hook_lines}
  - repo: {PRECOMMIT_HOOKS_REPO_URL}
    rev: {PRECOMMIT_HOOKS_REV}
    hooks:
{precommit_hook_lines}
  - repo: {ACTIONLINT_REPO_URL}
    rev: {ACTIONLINT_REV}
    hooks:
      # Forgejo workflows use GitHub Actions syntax; no exclude split is needed yet.
      - id: actionlint
        files: {FORGEJO_WORKFLOW_FILES}
  - repo: {FORGEJO_RUNNER_REPO_URL}
    rev: {FORGEJO_RUNNER_REV}
    hooks:
      - id: forgejo-runner-validate
  - repo: {SHELLCHECK_REPO_URL}
    rev: {SHELLCHECK_REV}
    hooks:
      - id: shellcheck
        exclude: {SHELLCHECK_EXCLUDE}
        args: [--severity=error]
  - repo: {TYPOS_REPO_URL}
    rev: {TYPOS_REV}
    hooks:
      - id: typos
        args: []
  {END_MARKER}
"""


def empty_config_template(rev: str, hook_ids: list[str] | None = None) -> str:
    return f"""\
repos:
{managed_block(rev, hook_ids)}"""


def list_local_repo_dirs() -> list[Path]:
    """Every git working tree checked out under ~/projects/<org>/*.

    Owner-agnostic by design: this is a local-fleet tool (it runs
    `pre-commit install` inside each checkout), so the on-disk set is both
    the authoritative candidate list and the only set it can act on. Driving
    off disk via config.iter_workspace_repos() instead of `gh repo list
    <single-owner>` (or a single hardcoded org dir) means the org migration
    (coilyco-bridge / coilyco-flight-deck) can't silently strand repos.
    apply_to_repo() still filters the source repo, the opt-out marker, and
    non-git dirs; --skip handles one-off exclusions.
    """
    return cfg.iter_workspace_repos()


def strip_legacy_blocks(text: str) -> tuple[str, int]:
    """Drop every legacy per-hook managed block. Returns (new_text, n_removed)."""
    removed = 0
    for begin, end in LEGACY_BLOCK_MARKERS:
        pattern = re.compile(
            re.escape(begin) + r".*?" + re.escape(end) + r"\n?",
            re.DOTALL,
        )
        new_text, n = pattern.subn("", text)
        if n:
            removed += n
            text = new_text
    for repo_url in MANAGED_REPO_URLS:
        lines = text.splitlines(keepends=True)
        new_lines: list[str] = []
        skipping = False
        stripped_here = 0
        for line in lines:
            if skipping:
                if re.match(r"^\s{2}-\s+repo:\s+", line):
                    skipping = False
                else:
                    continue
            if re.match(rf"^\s{{2}}-\s+repo:\s+{re.escape(repo_url)}\s*$", line):
                skipping = True
                stripped_here += 1
                continue
            new_lines.append(line)
        if stripped_here:
            removed += stripped_here
            text = "".join(new_lines)
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text, removed


def upsert_managed_block(
    config_path: Path, rev: str, hook_ids: list[str] | None = None
) -> tuple[str, int]:
    """Insert or refresh the agentic-os upstream-ref block.

    Returns (status, legacy_blocks_removed).
    """
    if not config_path.exists():
        config_path.write_text(empty_config_template(rev, hook_ids))
        return "created", 0

    original_text = config_path.read_text()
    block = managed_block(rev, hook_ids)
    if BEGIN_MARKER in original_text and END_MARKER in original_text:
        before, _, rest = original_text.partition(BEGIN_MARKER)
        _, _, after = rest.partition(END_MARKER)
        before, removed_before = strip_legacy_blocks(before)
        after, removed_after = strip_legacy_blocks(after)
        legacy_removed = removed_before + removed_after
        before = before.rstrip()
        after = after.lstrip("\n")
        new_text = before + "\n\n" + block + (after if after else "")
        if new_text == original_text:
            return "unchanged", legacy_removed
        config_path.write_text(new_text)
        return "updated", legacy_removed

    text, legacy_removed = strip_legacy_blocks(original_text)
    if not text.endswith("\n"):
        text += "\n"
    text += block
    config_path.write_text(text)
    return "appended", legacy_removed


def drop_legacy_stamped_scripts(repo_dir: Path) -> list[str]:
    """Delete stamped check-*.py copies from the consumer's scripts/ dir."""
    dropped: list[str] = []
    for rel in LEGACY_STAMPED_SCRIPTS:
        p = repo_dir / rel
        if p.is_file():
            p.unlink()
            dropped.append(rel)
    return dropped


def install_pre_commit_hooks(repo_dir: Path) -> str:
    result = subprocess.run(
        [
            "pre-commit", "install",
            "--hook-type", "pre-commit",
            "--hook-type", "commit-msg",
            "--hook-type", "prepare-commit-msg",
        ],
        cwd=repo_dir, capture_output=True, text=True, check=False,
    )
    if result.returncode != 0:
        return f"install-failed: {result.stderr.strip()}"
    return "installed"


def apply_to_repo(repo_dir: Path, rev: str, dry_run: bool) -> tuple[str, str]:
    repo = repo_dir.name
    if repo == "agentic-os":
        # Source repo dogfoods via repo: local; upstream-ref would duplicate IDs.
        return ("skipped", "self (source repo)")
    if not repo_dir.is_dir():
        return ("skipped", "not checked out locally")
    if not (repo_dir / ".git").exists():
        return ("skipped", "not a git working tree")
    if (repo_dir / IGNORE_MARKER).exists():
        return ("skipped", f"opted out ({IGNORE_MARKER})")

    config_path = repo_dir / ".pre-commit-config.yaml"
    hook_ids = hook_ids_for(repo)

    if dry_run:
        if not config_path.exists():
            yaml_status = "would create config"
        else:
            text = config_path.read_text()
            n_legacy = sum(
                1 for begin, _ in LEGACY_BLOCK_MARKERS if begin in text
            )
            has_managed = BEGIN_MARKER in text
            parts = []
            if has_managed:
                parts.append("refresh agentic-os block")
            else:
                parts.append("insert agentic-os block")
            if n_legacy:
                parts.append(f"strip {n_legacy} legacy block(s)")
            n_stamped = sum(
                1 for rel in LEGACY_STAMPED_SCRIPTS if (repo_dir / rel).is_file()
            )
            if n_stamped:
                parts.append(f"drop {n_stamped} stamped script(s)")
            yaml_status = ", ".join(parts)
        return ("dryrun", yaml_status)

    yaml_status, legacy_removed = upsert_managed_block(config_path, rev, hook_ids)
    dropped = drop_legacy_stamped_scripts(repo_dir)
    install_status = install_pre_commit_hooks(repo_dir)
    parts = [yaml_status]
    if legacy_removed:
        parts.append(f"legacy-blocks={legacy_removed}")
    if dropped:
        parts.append(f"dropped={len(dropped)}")
    parts.append(install_status)
    return ("applied", ", ".join(parts))


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--repo", help="apply to a single repo by name")
    ap.add_argument("--skip", nargs="*", default=[])
    ap.add_argument(
        "--rev",
        default=None,
        help="aos-precommit release tag to pin (default: latest package tag, "
        f"else {FALLBACK_REV})",
    )
    args = ap.parse_args(argv)
    if args.rev is None:
        args.rev = default_rev()

    all_dirs = list_local_repo_dirs()
    if args.repo:
        repos = [d for d in all_dirs if d.name == args.repo]
        if not repos:
            print(
                f"No checked-out repo named {args.repo!r} under "
                f"{cfg.projects_root()}"
            )
            return 1
    else:
        skip = set(args.skip)
        repos = [d for d in all_dirs if d.name not in skip]

    print(
        f"Rolling out aos-precommit suite "
        f"(rev={args.rev}) to {len(repos)} repo(s)"
    )
    if args.dry_run:
        print("(dry run)")
    print()

    counts: dict[str, int] = {}
    for repo_dir in repos:
        action, detail = apply_to_repo(repo_dir, args.rev, args.dry_run)
        counts[action] = counts.get(action, 0) + 1
        print(f"  {repo_dir.name:24} {action:8} {detail}")

    print()
    print("Summary:", ", ".join(f"{k}={v}" for k, v in counts.items()))
    return 0


if __name__ == "__main__":
    sys.exit(main())
