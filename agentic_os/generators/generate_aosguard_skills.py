#!/usr/bin/env python3
"""Split umbra's one aosguard skill into one skill per wrapped area.

A single `aosguard` skill only loads once an agent already suspects it needs
aosguard, which is the retrieval failure agentic-os#1028 records: an agent read
that operator verbs live here, found no `reopen` on the MCP surface, and wrote
"denied" into three tickets. `aosguard-forgejo` matches the entity the agent is
demonstrably working with instead.

Generated from umbra's own index, so a new wrapped area produces its skill
with no hand edit. The concept skill that says what aosguard is, and is not,
is hand-written at .agents/skills/tooling-aosguard.
"""

from __future__ import annotations

import argparse
import shutil
import sys
from pathlib import Path

import yaml

BINARY = "aosguard"
INDEX = Path("references") / "commands.yaml"


def area_of(path: list[str]) -> str | None:
    """The wrapped entity a leaf belongs to, or None when it has no area."""
    if len(path) < 3 or path[0] != BINARY or path[1] != "ops":
        return None
    return path[2]


def group_by_area(commands: list[dict]) -> dict[str, list[dict]]:
    grouped: dict[str, list[dict]] = {}
    for command in commands:
        area = area_of(list(command.get("path") or []))
        if area is None:
            continue
        grouped.setdefault(area, []).append(command)
    return grouped


def render_skill(area: str, commands: list[dict]) -> str:
    """One area's SKILL.md. Kept short: the index is the payload."""
    verbs = sorted({" ".join(c["path"][3:]) for c in commands if len(c["path"]) > 3})
    preview = ", ".join(verbs[:8])
    if len(verbs) > 8:
        preview += ", ..."
    return (
        "---\n"
        f"name: {BINARY}-{area}\n"
        f"description: Guarded operator verbs for {area} through the aosguard CLI. "
        f"Reach for it when a task needs {area} and an MCP surface lacks the verb. "
        f"Triggers - {area}, aosguard {area}.\n"
        "---\n"
        "\n"
        f"# aosguard {area}\n"
        "\n"
        f"`aosguard ops {area} <verb>` exposes {len(commands)} guarded leaves.\n"
        f"Start with `aosguard ops {area} --help`, and use `describe` where the\n"
        "policy offers it. The running CLI's help is authoritative for flags.\n"
        "\n"
        f"Verbs: {preview}\n"
        "\n"
        "`references/commands.yaml` indexes every leaf in this area.\n"
        "\n"
        "An absent verb on an MCP surface is not a denial. Check here first.\n"
        f"What aosguard is and is not: the `tooling-{BINARY}` skill.\n"
    )


def write_area_skill(root: Path, area: str, commands: list[dict]) -> Path:
    target = root / f"{BINARY}-{area}"
    (target / "references").mkdir(parents=True, exist_ok=True)
    (target / "SKILL.md").write_text(render_skill(area, commands), encoding="utf-8")
    payload = yaml.safe_dump({"commands": commands}, sort_keys=False, width=100)
    (target / INDEX).write_text(payload, encoding="utf-8")
    return target


def prune_stale(root: Path, areas: set[str]) -> list[str]:
    """Drop a skill whose area the policy no longer wraps."""
    removed = []
    for entry in sorted(root.glob(f"{BINARY}-*")):
        if not entry.is_dir():
            continue
        area = entry.name[len(BINARY) + 1 :]
        if area not in areas:
            shutil.rmtree(entry)
            removed.append(entry.name)
    return removed


def generate(root: Path) -> tuple[list[str], list[str]]:
    source = root / BINARY / INDEX
    if not source.is_file():
        raise SystemExit(f"no umbra index at {source}; run the aosguard build first")
    document = yaml.safe_load(source.read_text(encoding="utf-8")) or {}
    grouped = group_by_area(list(document.get("commands") or []))
    if not grouped:
        raise SystemExit(f"{source} lists no `aosguard ops <area>` leaves")
    written = [write_area_skill(root, area, cmds).name for area, cmds in sorted(grouped.items())]
    return written, prune_stale(root, set(grouped))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("--skills-root", default="dist/skills")
    args = parser.parse_args(argv)
    written, removed = generate(Path(args.skills_root))
    print(f"aosguard skills: wrote {len(written)} area skill(s)")
    for name in removed:
        print(f"aosguard skills: removed stale {name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
