#!/usr/bin/env python3
"""Keep GitHub and Forgejo Actions workflow YAML orchestration-only.

Every Actions step ``run`` value must be a scalar written on one physical YAML
line and must decode without embedded newlines. Script bodies belong in tracked
language-native files where ordinary linters and tests can exercise them.
"""

from __future__ import annotations

import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator

import yaml
from yaml.nodes import MappingNode, Node, ScalarNode, SequenceNode

HOOK_ID = "actions-run-one-line"
REPO_ROOT = Path.cwd()
WORKFLOW_DIRS = {".github", ".forgejo"}
ACTION_FILENAMES = {"action.yml", "action.yaml"}
YAML_SUFFIXES = {".yml", ".yaml"}
SKIP_DIR_NAMES = {".git", ".mypy_cache", ".venv", "__pycache__", "node_modules"}


@dataclass(frozen=True)
class Violation:
    path: Path
    line: int
    column: int
    reason: str

    def render(self) -> str:
        return (
            f"{self.path.as_posix()}:{self.line}:{self.column}: {self.reason} "
            "Move the script body into a tracked file and invoke it from one "
            "single-line run command."
        )


def is_actions_yaml(path: Path) -> bool:
    if path.name in ACTION_FILENAMES:
        return True
    if path.suffix not in YAML_SUFFIXES:
        return False
    parts = path.parts
    return any(
        parts[index] in WORKFLOW_DIRS and parts[index + 1] == "workflows"
        for index in range(len(parts) - 1)
    )


def _mapping_value(node: MappingNode, key: str) -> Node | None:
    for key_node, value_node in node.value:
        if isinstance(key_node, ScalarNode) and key_node.value == key:
            return value_node
    return None


def _step_run_nodes(root: Node) -> Iterator[Node]:
    seen: set[int] = set()

    def walk(node: Node) -> Iterator[Node]:
        identity = id(node)
        if identity in seen:
            return
        seen.add(identity)

        if isinstance(node, MappingNode):
            steps = _mapping_value(node, "steps")
            if isinstance(steps, SequenceNode):
                for step in steps.value:
                    if not isinstance(step, MappingNode):
                        continue
                    run = _mapping_value(step, "run")
                    if run is not None:
                        yield run
            for _, value_node in node.value:
                yield from walk(value_node)
        elif isinstance(node, SequenceNode):
            for value_node in node.value:
                yield from walk(value_node)

    yield from walk(root)


def _display_path(path: Path, root: Path) -> Path:
    try:
        return path.resolve().relative_to(root.resolve())
    except ValueError:
        return path


def check_file(path: Path, root: Path | None = None) -> list[Violation]:
    repo_root = root or REPO_ROOT
    display_path = _display_path(path, repo_root)
    try:
        source = path.read_text(encoding="utf-8")
        documents = list(yaml.compose_all(source, Loader=yaml.SafeLoader))
    except (OSError, UnicodeDecodeError) as exc:
        return [Violation(display_path, 1, 1, f"cannot read Actions YAML: {exc}.")]
    except yaml.YAMLError as exc:
        mark = getattr(exc, "problem_mark", None)
        line = mark.line + 1 if mark is not None else 1
        column = mark.column + 1 if mark is not None else 1
        return [Violation(display_path, line, column, f"invalid Actions YAML: {exc}.")]

    violations: list[Violation] = []
    for document in documents:
        if document is None:
            continue
        for run_node in _step_run_nodes(document):
            line = run_node.start_mark.line + 1
            column = run_node.start_mark.column + 1
            if not isinstance(run_node, ScalarNode):
                violations.append(
                    Violation(
                        display_path,
                        line,
                        column,
                        "Actions step run must be a scalar.",
                    )
                )
                continue
            if run_node.style in {"|", ">"}:
                reason = "Actions step run cannot use a YAML block scalar."
            elif run_node.end_mark.line > run_node.start_mark.line:
                reason = "Actions step run cannot span physical YAML lines."
            elif "\n" in run_node.value or "\r" in run_node.value:
                reason = "Actions step run cannot decode to multiple lines."
            else:
                continue
            violations.append(Violation(display_path, line, column, reason))
    return violations


def _tracked_paths(root: Path) -> list[Path]:
    try:
        result = subprocess.run(
            ["git", "ls-files", "-z"],
            cwd=root,
            capture_output=True,
            check=True,
            text=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return []
    return [root / item for item in result.stdout.split("\0") if item]


def _walk_paths(root: Path) -> list[Path]:
    return [
        path
        for path in root.rglob("*")
        if path.is_file()
        and not any(part in SKIP_DIR_NAMES for part in path.parts)
        and is_actions_yaml(path)
    ]


def action_files(paths: list[str], root: Path | None = None) -> list[Path]:
    repo_root = root or REPO_ROOT
    if paths:
        candidates: list[Path] = []
        for raw_path in paths:
            path = Path(raw_path)
            if path.is_dir():
                candidates.extend(_walk_paths(path))
            else:
                candidates.append(path)
    else:
        candidates = _tracked_paths(repo_root) or _walk_paths(repo_root)
    return sorted(
        {
            path
            for path in candidates
            if path.is_file() and is_actions_yaml(path)
        }
    )


def main(argv: list[str] | None = None) -> int:
    paths = list(sys.argv[1:] if argv is None else argv)
    violations: list[Violation] = []
    for path in action_files(paths):
        violations.extend(check_file(path))

    if not violations:
        print(f"{HOOK_ID} check: OK")
        return 0
    for violation in violations:
        sys.stderr.write(f"FAIL: {violation.render()}\n")
    sys.stderr.write(f"\n{len(violations)} multiline Actions run violation(s).\n")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
