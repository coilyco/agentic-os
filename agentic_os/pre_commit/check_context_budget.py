#!/usr/bin/env python3
"""Report the eager startup context each harness loads, on demand.

This measures everything a harness ingests at session start across three axes
that each have a different growth lever:

  * doc    - the installed AGENTS.md/CLAUDE.md load point. Measures that file
             directly, so the bytes match what the harness receives. Lever:
             edit the inputs owned by Agent Compose.
  * skills - every mounted skill's SKILL.md *frontmatter* (name + description) is
             eager so the model knows the skill exists; bodies load lazily on
             invoke. With a large skill surface this is routinely the BIGGEST
             axis, larger than the composed doc. Lever: prune the skill set.
  * mcp    - native MCP tool schemas. One mcporter inventory is projected into
             each native harness registry, where schema discovery is deferred.
             `mcporter call` remains the CLI fallback. The eager figure is near
             zero and is reported as a server-count note, not a token sum.

The three axes above are the *proactive* tier - eager prompt bytes, per harness.
Two further tiers are cheap-to-provide context a driver can reach one tool call
away, measured per working-dir/reference clone rather than per harness:

  * immediate  - a working-dir clone (`/workspace/<name>`): a grep surface, not
                 in the prompt. `immediate_walk` reports tracked-file count,
                 bytes, and the chars/4 token proxy over `git ls-files` (tracked
                 only, so build/vendor/untracked trees do not inflate it).
  * peripheral - the reference repos (`/substrate/<name>`): the same walker
                 applied per-repo across a set, plus a total. `peripheral_walk`
                 takes the repo set from its caller and stays role-agnostic.

`immediate_walk` / `peripheral_walk` are reusable primitives behind the same
`count_tokens` proxy, for ward's role-aware three-tier probe (ward#373) to call
for tiers 2/3 while reusing the doc/skill accounting above for tier 1. This
layer measures; it does not model ward roles, containers, or substrate sets -
those stay in ward, which owns them and passes the repo paths in.

Skill scope is cwd-dependent: `~/.claude/skills` is emptied by mount-skills.sh
and skills are symlinked into each repo's `.claude/skills`, so the eager set is
the global plugin skills plus the scoped skills discoverable from the cwd. The
tool dedups by resolved path, so the one canonical skill set mounted into many
repos counts once.

Token counting (v1): a deterministic chars/4 proxy. tiktoken has no qwen encoding
and the qwen BPE needs its vocab assets, so v1 ships a hermetic proxy behind
`count_tokens`; swapping in a real tokenizer is a one-function change. ~10% off
absolute but consistent across harnesses.

Skill roots are global (host-wide, not repo-scoped): module defaults, the modern
`skill_load_points:` projection, and the legacy `skill_roots:` override determine
them.

Usage:
    check-context-budget                  # report installed harness context
    check-context-budget --role sysadmin --snapshot context-budget-ops-before.yaml
    check-context-budget --role sysadmin --compare context-budget-ops-before.yaml \
        --snapshot context-budget-ops-after.yaml
"""
from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path
from typing import Iterable, NamedTuple

from agentic_os.config import iter_workspace_repos
from agentic_os.context_budget_tokens import (
    TOKENIZER_NOTE,
    count_tokens,
)
from agentic_os.frontmatter import split_frontmatter

HOME = Path.home()
CONFIG_PATH = HOME / ".config" / "agent-compose" / "agent-compose.yaml"
DEFAULT_LOAD_POINTS = {
    "claude": HOME / ".claude" / "CLAUDE.md",
    "codex": HOME / ".codex" / "AGENTS.md",
}

# Per-harness skill roots scanned for SKILL.md frontmatter; absolute = global
# plugins, relative = expanded against cwd + workspace repos (docs/context-budget.md).
DEFAULT_SKILL_ROOTS = {
    "claude": ["~/.claude/plugins", ".claude/skills"],
    "codex": ["~/.agents/skills", ".agents/skills"],
    "goose": ["~/.agents/skills", ".agents/skills"],
    "opencode": ["~/.agents/skills", ".agents/skills"],
}

