#!/usr/bin/env python3
"""Assert every catalog repo's .pre-commit-config.yaml carries the expected hook ids.

The expected set is what the applier would write for that repo, read from
agentic_os.hook_catalog rather than re-derived, so the two cannot disagree
silently. Walks every git working tree under ~/projects/<org>/*,
or `--source github` to query the contents API. Override the root with
$PROJECTS_ROOT. Run it after an apply-agentic-os-hooks.py sweep to verify every
consumer landed the managed block.
"""
from __future__ import annotations

import argparse
import base64
import json
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO_ROOT))

try:
    import yaml
except ModuleNotFoundError:
    sys.stderr.write(
        "audit-pre-commit-coverage: PyYAML required. pip install pyyaml\n"
    )
    sys.exit(2)

from agentic_os import config as cfg  # noqa: E402
from agentic_os import hook_catalog  # noqa: E402
from agentic_os import freshness  # noqa: E402

OWNER = "coilysiren"
AGENTIC_OS_URL = "https://github.com/coilysiren/agentic-os"


def gh(*args: str) -> str:
    result = subprocess.run(
        ["gh", *args], capture_output=True, text=True, check=False
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"gh {' '.join(args)!r} failed (rc={result.returncode}): "
            f"{result.stderr.strip()}"
        )
    return result.stdout


def list_active_repos() -> list[str]:
    out = gh(
        "repo", "list", OWNER,
        "--limit", "200",
        "--no-archived",
        "--source",
        "--json", "name",
        "--jq", ".[].name",
    )
    return [line.strip() for line in out.splitlines() if line.strip()]


def read_local_config(repo_dir: Path) -> str | None:
    path = repo_dir / ".pre-commit-config.yaml"
    if not path.is_file():
        return None
    return path.read_text(encoding="utf-8", errors="replace")


def read_remote_config(repo: str) -> str | None:
    try:
        payload_raw = gh(
            "api", f"repos/{OWNER}/{repo}/contents/.pre-commit-config.yaml"
        )
    except RuntimeError as exc:
        if "404" in str(exc) or "Not Found" in str(exc):
            return None
        raise
    payload: dict[str, Any] = json.loads(payload_raw)
    if payload.get("type") != "file":
        return None
    encoded = payload.get("content", "").replace("\n", "")
    if not encoded:
        return ""
    try:
        return base64.b64decode(encoded).decode("utf-8")
    except UnicodeDecodeError:
        return None


def _is_agentic_os_repo(repo_url: str) -> bool:
    """Match the agentic-os upstream-ref entry regardless of host or owner.

    The managed block points at the Forgejo mirror
    (forgejo.coilysiren.me/coilyco-flight-deck/agentic-os) while the GitHub
    mirror uses github.com/coilysiren/agentic-os. Match on the trailing
    `/agentic-os` path component so either lands.
    """
    return repo_url.rstrip("/").endswith("/agentic-os")


def referenced_hook_ids(config_text: str) -> set[str]:
    """Hook ids this config runs, from the agentic-os ref block or from local.

    agentic-os wires all of its own validators as `repo: local`, so an
    upstream-ref-only read reported the authoring repo as missing every hook it
    defines (agentic-os#7628). A locally wired id runs the same check.
    """
    try:
        data = yaml.safe_load(config_text)
    except yaml.YAMLError:
        return set()
    out: set[str] = set()
    for entry in (data or {}).get("repos") or []:
        if not isinstance(entry, dict):
            continue
        repo = entry.get("repo", "")
        if not _is_agentic_os_repo(repo) and repo != "local":
            continue
        for hook in entry.get("hooks") or []:
            if isinstance(hook, dict) and "id" in hook:
                out.add(hook["id"])
    return out


def audit_config(
    repo: str, config_text: str | None, expected: list[str]
) -> dict[str, Any]:
    if config_text is None:
        return {"repo": repo, "status": "no-config", "missing": expected}
    referenced = referenced_hook_ids(config_text)
    missing = [h for h in expected if h not in referenced]
    if not missing:
        return {"repo": repo, "status": "ok", "missing": []}
    return {"repo": repo, "status": "missing", "missing": missing}


