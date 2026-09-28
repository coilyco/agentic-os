"""Tests for scripts/trufflehog-scan.sh."""
from __future__ import annotations

import os
import shutil
import subprocess
import textwrap
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "trufflehog-scan.sh"

# The public sha256 of the sentry_sdk-2.70.0.tar.gz sdist, which trufflehog's
# SentryToken detector reads as a secret (agentic-os#8407).
SENTRY_SDIST_SHA256 = "27e4b512f47be93136645dbe8f02d430473ee73a389ce430b2b7104baa66cf75"  # trufflehog:ignore (public sdist hash under test)

# A non-Sentry credential shape that must still be caught inside a lockfile:
# a registry resolved URL carrying embedded basic-auth credentials.
BASIC_AUTH_RESOLVED = "https://alice:hunter2@registry.example.com/demo/-/demo-1.0.0.tgz"  # trufflehog:ignore (fake credential under test)

_SENTRY_SDIST_LINE = (
    'sdist = { url = "https://files.pythonhosted.org/packages/52/0a/'
    "e37553f3106d0f7f0d4b8ad2c7b92410d131929786ffd116833d3b8164df/"
    'sentry_sdk-2.70.0.tar.gz", hash = "sha256:'
    f"{SENTRY_SDIST_SHA256}"
    '", size = 1047234, upload-time = "2026-09-22T09:40:11.721Z" }'
)

_BUILD_CACHE_PATTERNS = [
    r"(^|/)target/",
    r"(^|/)\.venv/",
    r"(^|/)venv/",
    r"(^|/)node_modules/",
    r"(^|/)__pycache__/",
    r"(^|/)\.mypy_cache/",
    r"(^|/)\.pytest_cache/",
    r"(^|/)\.ruff_cache/",
    r"(^|/)(dist|build)/",
]

LOCKFILE_PATTERN = (
    r"(^|/)(uv\.lock|poetry\.lock|Pipfile\.lock|package-lock\.json|"
    r"pnpm-lock\.yaml|yarn\.lock|Cargo\.lock)$"
)

_EXCLUDE_CONTENT = "\n".join(_BUILD_CACHE_PATTERNS + [LOCKFILE_PATTERN]) + "\n"
_INCLUDE_CONTENT = LOCKFILE_PATTERN + "\n"


def _write_executable(path: Path, content: str) -> None:
    path.write_text(content, encoding="utf-8")
    path.chmod(0o755)


def _write_uv_lock(root: Path) -> None:
    (root / "uv.lock").write_text(
        "version = 1\n"
        "revision = 1\n"
        "requires-python = \">=3.11\"\n"
        "\n"
        "[[package]]\n"
        'name = "sentry-sdk"\n'
        'version = "2.70.0"\n'
        'source = { registry = "https://pypi.org/simple" }\n'
        f"{_SENTRY_SDIST_LINE}\n",
        encoding="utf-8",
    )


def _write_lockfile_with_basic_auth(root: Path) -> None:
    (root / "package-lock.json").write_text(
        "{\n"
        '  "name": "demo",\n'
        f'  "resolved": "{BASIC_AUTH_RESOLVED}"\n'
        "}\n",
        encoding="utf-8",
    )


def _run(root: Path) -> subprocess.CompletedProcess[str]:
    bin_dir = root / "bin"
    bin_dir.mkdir()
    trufflehog_args = root / "trufflehog-args.log"
    exclude_path_log = root / "trufflehog-exclude-path.log"
    exclude_exists_log = root / "trufflehog-exclude-exists.log"
    exclude_contents_log = root / "trufflehog-exclude-contents.log"
    include_path_log = root / "trufflehog-include-path.log"
    include_exists_log = root / "trufflehog-include-exists.log"
    include_contents_log = root / "trufflehog-include-contents.log"
    detectors_log = root / "trufflehog-detectors.log"

    _write_executable(
        bin_dir / "trufflehog",
        textwrap.dedent(
            f"""\
            #!/usr/bin/env bash
            set -euo pipefail
            printf '%s\\n' "$*" >> {trufflehog_args.as_posix()}
            exclude=""
            include=""
            detectors=""
            while [[ $# -gt 0 ]]; do
              case "$1" in
                --exclude-paths)
                  exclude="$2"
                  shift 2
                  ;;
                --include-paths)
                  include="$2"
                  shift 2
                  ;;
                --exclude-detectors)
                  detectors="$2"
                  shift 2
                  ;;
                *)
                  shift
                  ;;
              esac
            done
            if [[ -n "$exclude" ]]; then
              printf '%s\\n' "$exclude" >> {exclude_path_log.as_posix()}
              if [[ -f "$exclude" ]]; then
                printf 'yes\\n' >> {exclude_exists_log.as_posix()}
              else
                printf 'no\\n' >> {exclude_exists_log.as_posix()}
              fi
              cat "$exclude" > {exclude_contents_log.as_posix()}
            fi
            if [[ -n "$include" ]]; then
              printf '%s\\n' "$include" >> {include_path_log.as_posix()}
              if [[ -f "$include" ]]; then
                printf 'yes\\n' >> {include_exists_log.as_posix()}
              else
                printf 'no\\n' >> {include_exists_log.as_posix()}
              fi
              cat "$include" > {include_contents_log.as_posix()}
              printf '%s\\n' "$detectors" >> {detectors_log.as_posix()}
            fi
            exit 0
            """
        ),
    )

    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}{os.pathsep}{env['PATH']}"
    command = [str(SCRIPT)]
    if os.name == "nt":
        bash = shutil.which("bash")
        assert bash is not None
        command = [bash, str(SCRIPT)]
    return subprocess.run(
        command,
        cwd=root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )


