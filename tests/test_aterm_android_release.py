"""Exercise the aterm Android stable channel scripts against a scratch repo and a stub gh."""

from __future__ import annotations

import json
import os
from pathlib import Path
import stat
import subprocess

ROOT = Path(__file__).resolve().parent.parent
RELEASE = ROOT / "scripts" / "ci" / "aterm-android-release.sh"
BUILD = ROOT / "scripts" / "ci" / "aterm-android.sh"


# A host git config may sign commits through a remote key and run hooks, so read none.
NO_HOST_GIT = {"GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_SYSTEM": os.devnull}


def _git(repo: Path, *args: str) -> str:
    done = subprocess.run(
        ["git", "-c", "user.name=t", "-c", "user.email=t@example.com", *args],
        cwd=repo,
        env=os.environ | NO_HOST_GIT,
        capture_output=True,
        text=True,
        check=True,
    )
    return done.stdout.strip()


def _commit(repo: Path, path: str, subject: str) -> None:
    target = repo / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(subject, encoding="utf-8")
    _git(repo, "add", path)
    _git(repo, "commit", "-m", subject)


def _repo(tmp_path: Path) -> Path:
    repo = tmp_path / "repo"
    repo.mkdir()
    _git(repo, "init", "-b", "main")
    _commit(repo, "README.md", "chore: start")
    return repo


def _release(repo: Path, *args: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", str(RELEASE), *args],
        cwd=repo,
        env=os.environ | NO_HOST_GIT | (env or {}),
        capture_output=True,
        text=True,
        check=False,
    )


def test_version_code_is_minutes_since_the_epoch() -> None:
    done = subprocess.run(["bash", str(BUILD), "version-code", "1788000000"], capture_output=True, text=True, check=True)

    assert done.stdout.strip() == "29800000"


def test_version_code_today_exceeds_every_code_the_old_scheme_made() -> None:
    done = subprocess.run(["bash", str(BUILD), "version-code"], capture_output=True, text=True, check=True)

    assert 1_000_000 <= int(done.stdout) <= 2_100_000_000


def test_build_refuses_a_version_code_below_the_old_scheme(tmp_path: Path) -> None:
    done = subprocess.run(
        ["bash", str(BUILD), "build", "0.31.0", str(tmp_path / "out.apk")],
        env=os.environ | {"ATERM_ANDROID_VERSION_CODE": "30000"},
        capture_output=True,
        text=True,
        check=False,
    )

    assert done.returncode == 1
    assert "version code must be" in done.stderr


def test_a_push_builds_and_never_publishes(tmp_path: Path) -> None:
    done = _release(_repo(tmp_path), "plan", env={"EVENT_NAME": "push"})

    assert "build=true publish=false stable=false" in done.stdout


def test_a_dispatch_publishes_unless_it_is_a_dry_run(tmp_path: Path) -> None:
    repo = _repo(tmp_path)

    live = _release(repo, "plan", env={"EVENT_NAME": "workflow_dispatch", "DRY_RUN": "false"})
    dry = _release(repo, "plan", env={"EVENT_NAME": "workflow_dispatch", "DRY_RUN": "true"})

    assert "build=true publish=true stable=true" in live.stdout
    assert "build=true publish=false stable=true" in dry.stdout


