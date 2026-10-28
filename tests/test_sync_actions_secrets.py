"""Tests for the Actions-secret source manifest loader."""

from __future__ import annotations

import importlib.util
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


def test_deploy_pin_reconciler_secrets_are_mapped() -> None:
    mod = _load_script()
    deploy = mod.MAPPING[mod.slug("deploy")]
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


def test_apply_plan_reports_every_failure_and_still_writes_the_rest() -> None:
    """A 404 on one repo must not stop the entries after it (COI-2634)."""
    mod = _load_script()
    written: list[tuple[str, str]] = []

    def _read(param):
        if param == "/empty":
            raise SystemExit("ssm parameter /empty resolved empty")
        return "secret-value"

    def _write(token, repo_slug, name, value):
        if repo_slug == "o/gone":
            raise mod.urllib.error.HTTPError("u", 404, "Not Found", {}, None)
        written.append((repo_slug, name))

    plan = [
        ("o/gone", "A", "/p"),
        ("o/ok", "B", "/p"),
        ("o/ok", "C", "/empty"),
        ("o/ok", "D", "/p"),
    ]
    failures = mod.apply_plan(plan, "t", read=_read, write=_write)

    assert written == [("o/ok", "B"), ("o/ok", "D")]
    assert failures == ["o/gone: A: HTTP 404", "o/ok: C: SystemExit"]
    assert not any("secret-value" in line for line in failures)


def test_build_plan_fans_fleet_secrets_out_and_lets_a_repo_override() -> None:
    mod = _load_script()
    plan = mod.build_plan(
        {"o/a": {"X": "/own", "Y": "/y"}, "o/extra": {"Z": "/z"}},
        fleet={"X": "/fleet"},
        repos=("o/a", "o/b"),
    )
    assert plan == [
        ("o/a", "X", "/own"),
        ("o/a", "Y", "/y"),
        ("o/b", "X", "/fleet"),
        ("o/extra", "Z", "/z"),
    ]


def test_build_plan_without_fleet_is_just_the_mapping() -> None:
    mod = _load_script()
    assert mod.build_plan({"o/a": {"X": "/x"}}) == [("o/a", "X", "/x")]


def test_every_github_secret_name_and_repo_key_is_acceptable() -> None:
    """Real GitHub config validates through the loader's own rules."""
    mod = _load_script()
    named = {**mod.GITHUB_MAPPING, "fleet": mod.FLEET_SECRETS}
    assert mod.invalid_secret_names(named, mod.GITHUB_FORBIDDEN_SECRET_PREFIX) == []
    assert len(set(mod.WORKFLOW_REPOS)) == len(mod.WORKFLOW_REPOS)
    for key in (*mod.WORKFLOW_REPOS, *mod.GITHUB_MAPPING):
        owner, sep, repo = key.partition("/")
        assert sep and owner and repo and "/" not in repo, key


def test_github_forbids_only_its_own_reserved_prefixes() -> None:
    """FORGEJO_ is a legal GitHub name, GITHUB_ is not."""
    mod = _load_script()
    rule = mod.GITHUB_FORBIDDEN_SECRET_PREFIX
    assert mod.invalid_secret_names({"o/r": {"FORGEJO_X": "/p"}}, rule) == []
    assert mod.invalid_secret_names({"o/r": {"GITHUB_X": "/p"}}, rule) == ["o/r: GITHUB_X"]


def test_gh_set_secret_pipes_the_value_on_stdin_never_argv(monkeypatch) -> None:
    mod = _load_script()
    seen: dict[str, object] = {}

    def _fake_run(argv, **kwargs):
        seen["argv"] = argv
        seen["input"] = kwargs.get("input")
        return None

    monkeypatch.setattr(mod.subprocess, "run", _fake_run)
    mod.gh_set_secret("", "o/r", "NAME", "the-secret-value")

    assert seen["argv"] == ["gh", "secret", "set", "NAME", "--repo", "o/r"]
    assert "the-secret-value" not in " ".join(seen["argv"])
    assert seen["input"] == "the-secret-value"


def test_check_plan_reports_missing_unreadable_and_ok_by_name() -> None:
    mod = _load_script()

    def _lister(repo_slug):
        if repo_slug == "o/gone":
            raise mod.subprocess.CalledProcessError(1, "gh")
        return {"o/ok": {"A", "B"}, "o/partial": {"A"}}[repo_slug]

    plan = [
        ("o/ok", "A", "/p"),
        ("o/ok", "B", "/p"),
        ("o/partial", "A", "/p"),
        ("o/partial", "B", "/p"),
        ("o/gone", "A", "/p"),
    ]
    lines, problems = mod.check_plan(plan, lister=_lister)

    assert lines == [
        "o/ok: ok (2)",
        "o/partial: missing B",
        "o/gone: unreadable (CalledProcessError)",
    ]
    assert problems == 2


def test_github_dry_run_plans_every_repo_and_writes_nothing(monkeypatch, capsys) -> None:
    mod = _load_script()
    monkeypatch.setattr(mod, "WORKFLOW_REPOS", ("o/a", "o/b"))
    monkeypatch.setattr(mod, "FLEET_SECRETS", {"F": "/fleet"})
    monkeypatch.setattr(mod, "GITHUB_MAPPING", {"o/a": {"G": "/g"}})

    def _no_write(*_a, **_k):
        raise AssertionError("dry run must not write or read SSM")

    monkeypatch.setattr(mod, "apply_plan", _no_write)
    assert mod.main(["--dry-run", "--target", "github"]) == 0

    out = capsys.readouterr().out.splitlines()
    assert out == [
        "o/a: F <- ssm /fleet",
        "o/a: G <- ssm /g",
        "o/b: F <- ssm /fleet",
        "3 secrets across 2 repos on GitHub",
    ]


def test_check_is_refused_on_the_forgejo_target() -> None:
    mod = _load_script()
    with pytest.raises(SystemExit, match="--target github"):
        mod.main(["--check"])


def test_repo_filter_plans_a_repo_outside_the_workflow_set(monkeypatch, capsys) -> None:
    """A scratch repo gets the fleet secrets and its own entries, and nothing else."""
    mod = _load_script()
    monkeypatch.setattr(mod, "WORKFLOW_REPOS", ("o/a",))
    monkeypatch.setattr(mod, "FLEET_SECRETS", {"F": "/fleet"})
    monkeypatch.setattr(mod, "GITHUB_MAPPING", {"o/a": {"G": "/g"}, "o/scratch": {"S": "/s"}})

    assert mod.main(["--dry-run", "--target", "github", "--repo", "o/scratch"]) == 0

    assert capsys.readouterr().out.splitlines() == [
        "o/scratch: F <- ssm /fleet",
        "o/scratch: S <- ssm /s",
        "2 secrets across 1 repos on GitHub",
    ]


def test_repo_filter_narrows_the_forgejo_mapping(monkeypatch, capsys) -> None:
    mod = _load_script()
    monkeypatch.setattr(mod, "MAPPING", {"o/a": {"A": "/a"}, "o/b": {"B": "/b"}})

    assert mod.main(["--dry-run", "--repo", "o/b"]) == 0

    assert capsys.readouterr().out.splitlines() == [
        "o/b: B <- ssm /b",
        "1 secrets across 1 repos on Forgejo",
    ]


@pytest.mark.parametrize("value", ["noslash", "a/b/c", "/repo", "owner/"])
def test_repo_filter_rejects_a_malformed_slug(value) -> None:
    mod = _load_script()
    with pytest.raises(SystemExit, match="owner/repo"):
        mod.main(["--dry-run", "--repo", value])