class TierWalk(NamedTuple):
    """One clone's measured init-load context: file count, bytes, token proxy.

    `tokens` is the same chars/4 proxy as the proactive axes (via `count_tokens`),
    so tiers compare on one scale. For a whole-repo walk the number implies a full
    ingest that never happens - a driver greps, it does not read every tracked
    file - so file count and bytes carry the honest signal and tokens is the
    upper-bound proxy (ward#373 open fork 4).
    """

    files: int
    bytes: int
    tokens: int


def _git_tracked_files(repo: Path) -> list[str]:
    """Tracked, repo-relative paths in a working-dir clone via `git ls-files`.

    Tracked-only is deliberate: an untracked build/vendor/node_modules tree must
    not inflate the walk. Returns [] for a non-repo or when git is unavailable,
    so the walk degrades to zero rather than raising.
    """
    try:
        proc = subprocess.run(
            ["git", "-C", str(repo), "ls-files", "-z"],
            capture_output=True,
            text=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return []
    return [name for name in proc.stdout.split("\0") if name]


def immediate_walk(repo: Path) -> TierWalk:
    """Measure a working-dir clone (tier 2, `immediate`).

    Sums tracked-file count, on-disk bytes, and the chars/4 token proxy over
    `git ls-files`. A tracked path that no longer reads (deleted-but-staged,
    a submodule gitlink dir) is skipped so the three figures stay coherent -
    every counted file contributes its bytes and tokens.
    """
    files = 0
    total_bytes = 0
    total_tokens = 0
    for name in _git_tracked_files(repo):
        try:
            data = (repo / name).read_bytes()
        except OSError:
            continue
        files += 1
        total_bytes += len(data)
        total_tokens += count_tokens(data.decode("utf-8", errors="replace"))
    return TierWalk(files, total_bytes, total_tokens)


def peripheral_walk(repos: Iterable[Path]) -> tuple[TierWalk, list[tuple[str, TierWalk]]]:
    """Measure a set of reference repos (tier 3, `peripheral`).

    Applies `immediate_walk` per repo and returns (total, per-repo) where each
    per-repo entry is (repo name, walk). The caller supplies the repo set - ward
    passes its `/substrate` mirrors - so this layer never enumerates or names a
    substrate set of its own.
    """
    per_repo = [(repo.name, immediate_walk(repo)) for repo in repos]
    total = TierWalk(
        files=sum(w.files for _n, w in per_repo),
        bytes=sum(w.bytes for _n, w in per_repo),
        tokens=sum(w.tokens for _n, w in per_repo),
    )
    return total, per_repo


def _expand_skill_roots(roots: list[str], cwd: Path) -> list[Path]:
    """Resolve configured skill roots to concrete dirs to scan.

    Absolute / ~-rooted entries are taken as-is (global plugin dirs). A relative
    entry (e.g. `.claude/skills`) is expanded against the cwd and every workspace
    repo, so an elevated cwd still sees the per-repo mounted skill sets.
    """
    out: list[Path] = []
    for raw in roots:
        p = Path(raw).expanduser()
        if p.is_absolute():
            out.append(p)
            continue
        out.append(cwd / p)
        for repo in iter_workspace_repos():
            out.append(repo / p)
    return out


def skill_contributions(
    roots: list[str], cwd: Path
) -> tuple[int, list[tuple[str, int]], int]:
    """Sum eager SKILL.md frontmatter (name + description) across skill roots.

    Dedups by resolved SKILL.md path, so the one canonical skill set symlinked
    into many repos counts once. Returns (total tokens, top contributors, count).
    """
    seen: set[Path] = set()
    per_skill: list[tuple[str, int]] = []
    for root in _expand_skill_roots(roots, cwd):
        if not root.is_dir():
            continue
        # followlinks: mount-skills.sh symlinks each skill dir into .claude/skills,
        # and rglob won't descend symlinked dirs (recurse_symlinks=False on 3.13).
        for dirpath, _dirs, files in os.walk(root, followlinks=True):
            if "SKILL.md" not in files:
                continue
            skill_md = Path(dirpath) / "SKILL.md"
            try:
                resolved = skill_md.resolve()
            except OSError:
                resolved = skill_md
            if resolved in seen:
                continue
            seen.add(resolved)
            meta, _body = split_frontmatter(
                skill_md.read_text(encoding="utf-8", errors="replace")
            )
            name = str(meta.get("name", "") or skill_md.parent.name)
            desc = str(meta.get("description", ""))
            per_skill.append((name, count_tokens(f"{name}: {desc}")))
    per_skill.sort(key=lambda item: item[1], reverse=True)
    return sum(t for _n, t in per_skill), per_skill, len(per_skill)


def _human_bytes(n: int) -> str:
    """Compact byte size (e.g. 12.3M) for the tier walk lines."""
    size = float(n)
    for unit in ("B", "K", "M", "G"):
        if size < 1024 or unit == "G":
            return f"{size:.0f}{unit}" if unit == "B" else f"{size:.1f}{unit}"
        size /= 1024
    return f"{size:.1f}G"


def _tier_line(label: str, walk: TierWalk) -> str:
    """One aligned `label  N files  bytes  ~tok` row for a tier walk."""
    return (
        f"  {label:22} {walk.files:6} files  {_human_bytes(walk.bytes):>8}  "
        f"~{walk.tokens} tok"
    )


def tier_section(immediate: list[Path], peripheral: list[Path]) -> list[str]:
    """Render the immediate (working-dir) + peripheral (reference) tier walks.

    Only emitted when the caller passes paths; the aos CLI stays role-agnostic
    and never names a substrate set. tokens is the chars/4 upper-bound proxy -
    file count and bytes carry the honest cheap-to-provide signal.
    """
    if not immediate and not peripheral:
        return []
    lines = ["", "role-reachable tiers (working clone + reference repos, not eager prompt)"]
    for repo in immediate:
        lines.append(_tier_line(f"immediate  {repo.name}", immediate_walk(repo)))
    if peripheral:
        total, per_repo = peripheral_walk(peripheral)
        for name, walk in per_repo:
            lines.append(_tier_line(f"peripheral {name}", walk))
        lines.append(_tier_line("peripheral TOTAL", total))
    return lines


def _harness_block(
    harness: str,
    load_point: Path,
    doc_total: int,
    doc_top: list[tuple[str, int]],
    skill_total: int,
    skill_top: list[tuple[str, int]],
    skill_count: int,
    mcp_servers: int | None,
) -> list[str]:
    """Render one harness's doc, skills, and MCP breakdown."""
    total = doc_total + skill_total
    lines = [f"{harness:9} {total:6} tok"]
    lines.append(f"          doc    {doc_total:6} tok  ({load_point})")
    if doc_top:
        name, tok = doc_top[0]
        lines.append(f"            top: {tok:6} tok  {name}")
    lines.append(f"          skills {skill_total:6} tok  ({skill_count} skills, frontmatter only)")
    for name, tok in skill_top[:3]:
        lines.append(f"            top: {tok:6} tok  {name}")
    mcp_label = f"{mcp_servers} servers (native/deferred, CLI fallback)" if mcp_servers is not None else "not measured"
    lines.append(f"          mcp       n/a       ({mcp_label})")
    lines.append("")
    return lines


def read_mcporter_servers(path: Path) -> list[str] | None:
    """Return mcporter's exposed server names, or None when no config is found."""
    if not path.is_file():
        return None
    import json

    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError):
        return None
    servers = data.get("mcpServers")
    if not isinstance(servers, dict):
        return None
    return sorted(servers)


