"""Tests for the fixed structural Goose context baseline."""

from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml

from agentic_os import context_budget_goose as goose


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def component_rows(snapshot: dict[str, object]) -> list[dict[str, object]]:
    return list(goose._snapshot_component_rows(snapshot))


def provider_fixture(root: Path, *, harness: str = "goose") -> Path:
    provider = root / "provider"
    write(
        provider / "aos" / "role-harnesses.json",
        json.dumps(
            {
                "roles": [
                    {
                        "role": "ops",
                        "intents": [
                            {
                                "intent": "operational-decision",
                                "harness": harness,
                            }
                        ],
                    }
                ]
            }
        ),
    )
    write(
        provider / ".agents" / "roles.kdl",
        "roles {\n"
        "    role ops {\n"
        "        composed-skill tooling-ops-live-remediation\n"
        "        intent operational-decision {\n"
        f"            harness {harness}\n"
        "        }\n"
        "    }\n"
        "}\n",
    )
    write(
        provider / ".agents" / "skills" / "alpha" / "SKILL.md",
        "---\nname: alpha\ndescription: Alpha capability.\n---\n# Alpha body\n",
    )
    write(
        provider
        / ".agents"
        / "composed"
        / "tooling-ops-live-remediation"
        / "COMPOSED.md",
        "---\n"
        "name: tooling-ops-live-remediation\n"
        "description: Bounded remediation.\n"
        "---\n"
        "# Remediate\n",
    )
    return provider


def bundle_fixture(root: Path) -> Path:
    bundle = root / "bundle"
    write(
        bundle / "manifest.json",
        json.dumps(
            {
                "format": "agent-compose.bundle",
                "role": "ops",
                "personalities": ["protective", "grounded", "reflective"],
                "density": "full",
                "sources": ["person:kai", "aos-public"],
                "delivery": {
                    "mode": "native-skills",
                    "instructions": "content/instructions.md",
                    "skills_root": "content/skills",
                },
            }
        ),
    )
    write(bundle / "content" / "instructions.md", "# Role instructions\nOps briefing.\n")
    write(
        bundle / "content" / "skills" / "aos-public" / "alpha" / "SKILL.md",
        "---\nname: alpha\ndescription: Alpha capability.\n---\n# Alpha body\n",
    )
    write(
        bundle
        / "content"
        / "skills"
        / "aos-public"
        / "tooling-ops-live-remediation"
        / "SKILL.md",
        "---\n"
        "name: tooling-ops-live-remediation\n"
        "description: Bounded remediation.\n"
        "---\n"
        "# Remediate\n",
    )
    write(
        bundle / "content" / "skills" / "aos-public" / "alpha" / "references" / "detail.md",
        "Lazy detail.\n",
    )
    return bundle


def repo_fixture(root: Path) -> tuple[Path, Path]:
    repo = root / "repo"
    cwd = repo / "nested" / "deeper"
    cwd.mkdir(parents=True)
    write(repo / "AGENTS.md", "# Root instructions\n")
    write(repo / "nested" / "AGENTS.md", "# Nested instructions\n")
    return repo, cwd


