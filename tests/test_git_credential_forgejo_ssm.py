"""The Forgejo git credential helper reads SSM once per burst, never twice per request.

Pins teable:coilyco/agentic-os#8222: partial-clone lazy fetches ran the helper about 20
times a minute, each an SSM and an STS call. The stub `aws` below counts reads, and the
cache runs against a throwaway socket directory so no real daemon or token is touched.
"""

from __future__ import annotations

import os
import subprocess
import time
from pathlib import Path

import pytest

HELPER = Path(__file__).resolve().parents[1] / "scripts" / "git-credential-forgejo-ssm.sh"
HOST = "forgejo.coilysiren.me"
REQUEST = f"protocol=https\nhost={HOST}\n\n"


class Harness:
    def __init__(self, root: Path) -> None:
        self.root = root
        self.calls = root / "aws-calls"
        self.token = root / "token"
        self.token.write_text("tok-one")
        self.home = root / "home"
        self.cache_home = root / "cache"
        self.home.mkdir()
        self.cache_home.mkdir()
        self.aws = root / "aws"
        self.aws.write_text(
            f'#!/bin/sh\necho call >> "{self.calls}"\nprintf "%s\\n" "$(cat "{self.token}")"\n'
        )
        self.aws.chmod(0o755)

    def env(self, **extra: str) -> dict[str, str]:
        env = {
            "PATH": os.environ["PATH"],
            "HOME": str(self.home),
            "XDG_CACHE_HOME": str(self.cache_home),
            "FORGEJO_CREDENTIAL_AWS": str(self.aws),
        }
        env.update(extra)
        return env

    def run(self, op: str, request: str = REQUEST, **extra: str) -> str:
        done = subprocess.run(
            ["bash", str(HELPER), op],
            input=request,
            capture_output=True,
            text=True,
            env=self.env(**extra),
            check=False,
            timeout=30,
        )
        assert done.returncode == 0, done.stderr
        return done.stdout

    def reads(self) -> int:
        return len(self.calls.read_text().splitlines()) if self.calls.exists() else 0

    def stop_daemon(self) -> None:
        subprocess.run(
            ["git", "credential-cache", "exit"], env=self.env(), capture_output=True, check=False
        )


@pytest.fixture
def h(tmp_path: Path):
    harness = Harness(tmp_path)
    yield harness
    harness.stop_daemon()


def test_a_burst_of_gets_reads_ssm_once(h: Harness) -> None:
    first = h.run("get")
    assert first == "username=coilyco-ops\npassword=tok-one\n"
    for _ in range(19):
        assert h.run("get") == first
    assert h.reads() == 1


def test_with_the_cache_off_every_get_reads_ssm(h: Harness) -> None:
    # The state before the cache: 20 requests are 20 SSM reads.
    for _ in range(20):
        h.run("get", FORGEJO_CREDENTIAL_CACHE_SECONDS="0")
    assert h.reads() == 20


def test_erase_drops_the_cached_token_so_a_rotation_takes_effect(h: Harness) -> None:
    h.run("get")
    h.token.write_text("tok-two")
    assert "tok-one" in h.run("get"), "within the TTL the old token is served until git rejects it"
    h.run("erase", request=f"{REQUEST.rstrip()}\nusername=coilyco-ops\npassword=tok-one\n\n")
    assert h.run("get") == "username=coilyco-ops\npassword=tok-two\n"
    assert h.reads() == 2


def test_an_expired_entry_is_read_again(h: Harness) -> None:
    h.run("get", FORGEJO_CREDENTIAL_CACHE_SECONDS="1")
    time.sleep(2.2)
    h.run("get", FORGEJO_CREDENTIAL_CACHE_SECONDS="1")
    assert h.reads() == 2


def test_the_token_is_never_written_to_a_file(h: Harness) -> None:
    h.run("get")
    h.run("get")
    leaked = [
        path
        for path in h.root.rglob("*")
        if path.is_file() and path != h.token and b"tok-one" in path.read_bytes()
    ]
    assert leaked == []


def test_store_and_a_foreign_host_do_nothing(h: Harness) -> None:
    assert h.run("store", request=f"{REQUEST.rstrip()}\nusername=x\npassword=y\n\n") == ""
    assert h.run("get", request="protocol=https\nhost=github.com\n\n") == ""
    assert h.reads() == 0
    # And store seeded nothing, so the first real get still reads SSM.
    h.run("get")
    assert h.reads() == 1


def test_a_cache_that_cannot_start_degrades_to_one_read_per_get(h: Harness) -> None:
    broken = h.root / "not-a-directory"
    broken.write_text("x")
    for _ in range(3):
        out = h.run("get", XDG_CACHE_HOME=str(broken))
        assert out == "username=coilyco-ops\npassword=tok-one\n"
    assert h.reads() == 3
