from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
MODULE = ROOT / ".umbra" / "guardfiles" / "aosguard" / "netlify_domain_alias.py"


def _load():
    spec = importlib.util.spec_from_file_location("netlify_domain_alias", MODULE)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


@pytest.fixture()
def netlify(monkeypatch):
    module = _load()
    monkeypatch.setenv("NETLIFY_AUTH_TOKEN", "test-token")
    return module


def _stub(module, monkeypatch, site, sent):
    def request(method, path, token, payload=None):
        if method == "PATCH":
            sent.append(payload)
            return {**site, **payload}
        return site

    monkeypatch.setattr(module, "_request", request)


# The API replaces domain_aliases rather than merging, so a write that carried
# only the new alias would delete the others. This is that guard.
def test_add_preserves_existing_aliases(netlify, monkeypatch, capsys):
    site = {
        "name": "s",
        "custom_domain": "www.example.com",
        "domain_aliases": ["one.example.com"],
    }
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    netlify.main(["add", "--site", "s", "--alias", "two.example.com"])

    assert sent == [{"domain_aliases": ["one.example.com", "two.example.com"]}]


def test_add_batches_every_alias_into_one_write(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "www.example.com", "domain_aliases": []}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    netlify.main(["add", "--site", "s", "--alias", "a.example.com", "--alias", "b.example.com"])

    # One PATCH, not one per alias: each write re-issues the certificate.
    assert len(sent) == 1
    assert sent[0]["domain_aliases"] == ["a.example.com", "b.example.com"]


def test_add_is_idempotent_and_writes_nothing_when_present(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "w", "domain_aliases": ["a.example.com"]}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    netlify.main(["add", "--site", "s", "--alias", "a.example.com"])

    assert sent == []


def test_add_refuses_the_primary_domain(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "www.example.com", "domain_aliases": []}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    with pytest.raises(SystemExit):
        netlify.main(["add", "--site", "s", "--alias", "www.example.com"])
    assert sent == []


def test_add_refuses_an_empty_alias_list(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "w", "domain_aliases": []}
    _stub(netlify, monkeypatch, site, [])

    with pytest.raises(SystemExit):
        netlify.main(["add", "--site", "s"])


def test_missing_token_fails_closed(netlify, monkeypatch):
    monkeypatch.setenv("NETLIFY_AUTH_TOKEN", "")
    with pytest.raises(SystemExit):
        netlify.main(["show", "--site", "s"])


# A rename is a removal and an addition of the same thing. Two calls would be
# two certificate events on a live site, so it has to be one write.
def test_rename_is_one_write(netlify, monkeypatch):
    site = {
        "name": "s",
        "custom_domain": "www.example.com",
        "domain_aliases": ["keep.example.com", "old.example.com"],
    }
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    netlify.main(["add", "--site", "s", "--alias", "new.example.com", "--remove", "old.example.com"])

    assert len(sent) == 1
    assert sent[0]["domain_aliases"] == ["keep.example.com", "new.example.com"]


def test_remove_refuses_an_alias_the_site_does_not_have(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "w", "domain_aliases": ["a.example.com"]}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    with pytest.raises(SystemExit):
        netlify.main(["add", "--site", "s", "--remove", "typo.example.com"])
    assert sent == []


def test_remove_refuses_the_primary_domain(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "www.example.com", "domain_aliases": []}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    with pytest.raises(SystemExit):
        netlify.main(["add", "--site", "s", "--remove", "www.example.com"])
    assert sent == []


def test_the_same_name_cannot_be_added_and_removed(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "w", "domain_aliases": []}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    with pytest.raises(SystemExit):
        netlify.main(["add", "--site", "s", "--alias", "x.example.com", "--remove", "x.example.com"])
    assert sent == []


# Shedding every alias is allowed: the surface would otherwise be one-way.
def test_removing_every_alias_is_permitted(netlify, monkeypatch):
    site = {"name": "s", "custom_domain": "w", "domain_aliases": ["a.example.com"]}
    sent: list[dict] = []
    _stub(netlify, monkeypatch, site, sent)

    netlify.main(["add", "--site", "s", "--remove", "a.example.com"])

    assert sent == [{"domain_aliases": []}]
