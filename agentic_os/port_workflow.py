#!/usr/bin/env python3
"""Translate ``.forgejo/workflows`` files into ``.github/workflows`` files.

Does the mechanical half: maps runner labels, drops the dev-base ``container:``
and puts ``actions/setup-ci`` first in its place. What it cannot decide
it prints as ``TODO`` lines, so the output is a draft for the checker and a
human, not a finished port. See docs/dev-container.md.
"""

from __future__ import annotations

import argparse
import io
import sys
from pathlib import Path

from ruamel.yaml import YAML
from ruamel.yaml.comments import CommentedMap, CommentedSeq

from agentic_os.workflow_contract import Contract, load_contract

FORGEJO_DIR = Path(".forgejo/workflows")
GITHUB_DIR = Path(".github/workflows")


def make_yaml() -> YAML:
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.width = 4096
    yaml.indent(mapping=2, sequence=4, offset=2)
    return yaml


def map_runs_on(job: CommentedMap, contract: Contract) -> list[str]:
    """Rewrite ``runs-on`` in place and return the labels that were unmapped."""
    runs_on = job.get("runs-on")
    unmapped: list[str] = []

    def translate(label: str) -> str:
        if label in contract.forgejo_to_github:
            return contract.forgejo_to_github[label]
        if label not in contract.hosted:
            unmapped.append(label)
        return label

    if isinstance(runs_on, str):
        job["runs-on"] = translate(runs_on)
    elif isinstance(runs_on, CommentedSeq):
        for index, label in enumerate(runs_on):
            runs_on[index] = translate(str(label))
    return unmapped


def setup_ci_step(contract: Contract) -> CommentedMap:
    step = CommentedMap()
    step["uses"] = contract.setup_ci
    return step


def insert_setup_ci(job: CommentedMap, contract: Contract) -> None:
    # First, not after checkout: a remote action reads its own checkout, and
    # inserting mid-list strands the blank line ruamel hangs on the prior step.
    steps = job.get("steps")
    if isinstance(steps, CommentedSeq):
        steps.insert(0, setup_ci_step(contract))


def port_text(source: str, repo_root: Path, contract: Contract) -> tuple[str, list[str]]:
    """Return the translated workflow text and its TODO lines."""
    yaml = make_yaml()
    document = yaml.load(source)
    todos: list[str] = []
    jobs = document.get("jobs") if isinstance(document, CommentedMap) else None
    if not isinstance(jobs, CommentedMap):
        return source, ["no `jobs` mapping found, nothing was translated"]

    for name, job in jobs.items():
        if not isinstance(job, CommentedMap):
            continue
        original = job.get("runs-on")
        for label in original if isinstance(original, CommentedSeq) else [original]:
            if label in contract.needs_redesign:
                todos.append(f"job `{name}`: {label} - {contract.needs_redesign[label]}")
        for label in map_runs_on(job, contract):
            todos.append(f"job `{name}`: runs-on `{label}` has no mapping, choose a hosted label")
        if "container" in job:
            del job["container"]
            insert_setup_ci(job, contract)

    if (repo_root / "go.mod").is_file():
        todos.append("go.mod found: add `actions/setup-go` with `go-version-file: go.mod`")
    hooks = repo_root / ".pre-commit-config.yaml"
    hook_source = hooks.read_text(encoding="utf-8") if hooks.is_file() else ""
    if any(host in hook_source for host in contract.forbidden_hosts):
        todos.append(
            "`.pre-commit-config.yaml` fetches hooks from Forgejo, so a hosted "
            "`pre-commit` step fails until the hook pin rollout (COI-2943)"
        )

    out = io.StringIO()
    yaml.dump(document, out)
    text = out.getvalue()
    for host in contract.forbidden_hosts:
        if host in text:
            todos.append(f"`{host}` still appears in the output, replace each use")
    return text, todos


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("sources", nargs="+", type=Path, help=".forgejo/workflows files")
    parser.add_argument("--force", action="store_true", help="overwrite an existing output")
    args = parser.parse_args(argv)

    contract = load_contract()
    status = 0
    for source in args.sources:
        repo_root = source.resolve().parent.parent.parent
        target = repo_root / GITHUB_DIR / source.name
        if target.exists() and not args.force:
            sys.stderr.write(f"FAIL: {target} exists, pass --force to overwrite.\n")
            status = 1
            continue
        text, todos = port_text(source.read_text(encoding="utf-8"), repo_root, contract)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")
        print(f"wrote {target}")
        for todo in todos:
            print(f"TODO {source.name}: {todo}")
    return status


if __name__ == "__main__":
    raise SystemExit(main())