def _print_freshness(dirs: list[Path]) -> None:
    """Stamp the read, because these counts get quoted into records and PRs.

    A checkout answers from its last fetch, so a clean number taken from a
    stale one is wrong and says nothing about it (agentic-os#7632).
    """
    now = datetime.now(timezone.utc)
    print(f"Read at: {now.strftime('%Y-%m-%dT%H:%M:%SZ')}, against local checkouts")
    rows = [
        freshness.freshness_line(d.name, fresh, now)
        for d in dirs
        if freshness.stale_enough_to_mention(fresh := freshness.checkout_freshness(d))
    ]
    if not rows:
        return
    print(
        f"  {len(rows)} of {len(dirs)} checkout(s) are behind or unfetched, so these "
        "numbers are as of them rather than as of the fleet:"
    )
    for row in rows:
        print(f"    {row}")


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    ap.add_argument(
        "--source", choices=["local", "github"], default="local",
        help="local: walk ~/projects/<org>/* on disk (default). github: query the contents API."
    )
    ap.add_argument("--repo", help="audit a single repo by name")
    ap.add_argument("--skip", nargs="*", default=[])
    args = ap.parse_args(argv)

    skip = set(args.skip)
    base = hook_catalog.DEFAULT_HOOK_IDS

    # Local mode drives off the on-disk checkout set so it spans every org dir.
    # Github mode keeps querying the coilysiren owner via the contents API.
    results = []
    unarmed: list[tuple[str, list[str]]] = []
    if args.source == "local":
        dirs = cfg.iter_workspace_repos()
        if args.repo:
            dirs = [d for d in dirs if d.name == args.repo]
        else:
            dirs = [d for d in dirs if d.name not in skip]
        print(
            f"Auditing {len(dirs)} repo(s) against the {len(base)} hook(s) the "
            f"applier ships, per repo after its skips ({AGENTIC_OS_URL})"
        )
        print(f"Source: {args.source}")
        _print_freshness(dirs)
        print()
        for d in dirs:
            out, why = hook_catalog.opted_out(d)
            if out:
                results.append(
                    {"repo": d.name, "status": "exempt", "missing": [], "why": why}
                )
                continue
            config_text = read_local_config(d)
            results.append(
                audit_config(d.name, config_text, hook_catalog.hook_ids_for(d.name))
            )
            # Needs the filesystem, so github mode cannot answer it.
            referenced = referenced_hook_ids(config_text or "")
            found = hook_catalog.unarmed_spec_hooks(d, referenced)
            if found:
                unarmed.append((d.name, found))
    else:
        names = [args.repo] if args.repo else [
            r for r in list_active_repos() if r not in skip
        ]
        print(
            f"Auditing {len(names)} repo(s) against the {len(base)} hook(s) the "
            f"applier ships, per repo after its skips ({AGENTIC_OS_URL})"
        )
        print(f"Source: {args.source}")
        print()
        for name in names:
            try:
                results.append(
                    audit_config(
                        name,
                        read_remote_config(name),
                        hook_catalog.hook_ids_for(name),
                    )
                )
            except RuntimeError as exc:
                results.append(
                    {"repo": name, "status": "error", "missing": [], "error": str(exc)}
                )

    by_status: dict[str, list[dict[str, Any]]] = {}
    for r in results:
        by_status.setdefault(r["status"], []).append(r)

    for status in ("ok", "exempt", "missing", "no-config", "error"):
        entries = by_status.get(status, [])
        if not entries:
            continue
        print(f"== {status} ({len(entries)}) ==")
        for r in entries:
            if r["missing"]:
                print(f"  {r['repo']:28} missing: {', '.join(r['missing'])}")
            elif r.get("why"):
                print(f"  {r['repo']:28} {r['why']}")
            elif r.get("error"):
                print(f"  {r['repo']:28} error: {r['error']}")
            else:
                print(f"  {r['repo']:28}")
        print()

    # Runs, passes, and evaluates nothing, so it reads as coverage in every
    # surface except a filesystem look. Remediation is agentic-os#7770.
    if unarmed:
        print(f"== unarmed ({len(unarmed)}) ==")
        for name, ids in unarmed:
            print(f"  {name:28} {', '.join(ids)}")
        print()

    # A shipped id that cannot fire is one defect for the whole fleet rather
    # than a finding per repo, so it prints once and still fails the run.
    inert = hook_catalog.inert_shipped_ids()
    undeclared = hook_catalog.undeclared_shipped_ids()
    if inert or undeclared:
        print("== catalog (1) ==")
        if inert:
            print(
                f"  shipped but manual-only, so never runs in a consumer: "
                f"{', '.join(inert)}"
            )
        if undeclared:
            print(
                f"  shipped but undeclared in .pre-commit-hooks.yaml: "
                f"{', '.join(undeclared)}"
            )
        print()

    bad_repos = (
        len(by_status.get("missing", []))
        + len(by_status.get("no-config", []))
        + len(by_status.get("error", []))
    )
    if bad_repos:
        print(f"Coverage incomplete: {bad_repos} repo(s) need attention.")
    if inert or undeclared:
        print("Catalog incomplete: the shipped set carries a hook that cannot run.")
    if unarmed:
        print(
            f"Enforcement incomplete: {len(unarmed)} repo(s) run a hook with no "
            f"spec for it to read."
        )
    if bad_repos or inert or undeclared or unarmed:
        return 1
    print("Coverage complete.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
