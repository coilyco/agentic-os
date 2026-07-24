"""Sync AOSH-selected local harness models into the AOS Ward bundle."""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_AOSH_ROOT = REPO_ROOT.parents[1] / "coilyco-bridge" / "agentic-os-hardware"
PAIRINGS_PATH = Path(".agents/skills/leaderboard-agent-model-pairs/info/94-pairings.yaml")
INVENTORY_PATH = Path(".agents/skills/leaderboard-agent-model-pairs/info/90-inventory.yaml")
DEFAULT_BUNDLE = REPO_ROOT / ".ward" / "agents.kdl"
# Only harnesses whose deployment-wide model is selected by an AOSH route belong
# here. OpenCode is an AOS-local backend overlay, not the engineer role's route.
SYNCED_HARNESSES = ("goose",)


class SyncError(RuntimeError):
    """A selected roster or bundle invariant is invalid."""


@dataclass(frozen=True)
class Selection:
    model: str
    server: str


def _load_yaml(path: Path) -> dict[str, Any]:
    try:
        value = yaml.safe_load(path.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as exc:
        raise SyncError(f"read {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise SyncError(f"{path}: expected a YAML mapping")
    return value


def _entries(document: dict[str, Any], path: Path) -> list[dict[str, Any]]:
    value = document.get("entries")
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        raise SyncError(f"{path}: entries must be a list of mappings")
    return value


def load_selections(aosh_root: Path) -> dict[str, Selection]:
    pairings_file = aosh_root / PAIRINGS_PATH
    inventory_file = aosh_root / INVENTORY_PATH
    pairings = _entries(_load_yaml(pairings_file), pairings_file)
    inventory = _entries(_load_yaml(inventory_file), inventory_file)

    selected: dict[str, Selection] = {}
    for harness in SYNCED_HARNESSES:
        matches = [entry for entry in pairings if entry.get("agent") == harness]
        if len(matches) != 1:
            raise SyncError(
                f"{pairings_file}: expected one {harness} pairing, found {len(matches)}"
            )
        model = matches[0].get("model")
        server = matches[0].get("server")
        if not isinstance(model, str) or not model or not isinstance(server, str) or not server:
            raise SyncError(f"{pairings_file}: {harness} pairing needs non-empty model and server")
        provisioned = [
            entry
            for entry in inventory
            if entry.get("model") == model
            and entry.get("server") == server
            and entry.get("keep") is True
        ]
        if len(provisioned) != 1:
            raise SyncError(
                f"{inventory_file}: {harness} selection {model!r} on {server!r} "
                "is not uniquely provisioned with keep: true"
            )
        selected[harness] = Selection(model=model, server=server)
    return selected


def _agent_block_pattern(harness: str) -> re.Pattern[str]:
    return re.compile(
        rf"(?ms)(^    agent {re.escape(harness)} \{{\n)(.*?)(^    \}}\n)",
    )


def bundle_models(text: str) -> dict[str, str | None]:
    models: dict[str, str | None] = {}
    for harness in SYNCED_HARNESSES:
        match = _agent_block_pattern(harness).search(text)
        if match is None:
            raise SyncError(f".ward/agents.kdl: missing agent {harness} block")
        model = re.search(r"(?m)^        model[ \t]+([^\n]+)$", match.group(2))
        models[harness] = model.group(1).strip() if model else None
    return models


def render_bundle(text: str, selected: dict[str, Selection]) -> str:
    rendered = text
    for harness in SYNCED_HARNESSES:
        pattern = _agent_block_pattern(harness)
        match = pattern.search(rendered)
        if match is None:
            raise SyncError(f".ward/agents.kdl: missing agent {harness} block")
        body = match.group(2)
        model_line = f"        model {selected[harness].model}"
        if re.search(r"(?m)^        model[ \t]+[^\n]+$", body):
            body = re.sub(r"(?m)^        model[ \t]+[^\n]+$", model_line, body, count=1)
        else:
            body = model_line + "\n" + body
        rendered = (
            rendered[: match.start()]
            + match.group(1)
            + body
            + match.group(3)
            + rendered[match.end() :]
        )
    return rendered


def run(aosh_root: Path, bundle: Path, *, check: bool) -> int:
    selected = load_selections(aosh_root)
    try:
        current = bundle.read_text(encoding="utf-8")
    except OSError as exc:
        raise SyncError(f"read {bundle}: {exc}") from exc
    expected = render_bundle(current, selected)

    if current == expected:
        pairs = ", ".join(f"{name}={selected[name].model}" for name in SYNCED_HARNESSES)
        print(f"ok: AOS local harness models match the AOSH provisioned roster ({pairs})")
        return 0
    if check:
        actual = bundle_models(current)
        for name in SYNCED_HARNESSES:
            if actual[name] != selected[name].model:
                print(
                    f"drift: {name} bundle={actual[name] or '<missing>'} "
                    f"aosh={selected[name].model}",
                    file=sys.stderr,
                )
        print("run `ward exec sync-local-models` from the AOS checkout", file=sys.stderr)
        return 1

    bundle.write_text(expected, encoding="utf-8")
    print(f"updated {bundle} from {aosh_root / PAIRINGS_PATH}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check", action="store_true", help="report drift without changing the bundle"
    )
    parser.add_argument(
        "--if-present",
        action="store_true",
        help="skip when the default sibling AOSH checkout is absent",
    )
    parser.add_argument("--aosh-root", type=Path, default=DEFAULT_AOSH_ROOT)
    parser.add_argument("--bundle", type=Path, default=DEFAULT_BUNDLE)
    args = parser.parse_args(argv)

    if args.if_present and not args.aosh_root.exists():
        print(f"skip: AOSH checkout is absent at {args.aosh_root}")
        return 0
    try:
        return run(args.aosh_root, args.bundle, check=args.check)
    except SyncError as exc:
        print(f"local-model-sync: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