def test_the_schedule_skips_a_day_with_no_app_change(tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    _commit(repo, "aterm-android/a.txt", "feat(aterm-android): first")
    _git(repo, "tag", "aterm-android-v0.1.0")
    _commit(repo, "docs/unrelated.md", "docs: unrelated")

    done = _release(repo, "plan", env={"EVENT_NAME": "schedule"})

    assert "previous_tag=aterm-android-v0.1.0 build=false publish=false stable=false" in done.stdout


def test_the_schedule_publishes_after_an_app_change(tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    _git(repo, "tag", "aterm-android-v0.1.0")
    _commit(repo, "aterm-client/src/x.ts", "fix(aterm-client): a thing")

    done = _release(repo, "plan", env={"EVENT_NAME": "schedule"})

    assert "build=true publish=true stable=true" in done.stdout


def test_the_previous_tag_is_the_newest_by_version_not_by_name(tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    for tag in ("aterm-android-v0.9.0", "aterm-android-v0.10.0", "aos-v0.500.0"):
        _git(repo, "tag", tag)

    assert _release(repo, "previous_tag").stdout.strip() == "aterm-android-v0.10.0"


def test_notes_list_one_line_per_app_change_without_the_commit_prefix(tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    _git(repo, "tag", "aterm-android-v0.1.0")
    _commit(repo, "aterm-client/a.ts", "feat(aterm-client): a card shows its text (COI-1)")
    _commit(repo, "aterm/daemon.go", "feat(aterm): daemon only change")
    _commit(repo, "aterm-android/b.rs", "fix(aterm-android)!: keystore alias moves")

    body = _release(repo, "notes", "aterm-android-v0.1.0", "0.2.0", "29800000", "0123456789abcdef").stdout

    assert body.splitlines()[0] == "aterm Android 0.2.0, version code 29800000, built from 01234567."
    assert "* a card shows its text (COI-1)" in body
    assert "* keystore alias moves" in body
    assert "daemon only change" not in body


def test_notes_for_a_rebuild_say_nothing_changed(tmp_path: Path) -> None:
    repo = _repo(tmp_path)
    _git(repo, "tag", "aterm-android-v0.1.0")

    body = _release(repo, "notes", "aterm-android-v0.1.0", "0.2.0", "29800000", "abc").stdout

    assert "No app changes since aterm-android-v0.1.0" in body


def _stub_gh(tmp_path: Path, existing: set[str]) -> tuple[Path, Path]:
    bindir = tmp_path / "bin"
    bindir.mkdir()
    log = tmp_path / "gh.log"
    stub = bindir / "gh"
    known = " ".join(sorted(existing))
    stub.write_text(
        "#!/bin/sh\n"
        f'echo "$@" >> "{log}"\n'
        'if [ "$1 $2" = "release view" ]; then\n'
        f'  for t in {known}; do [ "$t" = "$3" ] && exit 0; done\n'
        "  exit 1\n"
        "fi\n",
        encoding="utf-8",
    )
    stub.chmod(stub.stat().st_mode | stat.S_IXUSR)
    return bindir, log


def _publish(tmp_path: Path, existing: set[str]) -> list[str]:
    bindir, log = _stub_gh(tmp_path, existing)
    apk = tmp_path / "aterm.apk"
    apk.write_bytes(b"apk")
    notes = tmp_path / "notes.md"
    notes.write_text("notes", encoding="utf-8")
    done = _release(
        tmp_path,
        "publish",
        "aterm-android-v0.2.0",
        str(apk),
        str(notes),
        "abc123",
        env={"PATH": f"{bindir}{os.pathsep}{os.environ['PATH']}", "GITHUB_REPOSITORY": "o/r"},
    )
    assert done.returncode == 0, done.stderr
    return [line for line in log.read_text(encoding="utf-8").splitlines() if not line.startswith("release view")]


def test_publish_creates_both_releases_under_their_fixed_titles(tmp_path: Path) -> None:
    calls = _publish(tmp_path, set())

    assert calls[0].startswith("release create aterm-android-v0.2.0 ")
    assert "--title aterm Android stable " in calls[0]
    assert "--latest=false" in calls[0]
    assert calls[1].startswith("release upload aterm-android-v0.2.0 ")
    assert "--title aterm Android newest (fixed link) " in calls[2]
    assert "--prerelease" in calls[2]
    assert calls[3].startswith("release upload aterm-android-newest ")
    assert all("--clobber" in line and "/aterm.apk " in line for line in (calls[1], calls[3]))


def test_publish_converges_when_both_releases_already_exist(tmp_path: Path) -> None:
    calls = _publish(tmp_path, {"aterm-android-v0.2.0", "aterm-android-newest"})

    assert [line.split()[1] for line in calls] == ["edit", "upload", "edit", "upload"]


PLAY = ROOT / "scripts" / "ci" / "aterm-android-play.sh"


def _play(tmp_path: Path, *, real_upload_key: bool) -> tuple[subprocess.CompletedProcess[str], list[str]]:
    bindir = tmp_path / "bin"
    bindir.mkdir()
    log = tmp_path / "curl.log"
    stub = bindir / "curl"
    stub.write_text(
        "#!/bin/sh\n"
        f'echo "$@" >> "{log}"\n'
        'case "$*" in\n'
        '  *oauth.test*) echo \'{"access_token":"tok"}\' ;;\n'
        '  *:commit*) echo "{}" ;;\n'
        '  *"/bundles?"*) echo \'{"versionCode":42}\' ;;\n'
        '  *"/tracks/"*) echo "{}" ;;\n'
        '  *) echo \'{"id":"e1"}\' ;;\n'
        "esac\n",
        encoding="utf-8",
    )
    stub.chmod(stub.stat().st_mode | stat.S_IXUSR)
    key = subprocess.run(["openssl", "genrsa", "2048"], capture_output=True, text=True, check=True).stdout
    account = json.dumps({"client_email": "svc@example.test", "private_key": key})
    aab, notes = tmp_path / "aterm.aab", tmp_path / "notes.md"
    aab.write_bytes(b"aab")
    notes.write_text("one line of notes", encoding="utf-8")
    env = {
        "PATH": f"{bindir}{os.pathsep}{os.environ['PATH']}",
        "PLAY_SERVICE_ACCOUNT_JSON": account,
        "PLAY_API": "http://play.test",
        "PLAY_TOKEN_URL": "http://oauth.test/token",
    }
    if real_upload_key:
        env["ATERM_ANDROID_UPLOAD_KEYSTORE_B64"] = "x"
    done = subprocess.run(
        ["bash", str(PLAY), "upload", str(aab), "29897091", str(notes)],
        env=os.environ | env,
        capture_output=True,
        text=True,
        check=False,
    )
    return done, log.read_text(encoding="utf-8").splitlines() if log.exists() else []


def test_play_upload_runs_one_edit_that_ends_in_a_commit(tmp_path: Path) -> None:
    done, calls = _play(tmp_path, real_upload_key=True)

    assert done.returncode == 0, done.stderr
    assert "uploaded version code 29897091 to the internal track as draft" in done.stdout
    assert "oauth.test/token" in calls[0] and "assertion=" in calls[0]
    assert "applications/dev.coilyco.treehouse/edits " in calls[1] + " "
    assert "edits/e1/bundles?uploadType=media" in calls[2]
    assert "-X PUT" in calls[3] and "edits/e1/tracks/internal" in calls[3]
    assert '"status":"draft"' in calls[3] and '"versionCodes":["29897091"]' in calls[3]
    assert calls[4].endswith("edits/e1:commit")


def test_play_upload_refuses_a_throwaway_signed_bundle_before_any_request(tmp_path: Path) -> None:
    done, calls = _play(tmp_path, real_upload_key=False)

    assert done.returncode == 1
    assert "throwaway-signed" in done.stderr
    assert calls == []
