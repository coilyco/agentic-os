"""Tests for scripts/trufflehog-scan.sh."""
from __future__ import annotations

import os
import subprocess
import textwrap
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "trufflehog-scan.sh"


def _write_executable(path: Path, content: str) -> None:
    path.write_text(content, encoding="utf-8")
    path.chmod(0o755)


def _run(root: Path) -> subprocess.CompletedProcess[str]:
    bin_dir = root / "bin"
    bin_dir.mkdir()
    trufflehog_args = root / "trufflehog-args.log"
    trufflehog_path = root / "trufflehog-path.log"
    trufflehog_exists = root / "trufflehog-exists.log"
    trufflehog_contents = root / "trufflehog-contents.log"

    _write_executable(
        bin_dir / "trufflehog",
        textwrap.dedent(
            f"""\
            #!/usr/bin/env bash
            set -euo pipefail
            printf '%s\\n' "$*" >> {trufflehog_args!s}
            exclude=""
            while [[ $# -gt 0 ]]; do
              case "$1" in
                --exclude-paths)
                  exclude="$2"
                  shift 2
                  ;;
                *)
                  shift
                  ;;
              esac
            done
            printf '%s\\n' "$exclude" >> {trufflehog_path!s}
            if [[ -f "$exclude" ]]; then
              printf 'yes\\n' >> {trufflehog_exists!s}
            else
              printf 'no\\n' >> {trufflehog_exists!s}
            fi
            cat "$exclude" > {trufflehog_contents!s}
            exit 0
            """
        ),
    )

    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    return subprocess.run(
        [str(SCRIPT)],
        cwd=root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )


def test_uses_real_temp_file_for_exclude_paths(tmp_path: Path) -> None:
    result = _run(tmp_path)

    assert result.returncode == 0
    assert (tmp_path / "trufflehog-args.log").read_text(encoding="utf-8").strip().startswith(
        "git file://. --since-commit HEAD --exclude-paths "
    )
    exclude_path = (tmp_path / "trufflehog-path.log").read_text(encoding="utf-8").strip()
    assert not exclude_path.startswith("/dev/fd/")
    assert (tmp_path / "trufflehog-exists.log").read_text(encoding="utf-8").strip() == "yes"
    assert (tmp_path / "trufflehog-contents.log").read_text(encoding="utf-8") == textwrap.dedent(
        """\
        (^|/)target/
        (^|/)\\.venv/
        (^|/)venv/
        (^|/)node_modules/
        (^|/)__pycache__/
        (^|/)\\.mypy_cache/
        (^|/)\\.pytest_cache/
        (^|/)\\.ruff_cache/
        (^|/)(dist|build)/
        """
    )
