"""Tests for the Actions-secret source manifest loader."""

from __future__ import annotations

import importlib.util
import json
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "sync-actions-secrets.py"


def _load_script():
    spec = importlib.util.spec_from_file_location("sync_actions_secrets", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_real_action_defaults_manifest_is_valid() -> None:
    mod = _load_script()
    sources = mod.load_secret_sources(mod.TELEGRAM_DEFAULTS_PATH)
    assert sources
    assert sources.items() <= mod.MAPPING[mod.slug("agentic-os")].items()


def test_slug_defaults_to_the_release_train_owner() -> None:
    mod = _load_script()
    assert mod.slug("agentic-os") == f"{mod.OWNER}/agentic-os"
    assert mod.slug("deploy", "coilyco-bridge") == "coilyco-bridge/deploy"


def test_mapping_keys_are_owner_qualified() -> None:
    """Every key must be owner/repo, since put_secret interpolates it directly."""
    mod = _load_script()
    assert mod.MAPPING
    for key in mod.MAPPING:
        owner, sep, repo = key.partition("/")
        assert sep and owner and repo and "/" not in repo, key


def test_mapping_spans_more_than_one_owner() -> None:
    """Guards the cross-org shape against a regression back to a single OWNER."""
    mod = _load_script()
    owners = {key.split("/", 1)[0] for key in mod.MAPPING}
    assert {"coilyco-flight-deck", "coilyco-bridge"} <= owners


def test_deploy_pin_reconciler_secrets_are_mapped() -> None:
    mod = _load_script()
    deploy = mod.MAPPING[mod.slug("deploy", "coilyco-bridge")]
    assert deploy["DEPLOY_PUSH_TOKEN"] == "/forgejo/coilyco-ops/ci-release-token"
    assert (
        deploy["REGISTRY_READ_TOKEN"] == "/forgejo/coilyco-ops/registry-read-token"
    )


def test_housecast_publishes_with_a_pypi_token() -> None:
    """Forgejo is not a PyPI trusted-publishing provider, so the token is the only path."""
    mod = _load_script()
    housecast = mod.MAPPING[mod.slug("housecast")]
    assert housecast["PYPI_TOKEN"] == "/coilysiren/pypi/token"


def test_every_mapped_secret_name_is_acceptable_to_forgejo() -> None:
    """The real mapping must not contain a name the server would 400 on."""
    mod = _load_script()
    assert mod.invalid_secret_names(mod.MAPPING) == []


@pytest.mark.parametrize(
    "name",
    [
        "FORGEJO_REGISTRY_READ_TOKEN",
        "forgejo_lowercase_is_also_reserved",
        "GITEA_TOKEN",
        "GITHUB_TOKEN",
        "1LEADING_DIGIT",
        "HAS-A-DASH",
        "",
    ],
)
def test_invalid_secret_names_flags_rejected_names(name) -> None:
    mod = _load_script()
    assert mod.invalid_secret_names({"o/r": {name: "/p"}}) == [f"o/r: {name}"]


@pytest.mark.parametrize(
    "name", ["DEPLOY_PUSH_TOKEN", "REGISTRY_READ_TOKEN", "_UNDERSCORE_LEAD", "A1"]
)
def test_invalid_secret_names_accepts_valid_names(name) -> None:
    mod = _load_script()
    assert mod.invalid_secret_names({"o/r": {name: "/p"}}) == []


def test_put_secret_targets_the_mapping_key(monkeypatch) -> None:
    """The URL must carry the entry's own owner, not a module-level default."""
    mod = _load_script()
    seen: dict[str, str] = {}

    class _Response:
        def __enter__(self):
            return self

        def __exit__(self, *_):
            return False

        def read(self):
            return b""

    def _fake_urlopen(request, **_):
        seen["url"] = request.full_url
        return _Response()

    monkeypatch.setattr(mod.urllib.request, "urlopen", _fake_urlopen)
    mod.put_secret("t", "coilyco-bridge/deploy", "DEPLOY_PUSH_TOKEN", "v")

    assert seen["url"] == (
        f"{mod.FORGEJO_BASE}/repos/coilyco-bridge/deploy"
        "/actions/secrets/DEPLOY_PUSH_TOKEN"
    )


def test_manifest_loader_rejects_missing_parameter_path(tmp_path) -> None:
    mod = _load_script()
    manifest = tmp_path / "defaults.json"
    manifest.write_text(
        json.dumps(
            {
                "schema-version": 1,
                "secrets": {
                    "token": {
                        "actions-secret": "TOKEN",
                        "ssm-parameter": "not-an-absolute-path",
                    }
                },
            }
        ),
        encoding="utf-8",
    )
    with pytest.raises(SystemExit, match="invalid action secret sources"):
        mod.load_secret_sources(manifest)


def test_admin_token_requires_attended_environment(monkeypatch) -> None:
    mod = _load_script()
    monkeypatch.delenv("FORGEJO_ADMIN_TOKEN", raising=False)

    with pytest.raises(SystemExit, match="FORGEJO_ADMIN_TOKEN is required"):
        mod.admin_token()


def test_admin_token_reads_attended_environment(monkeypatch) -> None:
    mod = _load_script()
    monkeypatch.setenv("FORGEJO_ADMIN_TOKEN", " fixture-token ")

    assert mod.admin_token() == "fixture-token"
