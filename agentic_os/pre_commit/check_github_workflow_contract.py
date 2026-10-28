#!/usr/bin/env python3
"""Keep ``.github/workflows`` files inside the Forgejo-retirement contract.

A ported workflow must run on a GitHub-hosted runner label from
``workflow_contract.json`` and must not name the Forgejo host, which a hosted
runner cannot reach. Forgejo workflow trees are left alone: they keep their own
labels until the repo flips. See docs/dev-container.md.
"""

from __future__ import annotations

import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator

import yaml
from yaml.nodes import MappingNode, Node, ScalarNode, SequenceNode

from agentic_os.pre_commit.check_actions_run_one_line import action_files
from agentic_os.workflow_contract import Contract, load_contract

HOOK_ID = "github-workflow-contract"
REPO_ROOT = Path.cwd()


@dataclass(frozen=True)
class Violation:
    path: Path
    line: int
    column: int
    reason: str

    def render(self) -> str:
        return f"{self.path.as_posix()}:{self.line}:{self.column}: {self.reason}"


def is_github_workflow(path: Path) -> bool:
    parts = path.parts
    return path.suffix in {".yml", ".yaml"} and any(
        parts[index] == ".github" and parts[index + 1] == "workflows"
        for index in range(len(parts) - 1)
    )


def mapping_value(node: MappingNode, key: str) -> Node | None:
    for key_node, value_node in node.value:
        if isinstance(key_node, ScalarNode) and key_node.value == key:
            return value_node
    return None


def scalars(node: Node) -> Iterator[ScalarNode]:
    if isinstance(node, ScalarNode):
        yield node
    elif isinstance(node, MappingNode):
        for key_node, value_node in node.value:
            yield from scalars(key_node)
            yield from scalars(value_node)
    elif isinstance(node, SequenceNode):
        for item in node.value:
            yield from scalars(item)


def runs_on_labels(node: Node) -> list[ScalarNode]:
    if isinstance(node, ScalarNode):
        return [node]
    if isinstance(node, SequenceNode):
        return [item for item in node.value if isinstance(item, ScalarNode)]
    if isinstance(node, MappingNode):
        labels = mapping_value(node, "labels")
        return runs_on_labels(labels) if labels is not None else []
    return []


def label_reason(label: str, contract: Contract) -> str | None:
    if label in contract.hosted:
        return None
    if "${{" in label:
        return (
            f"runs-on `{label}` is an expression the checker cannot resolve. "
            "Write the hosted label literally."
        )
    if label in contract.forgejo_to_github:
        return (
            f"runs-on `{label}` is a Forgejo runner label. "
            f"Use `{contract.forgejo_to_github[label]}`."
        )
    return (
        f"runs-on `{label}` is outside the contract. "
        f"Use one of: {', '.join(contract.hosted)}."
    )


def check_file(
    path: Path, root: Path | None = None, contract: Contract | None = None
) -> list[Violation]:
    repo_root = root or REPO_ROOT
    rules = contract or load_contract()
    try:
        display = path.resolve().relative_to(repo_root.resolve())
    except ValueError:
        display = path
    try:
        documents = list(
            yaml.compose_all(path.read_text(encoding="utf-8"), Loader=yaml.SafeLoader)
        )
    except (OSError, UnicodeDecodeError, yaml.YAMLError) as exc:
        return [Violation(display, 1, 1, f"cannot read workflow YAML: {exc}.")]

    violations: list[Violation] = []

    def add(node: Node, reason: str) -> None:
        violations.append(
            Violation(display, node.start_mark.line + 1, node.start_mark.column + 1, reason)
        )

    for document in documents:
        if not isinstance(document, MappingNode):
            continue
        for scalar in scalars(document):
            for host in rules.forbidden_hosts:
                if host in scalar.value:
                    add(scalar, f"names `{host}`, which a GitHub-hosted runner cannot reach.")
        jobs = mapping_value(document, "jobs")
        if not isinstance(jobs, MappingNode):
            continue
        for _, job in jobs.value:
            if not isinstance(job, MappingNode):
                continue
            runs_on = mapping_value(job, "runs-on")
            if runs_on is None:
                continue
            if isinstance(runs_on, MappingNode) and mapping_value(runs_on, "group"):
                add(runs_on, "runs-on names a runner group, which needs a self-hosted runner.")
            for label in runs_on_labels(runs_on):
                reason = label_reason(label.value, rules)
                if reason is not None:
                    add(label, reason)
    return violations


def main(argv: list[str] | None = None) -> int:
    paths = list(sys.argv[1:] if argv is None else argv)
    violations: list[Violation] = []
    for path in action_files(paths):
        if is_github_workflow(path):
            violations.extend(check_file(path))

    if not violations:
        print(f"{HOOK_ID} check: OK")
        return 0
    for violation in violations:
        sys.stderr.write(f"FAIL: {violation.render()}\n")
    sys.stderr.write(f"\n{len(violations)} workflow contract violation(s).\n")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