def run(
    config_path: Path,
    mcporter_path: Path,
    cwd: Path,
    *,
    immediate: list[Path] | None = None,
    peripheral: list[Path] | None = None,
) -> int:
    skill_roots = {
        harness: list(roots) for harness, roots in DEFAULT_SKILL_ROOTS.items()
    }
    servers = read_mcporter_servers(mcporter_path)
    mcp_count = len(servers) if servers is not None else None

    lines = [f"context-budget report  ({TOKENIZER_NOTE})", ""]

    load_points = dict(DEFAULT_LOAD_POINTS)
    if config_path.is_file():
        lines.append(
            "agent-compose owns composition; measuring its installed load points directly."
        )
    else:
        lines.append("agent-compose config absent; measuring installed load points directly.")
    lines.append("")
    doc_plan = {
        harness: (
            path,
            count_tokens(path.read_text(encoding="utf-8", errors="replace"))
            if path.is_file()
            else 0,
            [],
        )
        for harness, path in load_points.items()
    }

    for harness in sorted(load_points):
        load_point, doc_total, doc_top = doc_plan[harness]
        skill_total, skill_top, skill_count = skill_contributions(
            skill_roots.get(harness, []), cwd
        )
        block = _harness_block(
            harness, load_point, doc_total, doc_top, skill_total, skill_top,
            skill_count, mcp_count,
        )
        lines.extend(block)

    lines.append(
        "skills = plugin + scoped SKILL.md frontmatter, deduped, discoverable across "
        "the workspace (elevated-cwd worst case; a session in one repo sees fewer). "
        "Bodies load lazily, not counted."
    )
    lines.append(
        "mcp eager surface is ~0: each native harness defers projected schemas. "
        "mcporter remains the inventory, schema browser, and CLI call fallback."
    )
    lines.extend(tier_section(immediate or [], peripheral or []))

    out = "\n".join(lines)
    print(out)
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Report eager context by harness.")
    parser.add_argument("--role", help="agent-compose role bundle to measure")
    parser.add_argument(
        "--provider",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="AOS provider root for role measurement (defaults to this checkout)",
    )
    parser.add_argument(
        "--additional-provider",
        action="append",
        default=[],
        metavar="ID=PATH",
        help=(
            "additional role-capability provider for role measurement; "
            "repeatable"
        ),
    )
    parser.add_argument(
        "--repo",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="repository root whose AGENTS.md cascade role mode measures",
    )
    parser.add_argument(
        "--cwd",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="CWD inside --repo for the role cascade",
    )
    parser.add_argument(
        "--snapshot",
        type=Path,
        help="write the deterministic grouped role YAML snapshot to this path",
    )
    parser.add_argument(
        "--compare",
        type=Path,
        help="compare role context against a prior snapshot",
    )
    parser.add_argument(
        "--skill-root",
        type=Path,
        action="append",
        default=[],
        help="extra plugin skill root to include; repeatable",
    )
    parser.add_argument(
        "--agent-compose",
        default="agent-compose",
        help="agent-compose executable used to materialize the role bundle",
    )
    parser.add_argument("--config", type=Path, default=CONFIG_PATH, help="agent-compose.yaml path")
    parser.add_argument(
        "--mcporter",
        type=Path,
        default=Path.home() / ".mcporter" / "mcporter.json",
        help="shared mcporter inventory projected into each native harness registry",
    )
    parser.add_argument(
        "--immediate",
        type=Path,
        action="append",
        default=[],
        metavar="REPO",
        help="working-dir clone to walk as tier 2 (immediate); repeatable",
    )
    parser.add_argument(
        "--peripheral",
        type=Path,
        action="append",
        default=[],
        metavar="REPO",
        help="reference repo to walk as tier 3 (peripheral), with a total; repeatable",
    )
    args = parser.parse_args()
    if args.additional_provider and not args.role:
        parser.error("--additional-provider requires --role")
    if args.role:
        from agentic_os.context_budget_role import (
            capture_snapshot,
            load_snapshot,
            parse_additional_provider,
            render_delta,
            render_snapshot,
            write_snapshot,
        )

        try:
            additional_providers: dict[str, Path] = {}
            for raw_provider in args.additional_provider:
                source_id, source_root = parse_additional_provider(raw_provider)
                if source_id in additional_providers:
                    raise RuntimeError(
                        f"provider source id is duplicated: {source_id}"
                    )
                additional_providers[source_id] = source_root
            snapshot = capture_snapshot(
                args.provider,
                args.repo,
                args.cwd,
                role=args.role,
                agent_compose=args.agent_compose,
                additional_providers=additional_providers,
                plugin_roots=args.skill_root,
                mcporter_path=args.mcporter,
            )
            print(render_snapshot(snapshot))
            if args.compare:
                print()
                print(render_delta(load_snapshot(args.compare), snapshot))
            if args.snapshot:
                write_snapshot(args.snapshot, snapshot)
                print(f"\nsnapshot: {args.snapshot}")
            return 0
        except RuntimeError as exc:
            sys.stderr.write(f"context-budget: {exc}\n")
            return 1

    try:
        return run(
            args.config,
            args.mcporter,
            Path.cwd(),
            immediate=args.immediate,
            peripheral=args.peripheral,
        )
    except RuntimeError as exc:
        sys.stderr.write(f"context-budget: {exc}\n")
        return 1


if __name__ == "__main__":
    sys.exit(main())
