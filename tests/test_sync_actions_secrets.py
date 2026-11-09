"""Tests for the Actions-secret source manifest loader."""

from __future__ import annotations

import importlib.util
import types
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
    """Every key must be owner/repo, since `gh secret set --repo` takes it directly."""
    mod = _load_script()
    assert mod.MAPPING
    for key in mod.MAPPING:
        owner, sep, repo = key.partition("/")
        assert sep and owner and repo and "/" not in repo, key


def test_housecast_publishes_with_a_pypi_token() -> None:
    """housecast publishes with an API token read from SSM."""
    mod = _load_script()
    housecast = mod.MAPPING[mod.slug("housecast")]
    assert housecast["PYPI_TOKEN"] == "/coilysiren/pypi/token"


@pytest.mark.parametrize(
    "name",
    [
        "GITHUB_TOKEN",
        "github_lowercase_is_also_reserved",
        "1LEADING_DIGIT",
        "HAS-A-DASH",
        "",
    ],
)
def test_invalid_secret_names_flags_rejected_names(name) -> None:
    mod = _load_script()
    assert mod.invalid_secret_names({"o/r": {name: "/p"}}) == [f"o/r: {name}"]


@pytest.mark.parametrize(
    "name", ["COILYCO_OPS_APP_ID", "SENTRY_CI_DSN", "_UNDERSCORE_LEAD", "A1"]
)
def test_invalid_secret_names_accepts_valid_names(name) -> None:
    mod = _load_script()
    assert mod.invalid_secret_names({"o/r": {name: "/p"}}) == []


def test_apply_plan_reports_every_failure_and_still_writes_the_rest() -> None:
    """A refused write on one repo must not stop the entries after it (COI-2634)."""
    mod = _load_script()
    written: list[tuple[str, str]] = []

    def _read(param):
        if param == "/empty":
            raise SystemExit("ssm parameter /empty resolved empty")
        return "secret-value"

    def _write(repo_slug, name, value):
        if repo_slug == "o/gone":
            raise mod.subprocess.CalledProcessError(1, "gh")
        written.append((repo_slug, name))

    plan = [
        ("o/gone", "A", "/p"),
        ("o/ok", "B", "/p"),
        ("o/ok", "C", "/empty"),
        ("o/ok", "D", "/p"),
    ]
    failures = mod.apply_plan(plan, read=_read, write=_write)

    assert written == [("o/ok", "B"), ("o/ok", "D")]
    assert failures == ["o/gone: A: CalledProcessError", "o/ok: C: SystemExit"]
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
    named = {**mod.MAPPING, "fleet": mod.FLEET_SECRETS}
    assert mod.invalid_secret_names(named) == []
    assert len(set(mod.WORKFLOW_REPOS)) == len(mod.WORKFLOW_REPOS)
    for key in (*mod.WORKFLOW_REPOS, *mod.MAPPING):
        owner, sep, repo = key.partition("/")
        assert sep and owner and repo and "/" not in repo, key


def test_github_forbids_only_its_own_reserved_prefixes() -> None:
    """Only GITHUB_ and a leading digit are refused."""
    mod = _load_script()
    assert mod.invalid_secret_names({"o/r": {"GITEA_X": "/p"}}) == []
    assert mod.invalid_secret_names({"o/r": {"GITHUB_X": "/p"}}) == ["o/r: GITHUB_X"]


def test_gh_set_secret_pipes_the_value_on_stdin_never_argv(monkeypatch) -> None:
    mod = _load_script()
    seen: dict[str, object] = {}

    def _fake_run(argv, **kwargs):
        seen["argv"] = argv
        seen["input"] = kwargs.get("input")
        return None

    monkeypatch.setattr(mod.subprocess, "run", _fake_run)
    mod.gh_set_secret("o/r", "NAME", "the-secret-value")

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
    monkeypatch.setattr(mod, "MAPPING", {"o/a": {"G": "/g"}})

    def _no_write(*_a, **_k):
        raise AssertionError("dry run must not write or read SSM")

    monkeypatch.setattr(mod, "apply_plan", _no_write)
    assert mod.main(["--dry-run"]) == 0

    out = capsys.readouterr().out.splitlines()
    assert out == [
        "o/a: F <- ssm /fleet",
        "o/a: G <- ssm /g",
        "o/b: F <- ssm /fleet",
        "3 secrets across 2 repos on GitHub",
    ]


def test_repo_filter_plans_a_repo_outside_the_workflow_set(monkeypatch, capsys) -> None:
    """A scratch repo gets the fleet secrets and its own entries, and nothing else."""
    mod = _load_script()
    monkeypatch.setattr(mod, "WORKFLOW_REPOS", ("o/a",))
    monkeypatch.setattr(mod, "FLEET_SECRETS", {"F": "/fleet"})
    monkeypatch.setattr(mod, "MAPPING", {"o/a": {"G": "/g"}, "o/scratch": {"S": "/s"}})

    assert mod.main(["--dry-run", "--repo", "o/scratch"]) == 0

    assert capsys.readouterr().out.splitlines() == [
        "o/scratch: F <- ssm /fleet",
        "o/scratch: S <- ssm /s",
        "2 secrets across 1 repos on GitHub",
    ]


@pytest.mark.parametrize("value", ["noslash", "a/b/c", "/repo", "owner/"])
def test_repo_filter_rejects_a_malformed_slug(value) -> None:
    mod = _load_script()
    with pytest.raises(SystemExit, match="owner/repo"):
        mod.main(["--dry-run", "--repo", value])


def test_ssm_get_restores_the_final_newline_a_private_key_needs(monkeypatch) -> None:
    mod = _load_script()
    outputs = {"/key": "-----BEGIN KEY-----\nbody\n-----END KEY-----\n", "/plain": "token\n"}

    def _fake_run(argv, **kwargs):
        return types.SimpleNamespace(stdout=outputs[argv[argv.index("--name") + 1]])

    monkeypatch.setattr(mod.subprocess, "run", _fake_run)

    assert mod.ssm_get("/key").endswith("-----END KEY-----\n")
    assert mod.ssm_get("/plain") == "token"


def test_secret_filter_narrows_the_plan_and_rejects_an_unknown_name(monkeypatch, capsys) -> None:
    mod = _load_script()
    monkeypatch.setattr(mod, "WORKFLOW_REPOS", ("o/a", "o/b"))
    monkeypatch.setattr(mod, "FLEET_SECRETS", {"F": "/fleet"})
    monkeypatch.setattr(mod, "MAPPING", {"o/a": {"G": "/g"}})

    assert mod.main(["--dry-run", "--target", "github", "--secret", "G"]) == 0
    assert capsys.readouterr().out.splitlines() == [
        "o/a: G <- ssm /g",
        "1 secrets across 1 repos on GitHub",
    ]
    with pytest.raises(SystemExit, match="matched no planned secret"):
        mod.main(["--dry-run", "--target", "github", "--secret", "NOPE"])


def test_a_removed_target_is_refused_by_the_parser() -> None:
    mod = _load_script()
    with pytest.raises(SystemExit) as refused:
        mod.main(["--dry-run", "--target", "elsewhere"])
    assert refused.value.code == 2
