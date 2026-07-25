"""Tests for scripts/docker-credential-forgejo-ssm."""
from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "docker-credential-forgejo-ssm"
BASH = (
    r"C:\Program Files\Git\bin\bash.exe"
    if os.name == "nt"
    else "bash"
)


def _bash_path(path: Path) -> str:
    if os.name != "nt":
        return str(path)
    resolved = path.resolve()
    return f"/{resolved.drive[0].lower()}{resolved.as_posix()[2:]}"


def _run_helper(tmp_path: Path, *, read_token: str = "read-token") -> subprocess.CompletedProcess[str]:
    bash_env = tmp_path / "bash-env"
    bash_env.write_text(
        """\
aws() {
  if [[ "${1:-}" == ssm && "${2:-}" == get-parameter ]]; then
    local name=""
    shift 2
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --name)
          name="$2"
          shift 2
          ;;
        --query|--output)
          shift 2
          ;;
        --with-decryption)
          shift 1
          ;;
        *)
          shift
          ;;
      esac
    done
    case "$name" in
      */registry-read-token)
        [[ -n "${FAKE_REGISTRY_READ_TOKEN:-}" ]] || return 1
        printf '%s' "$FAKE_REGISTRY_READ_TOKEN"
        ;;
      */registry-token)
        printf '%s' "publish-token"
        ;;
      *)
        return 1
        ;;
    esac
    return 0
  fi
  return 1
}
""",
        encoding="utf-8",
    )
    env = os.environ.copy()
    env.update(
        {
            "BASH_ENV": _bash_path(bash_env),
            "FAKE_REGISTRY_READ_TOKEN": read_token,
        }
    )
    return subprocess.run(
        [BASH, _bash_path(SCRIPT), "get"],
        input="forgejo.coilysiren.me\n",
        cwd=tmp_path,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )


def test_serves_read_token_when_available(tmp_path: Path) -> None:
    result = _run_helper(tmp_path)

    assert result.returncode == 0
    assert result.stderr == ""
    assert json.loads(result.stdout) == {
        "ServerURL": "forgejo.coilysiren.me",
        "Username": "coilyco-ops",
        "Secret": "read-token",
    }


def test_falls_back_to_registry_publish_token(tmp_path: Path) -> None:
    result = _run_helper(tmp_path, read_token="")

    assert result.returncode == 0
    assert "falling back to registry publish token" in result.stderr
    assert json.loads(result.stdout) == {
        "ServerURL": "forgejo.coilysiren.me",
        "Username": "coilyco-ops",
        "Secret": "publish-token",
    }


def test_rejects_other_registries(tmp_path: Path) -> None:
    result = subprocess.run(
        [BASH, _bash_path(SCRIPT), "get"],
        input="example.invalid\n",
        cwd=tmp_path,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 1
    assert "unsupported registry: example.invalid" in result.stderr