def test_validate_goose_route_requires_fixed_lane(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    goose.validate_goose_route(provider / "aos" / "role-harnesses.json")

    wrong = provider_fixture(tmp_path / "wrong", harness="codex")
    with pytest.raises(RuntimeError, match="must select goose"):
        goose.validate_goose_route(wrong / "aos" / "role-harnesses.json")


def test_agents_inventory_rejects_outside_cwd(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, _ = repo_fixture(tmp_path)
    with pytest.raises(RuntimeError, match="outside repository root"):
        goose.build_snapshot(bundle, provider, repo, tmp_path)


def test_agents_inventory_preserves_global_and_repo_delivery_paths(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    cwd = provider / "nested"
    cwd.mkdir()
    write(provider / "AGENTS.md", "# Shared global and repository base\n")
    write(cwd / "AGENTS.md", "# Nested repository context\n")
    monkeypatch.setattr(
        goose,
        "repository_identity",
        lambda _repo: "coilyco-flight-deck/agentic-os",
    )

    snapshot = goose.build_snapshot(bundle, provider, provider, cwd)
    components = [
        item for item in component_rows(snapshot) if item["kind"] == "agents-cascade"
    ]

    assert [item["delivery"] for item in components] == [
        "global-composed",
        "repo-cascade",
        "repo-cascade",
    ]
    assert [item["source"] for item in components[:2]] == [
        "coilyco-flight-deck/agentic-os:AGENTS.md",
        "coilyco-flight-deck/agentic-os:AGENTS.md",
    ]


def test_repository_identity_uses_remote_without_host(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    class Result:
        stdout = "https://example.invalid/owner/repository.git\n"

    monkeypatch.setattr(goose.subprocess, "run", lambda *args, **kwargs: Result())
    assert goose.repository_identity(tmp_path) == "owner/repository"


def test_build_snapshot_separates_eager_and_lazy_components(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    plugin = tmp_path / "plugin-skills"
    write(
        plugin / "plugin-tool" / "SKILL.md",
        "---\nname: plugin-tool\ndescription: Plugin capability.\n---\nPlugin body.\n",
    )

    first = goose.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        plugin_roots=[plugin],
        mcp_servers=["github", "forgejo"],
    )
    second = goose.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        plugin_roots=[plugin],
        mcp_servers=["forgejo", "github"],
    )

    assert first == second
    assert first["lane"] == {
        "harness": "goose",
        "role": "ops",
        "intent": "operational-decision",
    }
    assert first["cwd"] == "nested/deeper"
    assert first["mcp"] == {
        "delivery": "deferred",
        "server_count": 2,
        "eager_schema_count": 0,
    }
    groups = first["components"]
    assert isinstance(groups, dict)
    assert list(groups) == ["eager", "lazy"]
    for kinds in groups.values():
        assert isinstance(kinds, dict)
        assert list(kinds) == sorted(kinds)
        assert all(
            not str(component["id"]).startswith("skill:")
            for components in kinds.values()
            for component in components
        )
    skills = first["skills"]
    assert isinstance(skills, dict)
    assert list(skills) == [
        "aos-public/alpha",
        "aos-public/tooling-ops-live-remediation",
        "skill-root-0/plugin-tool",
    ]
    alpha = skills["aos-public/alpha"]
    assert set(alpha) == {
        "class",
        "eager",
        "lazy",
        "resources",
    }
    assert alpha["class"] == "ordinary"
    assert alpha["eager"] > 0
    assert alpha["lazy"] > 0
    assert alpha["resources"] == 1
    assert skills["aos-public/tooling-ops-live-remediation"]["class"] == "role-composed"
    assert skills["skill-root-0/plugin-tool"]["class"] == "plugin"
    components = {item["id"]: item for item in component_rows(first)}
    assert components["instructions:goose"]["delivery"] == ".config/goose/.goosehints"
    assert components["agents:000:AGENTS.md"]["eager"] is True
    assert components["agents:001:nested/AGENTS.md"]["eager"] is True
    assert components["mcp:deferred"]["tokens"] == 0
    assert first["breakdown"]["eager"]["ordinary-skill-frontmatter"]["tokens"] > 0
    assert first["breakdown"]["lazy"]["ordinary-skill-resource"]["tokens"] > 0
    assert first["totals"]["eager"]["tokens"] > 0
    assert first["totals"]["lazy"]["tokens"] > 0


def test_snapshot_rejects_wrong_bundle_role(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    manifest = json.loads((bundle / "manifest.json").read_text(encoding="utf-8"))
    manifest["role"] = "engineer"
    write(bundle / "manifest.json", json.dumps(manifest))

    with pytest.raises(RuntimeError, match="expected role ops"):
        goose.build_snapshot(bundle, provider, repo, cwd)


def test_snapshot_round_trip_and_delta(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    before = goose.build_snapshot(bundle, provider, repo, cwd)
    snapshot_path = tmp_path / "before.yaml"
    goose.write_snapshot(snapshot_path, before)
    serialized_text = snapshot_path.read_text(encoding="utf-8")
    serialized = yaml.safe_load(serialized_text)
    assert list(serialized["components"]) == ["eager", "lazy"]
    assert "skills" in serialized
    assert "aos-public/alpha: {class: ordinary, eager:" in serialized_text
    assert len(serialized_text.splitlines()) < 100

    write(repo / "nested" / "AGENTS.md", "# Nested instructions\n" + "More context.\n" * 5)
    after = goose.build_snapshot(bundle, provider, repo, cwd)
    rendered = goose.render_delta(goose.load_snapshot(snapshot_path), after)

    assert "Goose context delta" in rendered
    assert "~ agents:001:nested/AGENTS.md" in rendered
    assert before["payload_hash"] != after["payload_hash"]
    assert int(after["totals"]["eager"]["tokens"]) > int(
        before["totals"]["eager"]["tokens"]
    )


def test_load_snapshot_accepts_legacy_flat_json(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    current = goose.build_snapshot(bundle, provider, repo, cwd)
    legacy = {
        **current,
        "format": goose.LEGACY_FORMAT,
        "components": component_rows(current),
    }
    legacy.pop("skills")
    path = tmp_path / "before.json"
    write(path, json.dumps(legacy))

    loaded = goose.load_snapshot(path)

    assert loaded["format"] == goose.LEGACY_FORMAT
    assert "Goose context baseline" in goose.render_snapshot(loaded)


def test_plugin_skill_collision_fails_closed(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    plugin = tmp_path / "plugin-skills"
    write(
        plugin / "alpha" / "SKILL.md",
        "---\nname: alpha\ndescription: Different alpha.\n---\n",
    )

    with pytest.raises(RuntimeError, match="delivered more than once"):
        goose.build_snapshot(bundle, provider, repo, cwd, plugin_roots=[plugin])
