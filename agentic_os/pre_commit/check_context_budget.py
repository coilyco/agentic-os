#!/usr/bin/env python3
"""Report the eager startup-context budget each harness loads, on demand.

This measures everything a harness ingests at session start, per harness, against
a per-harness token budget, across three axes that each have a different growth
lever:

  * doc    - the composed AGENTS.md/CLAUDE.md load point (what agent-compose
             builds). Reuses the composer's own resolution, so the bytes match
             what the load point holds. Lever: edit the AGENTS.md sources.
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
absolute but consistent across harnesses, which a zero-sum budget needs.

Budgets and skill roots are global (host-wide, not repo-scoped): module defaults,
overridable by `budgets:` / `skill_roots:` in agent-compose.yaml or CLI flags.

Usage:
    check-context-budget                  # report every harness vs its budget
    check-context-budget --check          # exit 1 if any harness is over budget
    check-context-budget --claude-budget 12000
    check-context-budget --goose --snapshot before.json
    check-context-budget --goose --compare before.json --snapshot after.json
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
from agentic_os.generators.generate_agent_compose import (
    COMPOSED_PATH,
    CONFIG_PATH,
    DEFAULT_LOAD_POINTS,
    _split_frontmatter,
    compose,
    gather_sources,
    load_config,
    plan_outputs,
    resolve_load_points,
    select_by_scope,
    _normalize_scopes,
)

# Whole-baseline (doc + skills) budgets, not laws. Tune via agent-compose.yaml
# `budgets:` / CLI flags; per-harness rationale in docs/context-budget.md.
DEFAULT_BUDGETS = {"claude": 13_000, "codex": 6_000, "opencode": 5_000}

# Per-harness skill roots scanned for SKILL.md frontmatter; absolute = global
# plugins, relative = expanded against cwd + workspace repos (docs/context-budget.md).
DEFAULT_SKILL_ROOTS = {
    "claude": ["~/.claude/plugins", ".claude/skills"],
    "codex": ["~/.codex/skills", ".codex/skills"],
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


def _banner_tokens() -> int:
    """Token cost of the compose banner alone, to net it out of attribution."""
    return count_tokens(compose([]))


def doc_contributions(
    sources: list[Path], overrides: dict[Path, Path]
) -> tuple[int, list[tuple[str, int]]]:
    """Return (doc tokens, per-source tokens) for one harness's composed slice.

    Total is the real composed body the harness loads. Per-source nets out the
    banner so the breakdown sums close to the total and ranks contributors.
    """
    total = count_tokens(compose(sources, overrides))
    banner = _banner_tokens()
    per_source = [
        (
            str(src),
            max(
                0,
                count_tokens(
                    compose([src], {src: overrides[src]} if src in overrides else {})
                )
                - banner,
            ),
        )
        for src in sources
    ]
    per_source.sort(key=lambda item: item[1], reverse=True)
    return total, per_source


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
            meta, _body = _split_frontmatter(skill_md.read_text(encoding="utf-8", errors="replace"))
            name = str(meta.get("name", "") or skill_md.parent.name)
            desc = str(meta.get("description", ""))
            per_skill.append((name, count_tokens(f"{name}: {desc}")))
    per_skill.sort(key=lambda item: item[1], reverse=True)
    return sum(t for _n, t in per_skill), per_skill, len(per_skill)


def plan_from_config(
    config_path: Path, composed_path: Path
) -> tuple[
    dict[str, Path], dict[str, list[Path]], dict[str, dict[Path, Path]], dict[str, int]
] | None:
    """Reproduce the per-harness compose plan, or None when agent-compose is off."""
    if not config_path.is_file():
        return None
    config = load_config(config_path)
    gathered, _errors = gather_sources(config)
    sources = select_by_scope(gathered, _normalize_scopes(config.get("scopes")))
    if not sources:
        return None
    load_points = resolve_load_points(config)
    slices, overrides, _outputs, _harness_errors = plan_outputs(
        sources, load_points, composed_path
    )
    raw_budgets = config.get("budgets")
    budgets: dict[str, int] = {}
    if isinstance(raw_budgets, dict):
        budgets = {str(k): int(v) for k, v in raw_budgets.items() if isinstance(v, int)}
    return load_points, slices, overrides, budgets


def skill_roots_from_config(config_path: Path) -> dict[str, list[str]]:
    """Per-harness skill roots: module defaults overlaid by config `skill_roots:`."""
    roots = {h: list(r) for h, r in DEFAULT_SKILL_ROOTS.items()}
    if not config_path.is_file():
        return roots
    raw = load_config(config_path).get("skill_roots")
    if isinstance(raw, dict):
        for harness, value in raw.items():
            if isinstance(value, list):
                roots[str(harness)] = [str(v) for v in value]
    return roots


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


def _bar(tokens: int, budget: int, width: int = 24) -> str:
    """A small filled/empty bar showing tokens against budget."""
    if budget <= 0:
        return ""
    filled = min(width, round(width * tokens / budget))
    return "[" + "#" * filled + "-" * (width - filled) + "]"


def _harness_block(
    harness: str,
    load_point: Path,
    doc_total: int,
    doc_top: list[tuple[str, int]],
    skill_total: int,
    skill_top: list[tuple[str, int]],
    skill_count: int,
    mcp_servers: int | None,
    budget: int,
) -> tuple[list[str], bool]:
    """Render one harness's doc/skills/mcp breakdown vs its total budget."""
    total = doc_total + skill_total
    is_over = bool(budget) and total > budget
    flag = f"  OVER by {total - budget}" if is_over else ""
    lines = [f"{harness:9} {total:6} tok  / {budget:6} budget  {_bar(total, budget)}{flag}"]
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
    return lines, is_over


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
    composed_path: Path,
    cli_budgets: dict[str, int],
    mcporter_path: Path,
    cwd: Path,
    *,
    check: bool,
    immediate: list[Path] | None = None,
    peripheral: list[Path] | None = None,
) -> int:
    plan = plan_from_config(config_path, composed_path)
    skill_roots = skill_roots_from_config(config_path)
    servers = read_mcporter_servers(mcporter_path)
    mcp_count = len(servers) if servers is not None else None

    budgets = dict(DEFAULT_BUDGETS)
    lines = [f"context-budget report  ({TOKENIZER_NOTE})", ""]
    over: list[str] = []

    if plan is None:
        load_points = dict(DEFAULT_LOAD_POINTS)
        budgets.update(cli_budgets)
        lines.append("agent-compose config absent; measuring installed load points directly.")
        lines.append("")
        doc_plan = {
            h: (p, count_tokens(p.read_text(encoding="utf-8", errors="replace")) if p.is_file() else 0, [])
            for h, p in load_points.items()
        }
    else:
        load_points, slices, overrides, config_budgets = plan
        budgets.update(config_budgets)
        budgets.update(cli_budgets)
        doc_plan = {}
        for h in load_points:
            doc_total, doc_top = doc_contributions(slices.get(h, []), overrides.get(h, {}))
            doc_plan[h] = (load_points[h], doc_total, doc_top)

    for harness in sorted(load_points):
        load_point, doc_total, doc_top = doc_plan[harness]
        skill_total, skill_top, skill_count = skill_contributions(
            skill_roots.get(harness, []), cwd
        )
        block, is_over = _harness_block(
            harness, load_point, doc_total, doc_top, skill_total, skill_top,
            skill_count, mcp_count, budgets.get(harness, 0),
        )
        lines.extend(block)
        if is_over:
            over.append(harness)

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
    if check and over:
        sys.stderr.write(out + "\n")
        sys.stderr.write(f"\nover budget: {', '.join(over)}\n")
        return 1
    print(out)
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Report per-harness eager context budget.")
    parser.add_argument(
        "--goose",
        action="store_true",
        help="capture the fixed ops/operational-decision Goose structural baseline",
    )
    parser.add_argument(
        "--provider",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="AOS provider root for --goose (defaults to this checkout)",
    )
    parser.add_argument(
        "--repo",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="repository root whose AGENTS.md cascade --goose measures",
    )
    parser.add_argument(
        "--cwd",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="CWD inside --repo for the --goose cascade",
    )
    parser.add_argument(
        "--snapshot",
        type=Path,
        help="write the deterministic --goose JSON snapshot to this path",
    )
    parser.add_argument(
        "--compare",
        type=Path,
        help="compare --goose against a prior snapshot",
    )
    parser.add_argument(
        "--skill-root",
        type=Path,
        action="append",
        default=[],
        help="extra Goose/plugin skill root to include; repeatable",
    )
    parser.add_argument(
        "--agent-compose",
        default="agent-compose",
        help="agent-compose executable used to materialize the local Goose bundle",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="exit 1 if any harness is over budget (for CI), instead of just printing",
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
    for harness in DEFAULT_BUDGETS:
        parser.add_argument(
            f"--{harness}-budget", type=int, default=None, help=f"override the {harness} token budget"
        )
    args = parser.parse_args()
    if args.goose:
        from agentic_os.context_budget_goose import (
            capture_snapshot,
            load_snapshot,
            render_delta,
            render_snapshot,
            write_snapshot,
        )

        try:
            snapshot = capture_snapshot(
                args.provider,
                args.repo,
                args.cwd,
                agent_compose=args.agent_compose,
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

    cli_budgets = {
        harness: getattr(args, f"{harness}_budget")
        for harness in DEFAULT_BUDGETS
        if getattr(args, f"{harness}_budget") is not None
    }
    try:
        return run(
            args.config,
            COMPOSED_PATH,
            cli_budgets,
            args.mcporter,
            Path.cwd(),
            check=args.check,
            immediate=args.immediate,
            peripheral=args.peripheral,
        )
    except RuntimeError as exc:
        sys.stderr.write(f"context-budget: {exc}\n")
        return 1


if __name__ == "__main__":
    sys.exit(main())
