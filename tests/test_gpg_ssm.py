"""Tests for scripts/gpg-ssm."""
from __future__ import annotations

import subprocess
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "gpg-ssm"


def _write_executable(path: Path, text: str) -> None:
    path.write_text(text, encoding="utf-8")
    path.chmod(0o755)


def _make_fake_tools(root: Path, *, has_secret_key: bool) -> Path:
    bin_dir = root / "bin"
    bin_dir.mkdir()
    list_secret_keys = 'printf "sec:u:4096:1:ABCDEF1234567890:::\\n"; exit 0' if has_secret_key else "exit 1"

    _write_executable(
        bin_dir / "uname",
        "#!/usr/bin/env bash\n"
        "printf 'MSYS_NT-10.0\\n'\n",
    )

    _write_executable(
        bin_dir / "ward",
        "#!/usr/bin/env bash\n"
        "log=\"$TMPDIR/ward.log\"\n"
        "printf '%s\\n' \"$*\" >> \"$log\"\n"
        "case \"$*\" in\n"
        "  'ops aws sts get-caller-identity') exit 0 ;;\n"
        "  'ops aws ssm get-parameter'*)\n"
        "    case \"$*\" in\n"
        "      *'/coilysiren/gpg-secret-key'*) printf 'FAKE-SECRET-KEY\\n' ;;\n"
        "      *'/coilysiren/gpg-passphrase'*) printf 'FAKE-PASSPHRASE' ;;\n"
        "      *) exit 1 ;;\n"
        "    esac\n"
        "    exit 0\n"
        "    ;;\n"
        "esac\n"
        "exit 1\n",
    )

    _write_executable(
        bin_dir / "gpg",
        "#!/usr/bin/env bash\n"
        "log=\"$TMPDIR/gpg.log\"\n"
        "printf '%s\\n' \"$*\" >> \"$log\"\n"
        "printf 'MSYS_NO_PATHCONV=%s\\n' \"${MSYS_NO_PATHCONV-}\" >> \"$TMPDIR/gpg-env.log\"\n"
        "case \"$*\" in\n"
        "  *'--list-secret-keys'*)\n"
        f"    {list_secret_keys}\n"
        "    ;;\n"
        "  *'--import'*)\n"
        "    cat > \"$TMPDIR/imported-key\"\n"
        "    exit 0\n"
        "    ;;\n"
        "esac\n"
        "if printf '%s\\n' \"$*\" | grep -q -- '--passphrase-fd'; then\n"
        "  cat <&3 > \"$TMPDIR/passphrase\"\n"
        "fi\n"
        "exit 0\n",
    )

    return bin_dir


def _run_gpg_ssm(home: Path, path: Path, *args: str) -> subprocess.CompletedProcess[str]:
    env = {
        "HOME": str(home),
        "TMPDIR": str(home),
        "PATH": f"{path}:/usr/bin:/bin",
    }
    return subprocess.run(
        [str(SCRIPT), *args],
        capture_output=True,
        text=True,
        check=False,
        env=env,
    )


def test_passthrough_non_signing_invocation(tmp_path: Path) -> None:
    home = tmp_path / "home"
    home.mkdir()
    bin_dir = _make_fake_tools(tmp_path, has_secret_key=False)

    proc = _run_gpg_ssm(home, bin_dir, "--verify", "artifact.sig")

    assert proc.returncode == 0
    assert proc.stdout == ""
    assert proc.stderr == ""
    assert (home / "gpg.log").read_text(encoding="utf-8").strip() == "--verify artifact.sig"
    assert not (home / "ward.log").exists()


def test_signing_bootstraps_shared_secret_key(tmp_path: Path) -> None:
    home = tmp_path / "home"
    home.mkdir()
    (home / ".gitconfig").write_text("[user]\n\tsigningkey = ABCDEF1234567890\n", encoding="utf-8")
    bin_dir = _make_fake_tools(tmp_path, has_secret_key=False)

    proc = _run_gpg_ssm(home, bin_dir, "-bs")

    assert proc.returncode == 0
    assert proc.stdout == ""
    assert proc.stderr == ""
    assert (home / "ward.log").read_text(encoding="utf-8").splitlines() == [
        "ops aws sts get-caller-identity",
        "ops aws ssm get-parameter --name /coilysiren/gpg-secret-key --with-decryption --query Parameter.Value --output text",
        "ops aws ssm get-parameter --name /coilysiren/gpg-passphrase --with-decryption --query Parameter.Value --output text",
    ]
    assert (home / "imported-key").read_text(encoding="utf-8") == "FAKE-SECRET-KEY"
    assert (home / "passphrase").read_text(encoding="utf-8") == "FAKE-PASSPHRASE"
    assert "MSYS_NO_PATHCONV=1" in (home / "gpg-env.log").read_text(encoding="utf-8")


def test_signing_skips_secret_key_import_when_local_key_exists(tmp_path: Path) -> None:
    home = tmp_path / "home"
    home.mkdir()
    (home / ".gitconfig").write_text("[user]\n\tsigningkey = ABCDEF1234567890\n", encoding="utf-8")
    bin_dir = _make_fake_tools(tmp_path, has_secret_key=True)

    proc = _run_gpg_ssm(home, bin_dir, "-bs")

    assert proc.returncode == 0
    assert proc.stdout == ""
    assert proc.stderr == ""
    assert (home / "ward.log").read_text(encoding="utf-8").splitlines() == [
        "ops aws sts get-caller-identity",
        "ops aws ssm get-parameter --name /coilysiren/gpg-passphrase --with-decryption --query Parameter.Value --output text",
    ]
    assert not (home / "imported-key").exists()
