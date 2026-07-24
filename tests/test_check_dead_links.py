"""Tests for agentic_os.pre_commit.check_dead_links: repo-wide link guard."""
from __future__ import annotations

from pathlib import Path

import pytest

from agentic_os import config
from agentic_os.pre_commit import check_dead_links as cdl


def _write(root: Path, rel: str, body: str) -> None:
    p = root / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(body, encoding="utf-8")


def _run(monkeypatch: pytest.MonkeyPatch, root: Path) -> int:
    monkeypatch.setattr(cdl, "REPO_ROOT", root)
    monkeypatch.setattr(config, "REPO_ROOT", root)
    return cdl.main(["check-dead-links"])


def test_clean_repo_passes(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _write(tmp_path, "docs/x.md", "")
    _write(tmp_path, "README.md", "See [x](docs/x.md).\n")
    assert _run(monkeypatch, tmp_path) == 0


def test_dead_link_outside_skill_tree_is_caught(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    # The widened scan reaches docs/, not just .agents/skills/.
    _write(tmp_path, "docs/ansible.md", "Run [foo](../scripts/foo.sh).\n")
    assert _run(monkeypatch, tmp_path) == 1
    err = capsys.readouterr().err
    assert "dead link" in err
    assert str(Path("docs") / "ansible.md") in err


def test_repo_escaping_link_is_hard_violation(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    # A sibling target really exists on disk, but it is outside the repo root.
    (tmp_path.parent / "sibling.md").write_text("", encoding="utf-8")
    repo = tmp_path / "repo"
    _write(repo, "docs/a.md", "Up and out: [x](../../sibling.md).\n")
    assert _run(monkeypatch, repo) == 1
    err = capsys.readouterr().err
    assert "repo-escaping link" in err


def test_internal_dotdot_link_is_validated_not_skipped(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # An in-repo `../` link that resolves is allowed (no longer short-circuited).
    _write(tmp_path, "docs/target.md", "")
    _write(tmp_path, "pkg/README.md", "Up to [t](../docs/target.md).\n")
    assert _run(monkeypatch, tmp_path) == 0


def test_external_and_anchor_links_skipped(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _write(
        tmp_path,
        "README.md",
        "[web](https://example.com) [mail](mailto:x@y.z) [a](#section)\n",
    )
    assert _run(monkeypatch, tmp_path) == 0


def test_skip_dirs_and_excludes(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # node_modules is skipped structurally; a configured exclude opts the rest out.
    _write(tmp_path, "node_modules/dep/README.md", "[bad](./missing.md)\n")
    _write(tmp_path, "vendor/x.md", "[bad](./missing.md)\n")
    _write(
        tmp_path,
        "pyproject.toml",
        "[tool.agentic-os.dead-cross-links]\nexcludes = ['vendor/']\n",
    )
    assert _run(monkeypatch, tmp_path) == 0


def test_disabled_flag_skips(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    _write(tmp_path, "README.md", "[bad](./missing.md)\n")
    _write(
        tmp_path,
        "pyproject.toml",
        "[tool.agentic-os.dead-cross-links]\nenabled = false\n",
    )
    assert _run(monkeypatch, tmp_path) == 0
