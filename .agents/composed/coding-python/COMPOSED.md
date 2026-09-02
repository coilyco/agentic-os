---
name: coding-python
description: Kai's primary language. Umbrella for Python work across repos. Carries her defaults (3.12+, uv, ruff, pyright/mypy, pytest) so agents inherit them before reaching for generic Python knowledge.
seed:
  kind: language
  language: python
  extensions: [".py", ".pyi"]
---

# coding-python

Umbrella skill for any Python work. Kai's primary language, 10+ years of practice. Refine details over time as patterns crystallize.

## Triggers

Broad Python keyword surface - python, python3, .py, pip, uv, poetry, pyenv, pytest, ruff, mypy, pyright, asyncio, pydantic, typing, dataclass, fastapi, flask, django.

## Defaults

- **Version**: 3.12+ unless a target environment pins lower. Reach for the newest features Kai can use.
- **Env management**: `uv` (Astral). Not pip/venv directly, not poetry. `uv venv`, `uv pip install`, `uv run`.
- **Lint/format**: `ruff` for both. Single tool, fast, opinionated.
- **Type checking**: `pyright` or `mypy`. Type hints expected on new code, not retrofitted on legacy aggressively.
- **Tests**: `pytest`. Async tests via `pytest-asyncio` or `anyio`.
- **Formatting**: ruff format (replaces black). Line length 100 unless project pins otherwise.

## Style

- Type hints on function signatures, less religious about every local variable.
- Dataclasses or pydantic over dict-shaped objects when the form is known.
- f-strings over `.format()` or `%`. Always.
- `pathlib.Path` over `os.path` string surgery.
- Async-first when there's I/O. Sync only when nothing benefits from concurrency.
- Stdlib first when reasonable. Reach for deps when they earn it.

## When this skill is active

Editing or writing Python. Inherit Kai's preferences before reaching for general Python knowledge from training data.