def test_uses_real_temp_files_for_exclude_and_include_paths(tmp_path: Path) -> None:
    result = _run(tmp_path)

    assert result.returncode == 0
    args_lines = (tmp_path / "trufflehog-args.log").read_text(encoding="utf-8").strip().splitlines()
    assert len(args_lines) == 2
    assert args_lines[0].startswith("git file://. --since-commit HEAD --exclude-paths ")
    assert "--fail" in args_lines[0]
    assert args_lines[1].startswith("git file://. --since-commit HEAD --include-paths ")
    assert "--exclude-detectors SentryToken" in args_lines[1]
    assert "--fail" in args_lines[1]

    exclude_path = (tmp_path / "trufflehog-exclude-path.log").read_text(encoding="utf-8").strip()
    include_path = (tmp_path / "trufflehog-include-path.log").read_text(encoding="utf-8").strip()
    assert not exclude_path.startswith("/dev/fd/")
    assert not include_path.startswith("/dev/fd/")
    assert (tmp_path / "trufflehog-exclude-exists.log").read_text(encoding="utf-8").strip() == "yes"
    assert (tmp_path / "trufflehog-include-exists.log").read_text(encoding="utf-8").strip() == "yes"
    assert (tmp_path / "trufflehog-exclude-contents.log").read_text(encoding="utf-8") == _EXCLUDE_CONTENT
    assert (tmp_path / "trufflehog-include-contents.log").read_text(encoding="utf-8") == _INCLUDE_CONTENT
    assert (tmp_path / "trufflehog-detectors.log").read_text(encoding="utf-8").strip() == "SentryToken"


def _run_detection(root: Path, secret: str, detector: str) -> subprocess.CompletedProcess[str]:
    """Run the scan with a mock trufflehog that simulates both passes.

    The mock honors --exclude-paths, --include-paths, and --exclude-detectors:
    a file is in scope when an include-paths regex matches it, or when no
    exclude-paths regex matches it, and a finding is skipped when its detector
    is in the excluded set. A non-excluded finding in an in-scope file fails,
    mirroring --fail.
    """
    bin_dir = root / "bin"
    bin_dir.mkdir(exist_ok=True)

    _write_executable(
        bin_dir / "trufflehog",
        textwrap.dedent(
            """\
            #!/usr/bin/env bash
            set -euo pipefail
            secret="${MOCK_TRUFFLEHOG_SECRET:-}"
            [[ -n "$secret" ]] || exit 0
            detector="${MOCK_TRUFFLEHOG_DETECTOR:-SentryToken}"
            scope_file=""
            scope_mode=""
            excluded_detectors=""
            while [[ $# -gt 0 ]]; do
              case "$1" in
                --exclude-paths)
                  scope_file="$2"
                  scope_mode="exclude"
                  shift 2
                  ;;
                --include-paths)
                  scope_file="$2"
                  scope_mode="include"
                  shift 2
                  ;;
                --exclude-detectors)
                  excluded_detectors="$2"
                  shift 2
                  ;;
                *)
                  shift
                  ;;
              esac
            done
            if [[ -n "$excluded_detectors" ]]; then
              if [[ ",$excluded_detectors," == *",$detector,"* ]]; then
                exit 0
              fi
            fi
            while IFS= read -r file; do
              rel="${file#./}"
              in_scope=0
              while IFS= read -r pattern; do
                [[ -z "$pattern" ]] && continue
                if printf '%s\\n' "$rel" | grep -Eq -- "$pattern"; then
                  in_scope=1
                  break
                fi
              done < "$scope_file"
              if [[ "$scope_mode" == "include" ]]; then
                [[ "$in_scope" -eq 1 ]] || continue
              else
                [[ "$in_scope" -eq 0 ]] || continue
              fi
              printf 'found secret in %s\\n' "$rel" >&2
              exit 1
            done < <(grep -rlF -- "$secret" . 2>/dev/null)
            exit 0
            """
        ),
    )

    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}{os.pathsep}{env['PATH']}"
    env["MOCK_TRUFFLEHOG_SECRET"] = secret
    env["MOCK_TRUFFLEHOG_DETECTOR"] = detector
    command = [str(SCRIPT)]
    if os.name == "nt":
        bash = shutil.which("bash")
        assert bash is not None
        command = [bash, str(SCRIPT)]
    return subprocess.run(
        command,
        cwd=root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )


def test_lockfile_sentry_sdist_hash_passes(tmp_path: Path) -> None:
    _write_uv_lock(tmp_path)
    result = _run_detection(tmp_path, SENTRY_SDIST_SHA256, "SentryToken")
    assert result.returncode == 0, result.stderr


def test_non_lockfile_sentry_sdist_hash_still_fails(tmp_path: Path) -> None:
    (tmp_path / "src").mkdir()
    (tmp_path / "src" / "app.py").write_text(
        f"# unrelated source file\n_SENTRY_DSN = \"https://{SENTRY_SDIST_SHA256}@ingest.sentry.io/1\"\n",
        encoding="utf-8",
    )
    result = _run_detection(tmp_path, SENTRY_SDIST_SHA256, "SentryToken")
    assert result.returncode != 0


def test_lockfile_non_sentry_secret_still_fails(tmp_path: Path) -> None:
    _write_lockfile_with_basic_auth(tmp_path)
    result = _run_detection(tmp_path, BASIC_AUTH_RESOLVED, "URI")
    assert result.returncode != 0
