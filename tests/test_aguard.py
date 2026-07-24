"""Static contract for the independent aguard specgen project."""

from __future__ import annotations

import gzip
import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PROJECT = ROOT / ".specgen"
SOURCE = PROJECT / "aguard"
EXPECTED_MEMBERS = {
    "actions.kdl",
    "aws.kdl",
    "forgejo.kdl",
    "kubectl.kdl",
    "tailscale.kdl",
}


def test_aguard_has_one_static_binary_group() -> None:
    members = sorted(SOURCE.glob("*.kdl"))
    assert {member.name for member in members} == EXPECTED_MEMBERS

    wraps: set[str] = set()
    for member in members:
        text = member.read_text(encoding="utf-8")
        wraps.update(re.findall(r"(?m)^wrap\s+(\S+)", text))
        assert "wrap aos-agent " not in text
        assert "wrap aos-ward " not in text
        assert 'argv ".ward/' not in text

    assert wraps == {"aguard"}


def test_aguard_actions_use_packaged_python_modules() -> None:
    text = (SOURCE / "actions.kdl").read_text(encoding="utf-8")

    assert "exec python3" in text
    assert ".specgen/aguard/scripts/" not in text
    for module in (
        "forgejo_actions_list",
        "forgejo_actions_logs",
        "forgejo_actions_rerun",
    ):
        assert f'"agentic_os.{module}"' in text
        assert (ROOT / "agentic_os" / f"{module}.py").is_file()


def test_aguard_vendored_forgejo_contract_is_json() -> None:
    vendored = SOURCE / "forgejo.swagger.v1.json"
    assert json.loads(vendored.read_text(encoding="utf-8"))


def test_aguard_forgejo_lock_is_encoded_json() -> None:
    encoded = SOURCE / "forgejo.swagger.lock.json.gz"
    legacy = SOURCE / "forgejo.swagger.lock.json"

    assert encoded.is_file()
    assert not legacy.exists()
    assert json.loads(gzip.decompress(encoded.read_bytes()))


def test_aguard_dependency_lock_is_committed() -> None:
    lock = json.loads((PROJECT / "specverb.lock").read_text(encoding="utf-8"))
    assert lock["cliGuard"].startswith("v")
    assert not list(SOURCE.glob("*.md"))


def test_native_release_wrapper_embeds_the_actions_bridge() -> None:
    wrapper = ROOT / "aguard-release" / "main.go"
    text = wrapper.read_text(encoding="utf-8")

    assert "//go:embed payload/aguard payload/agentic_os/*" in text
    assert "PYTHONPATH=" in text
    build = (ROOT / "scripts" / "aos-release-build.sh").read_text(encoding="utf-8")
    assert "gzip -dc" in build
    for module in (
        "forgejo_actions_list.py",
        "forgejo_actions_logs.py",
        "forgejo_actions_rerun.py",
        "forgejo_actions_web.py",
    ):
        assert f'"$repo_root/agentic_os/{module}"' in build
