"""Tests for scripts/dep-bump.py (the dev-base pin auto-bump)."""
from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "dep-bump.py"

DOCKERFILE_SNIPPET = """\
FROM ubuntu:24.04

ARG UV_VERSION=0.11.21
ARG NODE_VERSION=22.22.3
ARG GO_VERSION=1.26.4
ARG MCPORTER_VERSION=0.12.2

ARG TARGETARCH

RUN curl "https://astral.sh/uv/${UV_VERSION}/install.sh"
"""


def _load_script():
    spec = importlib.util.spec_from_file_location("dep_bump", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_parse_args_block_collects_only_arg_defaults() -> None:
    script = _load_script()
    pins = script.parse_args_block(DOCKERFILE_SNIPPET)
    assert pins == {
        "UV_VERSION": "0.11.21",
        "NODE_VERSION": "22.22.3",
        "GO_VERSION": "1.26.4",
        "MCPORTER_VERSION": "0.12.2",
    }
    # `ARG TARGETARCH` has no default, so it must not appear at all.
    assert "TARGETARCH" not in pins


def test_set_arg_rewrites_only_the_named_default() -> None:
    script = _load_script()
    out = script.set_arg(DOCKERFILE_SNIPPET, "UV_VERSION", "0.12.0")
    assert "ARG UV_VERSION=0.12.0\n" in out
    # Other pins and the ${UV_VERSION} usage downstream are untouched.
    assert "ARG NODE_VERSION=22.22.3\n" in out
    assert "${UV_VERSION}" in out


def test_set_arg_rejects_unknown_arg() -> None:
    script = _load_script()
    with pytest.raises(SystemExit):
        script.set_arg(DOCKERFILE_SNIPPET, "NOPE_VERSION", "1.0.0")


def test_set_arg_in_tree_rewrites_each_dockerfile_with_arg(tmp_path: Path) -> None:
    script = _load_script()
    (tmp_path / "core").mkdir()
    (tmp_path / "lang-go").mkdir()
    (tmp_path / "lang-node").mkdir()
    (tmp_path / "core" / "Dockerfile").write_text("ARG GO_VERSION=1.26.4\n", encoding="utf-8")
    (tmp_path / "lang-go" / "Dockerfile").write_text(
        "ARG GO_VERSION=1.26.4\n", encoding="utf-8"
    )
    (tmp_path / "lang-node" / "Dockerfile").write_text(
        "ARG CLAUDE_VERSION=2.1.200\n", encoding="utf-8"
    )

    script.set_arg_in_tree(tmp_path, "GO_VERSION", "1.26.5")

    assert "ARG GO_VERSION=1.26.5\n" in (tmp_path / "core" / "Dockerfile").read_text()
    assert "ARG GO_VERSION=1.26.5\n" in (tmp_path / "lang-go" / "Dockerfile").read_text()
    assert "ARG CLAUDE_VERSION=2.1.200\n" in (
        tmp_path / "lang-node" / "Dockerfile"
    ).read_text()


def test_parse_semver_is_numeric_and_drops_tails() -> None:
    script = _load_script()
    assert script.parse_semver("0.11.21") == (0, 11, 21)
    assert script.parse_semver("2.35.5") == (2, 35, 5)
    assert script.parse_semver("1.38") == (1, 38, 0)
    # A prerelease tail must not throw or outrank its release.
    assert script.parse_semver("0.143.0-alpha.16") == (0, 143, 0)


def test_pick_highest_orders_numerically_not_lexically() -> None:
    script = _load_script()
    assert script.pick_highest(["0.11.9", "0.11.21", "0.11.10"]) == "0.11.21"
    assert script.pick_highest(["2.9.0", "2.35.11", "2.10.0"]) == "2.35.11"
    assert script.pick_highest([]) is None


def test_compute_plan_reports_only_drifted_pins(monkeypatch) -> None:
    script = _load_script()
    # Stub every resolver: uv drifts, node matches its current, go raises.
    monkeypatch.setitem(script.RESOLVERS, "UV_VERSION", lambda: "0.12.0")
    monkeypatch.setitem(script.RESOLVERS, "NODE_VERSION", lambda current: current)
    monkeypatch.setitem(
        script.RESOLVERS,
        "GO_VERSION",
        lambda: (_ for _ in ()).throw(RuntimeError("upstream down")),
    )
    # Restrict resolution to the pins present in the snippet.
    for name in list(script.RESOLVERS):
        if name not in {"UV_VERSION", "NODE_VERSION", "GO_VERSION"}:
            monkeypatch.delitem(script.RESOLVERS, name)

    plan = script.compute_plan(DOCKERFILE_SNIPPET)

    # Only uv drifted; node was current; go's raise was swallowed (fail-soft).
    assert plan == [{"arg": "UV_VERSION", "current": "0.11.21", "latest": "0.12.0"}]


def test_version_sensitive_pins_are_excluded_from_auto_bump() -> None:
    # golangci-lint / kdlfmt pin to consumer CI versions, not upstream latest, so
    # they get no resolver; trufflehog tracks latest, so it gets one (agentic-os#292).
    script = _load_script()
    assert "GOLANGCI_LINT_VERSION" not in script.RESOLVERS
    assert "KDLFMT_VERSION" not in script.RESOLVERS
    assert "TRUFFLEHOG_VERSION" in script.RESOLVERS
    assert "GH_VERSION" in script.RESOLVERS
    assert "HELM_VERSION" in script.RESOLVERS
    assert "KUBECTL_VERSION" in script.RESOLVERS
    assert "YQ_VERSION" in script.RESOLVERS


def test_mcporter_is_auto_bumped_from_npm_registry() -> None:
    # MCPorter is installed from npm, so the auto-bump tracks the npm registry's
    # latest published version like Claude does.
    script = _load_script()
    assert "MCPORTER_VERSION" in script.RESOLVERS


def test_mcporter_resolver_reads_npm_latest(monkeypatch) -> None:
    # The resolver should read the npm registry's latest metadata and return its
    # published version directly.
    script = _load_script()
    seen: dict[str, str] = {}

    def fake_get_json(url: str) -> object:
        seen["url"] = url
        return {"version": "0.12.3"}

    monkeypatch.setattr(script, "_get_json", fake_get_json)
    assert script._resolve_mcporter() == "0.12.3"
    assert seen["url"] == "https://registry.npmjs.org/mcporter/latest"


def test_gh_resolver_reads_the_latest_release(monkeypatch) -> None:
    script = _load_script()
    seen: dict[str, str] = {}

    def fake_get_json(url: str) -> object:
        seen["url"] = url
        return [{"draft": False, "prerelease": False, "tag_name": "v2.96.0"}]

    monkeypatch.setattr(script, "_get_json", fake_get_json)
    assert script._resolve_gh() == "2.96.0"
    assert seen["url"] == "https://api.github.com/repos/cli/cli/releases?per_page=100"


def test_helm_resolver_reads_the_latest_release(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setattr(
        script,
        "_get_json",
        lambda url: [{"draft": False, "prerelease": False, "tag_name": "v4.2.2"}],
    )
    assert script._resolve_helm() == "4.2.2"


def test_kubectl_resolver_reads_the_latest_release(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setattr(
        script,
        "_get_json",
        lambda url: [{"draft": False, "prerelease": False, "tag_name": "v1.36.2"}],
    )
    assert script._resolve_kubectl() == "1.36.2"


def test_yq_resolver_reads_the_latest_release(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setattr(
        script,
        "_get_json",
        lambda url: [{"draft": False, "prerelease": False, "tag_name": "v4.53.3"}],
    )
    assert script._resolve_yq() == "4.53.3"


def test_ward_binary_is_manual_until_aos_validates_prod_channel() -> None:
    # Raw ward releases are staging until release assets are executable-smoked and
    # aos promotes a candidate as prod/N-1, so dep-bump must not chase latest.
    script = _load_script()
    assert "WARD_VERSION" not in script.RESOLVERS


def test_dotnet_is_auto_bumped_and_stays_on_its_channel() -> None:
    # Like NODE_VERSION, the .NET SDK carries a resolver AND needs the current pin
    # to hold its major (eco-app's mods build against .NET 10) - agentic-os#329.
    script = _load_script()
    assert "DOTNET_VERSION" in script.RESOLVERS
    assert "DOTNET_VERSION" in script._NEEDS_CURRENT


def test_dotnet_resolver_reads_latest_sdk_for_the_pinned_channel(monkeypatch) -> None:
    # The resolver derives the channel from the current pin's major and returns the
    # channel metadata's `latest-sdk`, staying on 10.x for a 10.x pin.
    script = _load_script()
    seen: dict[str, str] = {}

    def fake_get_json(url: str) -> object:
        seen["url"] = url
        return {"channel-version": "10.0", "latest-sdk": "10.0.305"}

    monkeypatch.setattr(script, "_get_json", fake_get_json)
    assert script._resolve_dotnet("10.0.301") == "10.0.305"
    # Channel is keyed off the pinned major, so a 10.x pin never reaches into 11.0.
    assert "/10.0/" in seen["url"]


def test_compute_plan_skips_unresolved_pins(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setitem(script.RESOLVERS, "UV_VERSION", lambda: None)
    for name in list(script.RESOLVERS):
        if name != "UV_VERSION":
            monkeypatch.delitem(script.RESOLVERS, name)

    assert script.compute_plan(DOCKERFILE_SNIPPET) == []


def test_tree_plan_reads_the_split_dev_base_layout(monkeypatch) -> None:
    script = _load_script()
    monkeypatch.setitem(script.RESOLVERS, "UV_VERSION", lambda: "0.12.0")
    for name in list(script.RESOLVERS):
        if name != "UV_VERSION":
            monkeypatch.delitem(script.RESOLVERS, name)

    assert script._compute_tree_plan(script.DOCKERFILE) == [
        {"arg": "UV_VERSION", "current": "0.11.26", "latest": "0.12.0"}
    ]
