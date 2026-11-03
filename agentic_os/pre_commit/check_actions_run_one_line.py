#!/usr/bin/env python3
"""Keep GitHub Actions workflow YAML orchestration-only.

Every Actions step ``run`` value must be a scalar written on one physical YAML
line, must decode without embedded newlines, and must not carry a program body
inline. Script bodies belong in tracked language-native files where ordinary
linters and tests can exercise them. See docs/pre-commit-hygiene.md.
"""

from __future__ import annotations

import re
import shlex
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Iterator

import yaml
from yaml.nodes import MappingNode, Node, ScalarNode, SequenceNode
from agentic_os.pre_commit.tree import is_repo_content

HOOK_ID = "actions-run-one-line"
REPO_ROOT = Path.cwd()
WORKFLOW_DIRS = {".github"}
ACTION_FILENAMES = {"action.yml", "action.yaml"}
YAML_SUFFIXES = {".yml", ".yaml"}

# Why a length bar and not just the one-line rule: docs/pre-commit-hygiene.md.
MAX_INLINE_BODY_CHARS = 100
INLINE_SOURCE_FLAGS = {"-c", "-e", "-E", "-p", "--eval", "--exec", "--print"}
INTERPRETER_PATTERN = re.compile(
    r"^(python|node|nodejs|ruby|perl|bash|sh|zsh|dash|ksh)[0-9.]*$"
)
ESCAPED_NEWLINE_PATTERN = re.compile(r"\\+n")
# `<<<` is a herestring, which carries a word rather than a body.
HEREDOC_PATTERN = re.compile(r"(?<!<)<<-?\s*(?![<\s])(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\1")
INLINE_FALLBACK_PATTERN = re.compile(
    r"(?:^|[\s;|&(])(?P<name>[\w./+-]+)\s+"
    r"(?P<flag>-c|-e|-E|-p|--eval|--exec|--print)\s+(?P<body>\S.*)$"
)


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


def _is_interpreter(token: str) -> bool:
    return bool(INTERPRETER_PATTERN.match(PurePosixPath(token).name))


def _strip_quotes(body: str) -> str:
    if len(body) >= 2 and body[0] == body[-1] and body[0] in {"'", '"'}:
        return body[1:-1]
    return body


def _inline_program_bodies(command: str) -> Iterator[tuple[str, str, str]]:
    """Yield ``(interpreter, flag, body)`` for every inline-source invocation."""
    try:
        tokens = shlex.split(command)
    except ValueError:
        # Unbalanced quoting the shell would reject too. Fall back to reading
        # the tail of the line as the body rather than skipping the check.
        match = INLINE_FALLBACK_PATTERN.search(command)
        if match and _is_interpreter(match.group("name")):
            yield (
                match.group("name"),
                match.group("flag"),
                _strip_quotes(match.group("body")),
            )
        return
    for index in range(len(tokens) - 2):
        if _is_interpreter(tokens[index]) and tokens[index + 1] in INLINE_SOURCE_FLAGS:
            yield tokens[index], tokens[index + 1], tokens[index + 2]


def _inline_body_reason(command: str) -> str | None:
    """Why ``command`` carries a program body rather than invoking one."""
    for name, flag, body in _inline_program_bodies(command):
        if ESCAPED_NEWLINE_PATTERN.search(body):
            return (
                f"Actions step run inlines a multi-line program body "
                f"through `{name} {flag}`."
            )
        if len(body) > MAX_INLINE_BODY_CHARS:
            return (
                f"Actions step run inlines a {len(body)}-character program body "
                f"through `{name} {flag}` (max {MAX_INLINE_BODY_CHARS})."
            )
    if HEREDOC_PATTERN.search(command):
        return "Actions step run cannot open a heredoc body."
    return None


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
                inline_reason = _inline_body_reason(run_node.value)
                if inline_reason is None:
                    continue
                reason = inline_reason
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
        and is_repo_content(path.relative_to(root), root)
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
    sys.stderr.write(f"\n{len(violations)} Actions run shape violation(s).\n")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
