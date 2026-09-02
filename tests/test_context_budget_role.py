"""Tests for harness-neutral role context snapshots."""

from __future__ import annotations

import json
import shutil
from pathlib import Path

import pytest
import yaml

from agentic_os import context_budget_role as context
from agentic_os import agent_compose_person


FIXTURE_PERSONALITIES = ("protective", "grounded", "reflective")


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def write_person_snapshot(path: Path) -> None:
    write(
        path,
        json.dumps(
            {
                "format": agent_compose_person.PERSON_SNAPSHOT_FORMAT,
                "schema_version": agent_compose_person.PERSON_SNAPSHOT_SCHEMA_VERSION,
                "source": "person:fixture",
                "person": "fixture",
                "role_order": ["sysadmin"],
                "roles": {
                    "sysadmin": {
                        "personalities": list(FIXTURE_PERSONALITIES),
                        "supported_model_tiers": ["frontier", "commodity"],
                    }
                },
                "personalities": {
                    personality: {
                        "skill": agent_compose_person.personality_skill_id(
                            personality
                        )
                    }
                    for personality in FIXTURE_PERSONALITIES
                },
            },
            indent=2,
        )
        + "\n",
    )


def component_rows(snapshot: dict[str, object]) -> list[dict[str, object]]:
    return list(context._snapshot_component_rows(snapshot))


def provider_fixture(root: Path) -> Path:
    provider = root / "provider"
    write(
        provider / ".agents" / "roles.kdl",
        "roles {\n"
        "    role sysadmin {\n"
        "        composed-skill tooling-sysadmin-live-remediation\n"
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
        / "tooling-sysadmin-live-remediation"
        / "COMPOSED.md",
        "---\n"
        "name: tooling-sysadmin-live-remediation\n"
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
                "role": "sysadmin",
                "personalities": list(FIXTURE_PERSONALITIES),
                "sources": ["roster:core", "aos"],
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
        bundle / "content" / "skills" / "aos" / "alpha" / "SKILL.md",
        "---\nname: alpha\ndescription: Alpha capability.\n---\n# Alpha body\n",
    )
    write(
        bundle
        / "content"
        / "skills"
        / "aos"
        / "tooling-sysadmin-live-remediation"
        / "SKILL.md",
        "---\n"
        "name: tooling-sysadmin-live-remediation\n"
        "description: Bounded remediation.\n"
        "---\n"
        "# Remediate\n",
    )
    for personality in FIXTURE_PERSONALITIES:
        skill_id = agent_compose_person.personality_skill_id(personality)
        write(
            bundle
            / "content"
            / "skills"
            / context.PERSON_SOURCE_SEGMENT
            / skill_id
            / "SKILL.md",
            "---\n"
            f"name: {skill_id}\n"
            f"description: Bring {personality} attention.\n"
            "---\n"
            f"# {personality}\n",
        )
    write(
        bundle
        / "content"
        / "skills"
        / context.PERSON_SOURCE_SEGMENT
        / "role-sysadmin"
        / "SKILL.md",
        "---\n"
        "name: role-sysadmin\n"
        "description: Adopt the Ops charter.\n"
        "---\n"
        "# Ops\n",
    )
    write(
        bundle
        / "content"
        / "skills"
        / "aos"
        / "alpha"
        / "references"
        / "detail.md",
        "Lazy detail.\n",
    )
    return bundle


def repo_fixture(root: Path) -> tuple[Path, Path]:
    repo = root / "repo"
    cwd = repo / "nested" / "deeper"
    cwd.mkdir(parents=True, exist_ok=True)
    write(repo / "AGENTS.md", "# Root instructions\n")
    write(repo / "nested" / "AGENTS.md", "# Nested instructions\n")
    return repo, cwd


def build_fixture_snapshot(
    root: Path,
    *,
    plugin_roots: list[Path] | None = None,
    mcp_servers: list[str] | None = None,
) -> dict[str, object]:
    provider = provider_fixture(root)
    bundle = bundle_fixture(root)
    repo, cwd = repo_fixture(root)
    return context.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
        plugin_roots=plugin_roots or [],
        mcp_servers=mcp_servers or [],
    )


def test_aos_temporary_directory_nests_functional_namespaces(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    root = tmp_path / "aos"
    monkeypatch.setattr(context, "AOS_TEMP_ROOT", root)

    with context._aos_temporary_directory("role", "context") as temp:
        assert Path(temp).parent == root / "role" / "context"


def test_validate_role_requires_generated_role(
    tmp_path: Path,
) -> None:
    person_snapshot = tmp_path / "person.json"
    write_person_snapshot(person_snapshot)

    assert context.validate_role(
        person_snapshot, "sysadmin"
    ).personalities == FIXTURE_PERSONALITIES
    assert context.validate_role(person_snapshot, "sysadmin").model_tiers == (
        "frontier",
        "commodity",
    )

    with pytest.raises(RuntimeError, match="role science is absent"):
        context.validate_role(person_snapshot, "science")
    with pytest.raises(RuntimeError, match="role must be a lowercase slug"):
        context.validate_role(person_snapshot, "QA")


def test_agents_inventory_rejects_outside_cwd(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, _ = repo_fixture(tmp_path)
    with pytest.raises(RuntimeError, match="outside repository root"):
        context.build_snapshot(
            bundle,
            provider,
            repo,
            tmp_path,
            role="sysadmin",
            expected_personalities=FIXTURE_PERSONALITIES,
        )


def test_agents_inventory_measures_harness_neutral_repo_cascade(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    cwd = provider / "nested"
    cwd.mkdir()
    write(provider / "AGENTS.md", "# Shared global and repository base\n")
    write(cwd / "AGENTS.md", "# Nested repository context\n")
    monkeypatch.setattr(
        context,
        "repository_identity",
        lambda _repo: "coilyco-flight-deck/agentic-os",
    )

    snapshot = context.build_snapshot(
        bundle,
        provider,
        provider,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
    )
    components = [
        item for item in component_rows(snapshot) if item["kind"] == "agents-cascade"
    ]

    assert [item["delivery"] for item in components] == [
        "repo-cascade",
        "repo-cascade",
    ]
    assert [item["source"] for item in components] == [
        "coilyco-flight-deck/agentic-os:AGENTS.md",
        "coilyco-flight-deck/agentic-os:nested/AGENTS.md",
    ]


def test_repository_identity_uses_remote_without_host(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class Result:
        stdout = "https://example.invalid/owner/repository.git\n"

    monkeypatch.setattr(context.subprocess, "run", lambda *args, **kwargs: Result())
    assert context.repository_identity(tmp_path) == "owner/repository"


def test_build_snapshot_separates_eager_and_lazy_components(tmp_path: Path) -> None:
    plugin = tmp_path / "plugin-skills"
    write(
        plugin / "plugin-tool" / "SKILL.md",
        "---\nname: plugin-tool\ndescription: Plugin capability.\n---\nPlugin body.\n",
    )

    first = build_fixture_snapshot(
        tmp_path,
        plugin_roots=[plugin],
        mcp_servers=["github", "forgejo"],
    )
    second = build_fixture_snapshot(
        tmp_path,
        plugin_roots=[plugin],
        mcp_servers=["forgejo", "github"],
    )

    assert first == second
    assert first["subject"] == {"role": "sysadmin"}
    assert "model_class" not in first["bundle"]
    assert first["bundle"]["personalities"] == list(FIXTURE_PERSONALITIES)
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
    expected_personality_skills = [
        "roster:core/"
        + agent_compose_person.personality_skill_id(personality)
        for personality in FIXTURE_PERSONALITIES
    ]
    assert list(skills) == sorted(
        [
            "aos/alpha",
            "aos/tooling-sysadmin-live-remediation",
            "roster:core/role-sysadmin",
            *expected_personality_skills,
            "skill-root-0/plugin-tool",
        ]
    )
    alpha = skills["aos/alpha"]
    assert set(alpha) == {"class", "eager", "lazy", "resources"}
    assert alpha["class"] == "ordinary"
    assert alpha["eager"] > 0
    assert alpha["lazy"] > 0
    assert alpha["resources"] == 1
    assert skills["aos/tooling-sysadmin-live-remediation"]["class"] == "role-composed"
    assert skills["roster:core/role-sysadmin"]["class"] == "role"
    assert all(
        skills[skill_id]["class"] == "personality"
        for skill_id in expected_personality_skills
    )
    assert skills["skill-root-0/plugin-tool"]["class"] == "plugin"
    components = {item["id"]: item for item in component_rows(first)}
    assert components["instructions:role"]["delivery"] == "role-instructions"
    assert components["agents:000:AGENTS.md"]["eager"] is True
    assert components["agents:001:nested/AGENTS.md"]["eager"] is True
    assert components["mcp:deferred"]["tokens"] == 0
    assert first["breakdown"]["eager"]["ordinary-skill-frontmatter"]["tokens"] > 0
    assert first["breakdown"]["lazy"]["ordinary-skill-resource"]["tokens"] > 0
    assert first["totals"]["eager"]["tokens"] > 0
    assert first["totals"]["lazy"]["tokens"] > 0


def test_build_snapshot_attributes_additional_provider_skills(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    private_provider = tmp_path / "private-provider"
    write(
        private_provider / ".agents" / "skills" / "private-routing" / "SKILL.md",
        "---\n"
        "name: private-routing\n"
        "description: Private routing metadata.\n"
        "---\n"
        "# Private routing\n",
    )
    write(
        private_provider
        / ".agents"
        / "composed"
        / "private-role-method"
        / "COMPOSED.md",
        "---\n"
        "name: private-role-method\n"
        "description: Private role method.\n"
        "---\n"
        "# Private role method\n",
    )
    bundle = bundle_fixture(tmp_path)
    write(
        bundle / "content" / "skills" / "aosk" / "private-routing" / "SKILL.md",
        "---\n"
        "name: private-routing\n"
        "description: Private routing metadata.\n"
        "---\n"
        "# Private routing\n",
    )
    write(
        bundle
        / "content"
        / "skills"
        / "aosk"
        / "private-role-method"
        / "SKILL.md",
        "---\n"
        "name: private-role-method\n"
        "description: Private role method.\n"
        "---\n"
        "# Private role method\n",
    )
    manifest_path = bundle / "manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    manifest["sources"].append("aosk")
    write(manifest_path, json.dumps(manifest))
    repo, cwd = repo_fixture(tmp_path)

    snapshot = context.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
        additional_providers={"aosk": private_provider},
    )

    assert snapshot["bundle"]["sources"] == ["roster:core", "aos", "aosk"]
    assert list(snapshot["providers"]) == ["aos", "aosk"]
    assert snapshot["skills"]["aosk/private-routing"]["class"] == "ordinary"
    assert (
        snapshot["skills"]["aosk/private-role-method"]["class"]
        == "role-composed"
    )


def test_snapshot_rejects_wrong_bundle_role(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    manifest = json.loads((bundle / "manifest.json").read_text(encoding="utf-8"))
    manifest["role"] = "platform"
    write(bundle / "manifest.json", json.dumps(manifest))

    with pytest.raises(RuntimeError, match="expected role sysadmin"):
        context.build_snapshot(
            bundle,
            provider,
            repo,
            cwd,
            role="sysadmin",
            expected_personalities=FIXTURE_PERSONALITIES,
        )


def test_snapshot_rejects_personality_drift_from_agent_compose(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    manifest = json.loads(
        (bundle / "manifest.json").read_text(encoding="utf-8")
    )
    manifest["personalities"] = list(reversed(FIXTURE_PERSONALITIES))
    write(bundle / "manifest.json", json.dumps(manifest))

    with pytest.raises(RuntimeError, match="personalities differ"):
        context.build_snapshot(
            bundle,
            provider,
            repo,
            cwd,
            role="sysadmin",
            expected_personalities=FIXTURE_PERSONALITIES,
        )


def test_snapshot_rejects_missing_personality_skill_body(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    missing = agent_compose_person.personality_skill_id(
        FIXTURE_PERSONALITIES[0]
    )
    shutil.rmtree(
        bundle
        / "content"
        / "skills"
        / context.PERSON_SOURCE_SEGMENT
        / missing
    )

    with pytest.raises(RuntimeError, match="personality skills differ"):
        context.build_snapshot(
            bundle,
            provider,
            repo,
            cwd,
            role="sysadmin",
            expected_personalities=FIXTURE_PERSONALITIES,
        )


def test_snapshot_accepts_additional_roster_owned_skill(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    write(
        bundle
        / "content"
        / "skills"
        / context.PERSON_SOURCE_SEGMENT
        / "eval-role-comms"
        / "SKILL.md",
        "---\n"
        "name: eval-role-comms\n"
        "description: Evaluate role communication.\n"
        "---\n"
        "# Role communication\n",
    )

    snapshot = context.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
    )

    assert snapshot["skills"]["roster:core/eval-role-comms"]["class"] == "role"


def test_snapshot_round_trip_and_delta(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    repo, cwd = repo_fixture(tmp_path)
    before = context.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
    )
    snapshot_path = tmp_path / "before.yaml"
    context.write_snapshot(snapshot_path, before)
    serialized_text = snapshot_path.read_text(encoding="utf-8")
    serialized = yaml.safe_load(serialized_text)
    assert list(serialized["components"]) == ["eager", "lazy"]
    assert "skills" in serialized
    assert "aos/alpha: {class: ordinary, eager:" in serialized_text
    assert len(serialized_text.splitlines()) < 100

    write(repo / "nested" / "AGENTS.md", "# Nested instructions\n" + "More context.\n" * 5)
    after = context.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
    )
    rendered = context.render_delta(context.load_snapshot(snapshot_path), after)

    assert "Role context delta" in rendered
    assert "~ agents:001:nested/AGENTS.md" in rendered
    assert before["payload_hash"] != after["payload_hash"]
    assert int(after["totals"]["eager"]["tokens"]) > int(
        before["totals"]["eager"]["tokens"]
    )


def test_delta_rejects_different_role(tmp_path: Path) -> None:
    before = build_fixture_snapshot(tmp_path / "before")
    after = build_fixture_snapshot(tmp_path / "after")
    after["subject"] = {"role": "science"}
    after["repository"] = before["repository"]
    after["cwd"] = before["cwd"]

    with pytest.raises(RuntimeError, match="different subject"):
        context.render_delta(before, after)


def test_capture_requires_only_agent_compose_executable(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    provider = provider_fixture(tmp_path)
    source_bundle = bundle_fixture(tmp_path / "source")
    repo, cwd = repo_fixture(tmp_path)
    executable_lookups: list[str] = []
    operations: list[str] = []

    def fake_which(name: str) -> str:
        executable_lookups.append(name)
        return "agent-compose-test"

    class Result:
        stdout = "ok\n"
        stderr = ""

    def fake_run(command: list[str], **_kwargs: object) -> Result:
        if command[0] == "git":
            return Result()
        operation = command[1]
        operations.append(operation)
        if operation == "roster":
            roster = Path(command[command.index("--out") + 1])
            write(
                roster / "AGENTS.COMPOSE.md",
                "- If you are codex running the sysadmin role: your name is solar SRE.\n",
            )
            write_person_snapshot(roster / "person.json")
        elif operation == "compose":
            output = Path(command[command.index("--out") + 1])
            shutil.copytree(source_bundle, output / "bundle")
        return Result()

    monkeypatch.setattr(context.shutil, "which", fake_which)
    monkeypatch.setattr(context.subprocess, "run", fake_run)

    snapshot = context.capture_snapshot(
        provider,
        repo,
        cwd,
        role="sysadmin",
        agent_compose="agent-compose",
        mcporter_path=tmp_path / "missing-mcporter.json",
    )

    assert snapshot["subject"] == {"role": "sysadmin"}
    assert executable_lookups == ["agent-compose"]
    assert operations == ["roster", "compose"]


def test_request_uses_role_compatibility_tier_without_model_class() -> None:
    request = context._request_text("provider", "sysadmin", "frontier")
    assert 'model-tier "frontier"' in request
    assert "model-class" not in request


def test_request_includes_named_additional_providers() -> None:
    request = context._request_text(
        "providers/aos",
        "sysadmin",
        "frontier",
        {"aosk": "providers/aosk"},
    )

    assert 'source "aos" root="providers/aos" required=#true' in request
    assert 'source "aosk" root="providers/aosk" required=#true' in request


def test_parse_additional_provider_fails_closed() -> None:
    assert context.parse_additional_provider("aosk=/tmp/aosk") == (
        "aosk",
        Path("/tmp/aosk"),
    )
    with pytest.raises(RuntimeError, match="ID=PATH"):
        context.parse_additional_provider("aosk")
    with pytest.raises(RuntimeError, match="reserved"):
        context.parse_additional_provider("aos=/tmp/aos")


def test_plugin_skill_collision_fails_closed(tmp_path: Path) -> None:
    plugin = tmp_path / "plugin-skills"
    write(
        plugin / "alpha" / "SKILL.md",
        "---\nname: alpha\ndescription: Different alpha.\n---\n",
    )

    with pytest.raises(RuntimeError, match="delivered more than once"):
        build_fixture_snapshot(tmp_path, plugin_roots=[plugin])


def test_build_snapshot_measures_the_composed_operating_base(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    write(provider / "AGENTS.md", "# Public operating base\n" + "base line\n" * 40)
    private_provider = tmp_path / "private-provider"
    write(private_provider / "AGENTS.md", "# Private operating context\n")
    write(
        private_provider / ".agents" / "skills" / "private-routing" / "SKILL.md",
        "---\nname: private-routing\ndescription: Private routing.\n---\n# Private routing\n",
    )
    bundle = bundle_fixture(tmp_path)
    manifest_path = bundle / "manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    manifest["sources"].append("aosk")
    write(manifest_path, json.dumps(manifest))
    repo, cwd = repo_fixture(tmp_path)

    snapshot = context.build_snapshot(
        bundle,
        provider,
        repo,
        cwd,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
        additional_providers={"aosk": private_provider},
    )

    rows = [
        item for item in component_rows(snapshot) if item["kind"] == "operating-base"
    ]
    assert [item["delivery"] for item in rows] == ["composed-global", "composed-global"]
    assert all(item["eager"] for item in rows)
    assert sum(int(item["bytes"]) for item in rows) > 400


def test_operating_base_does_not_double_count_the_active_repository(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    provider = provider_fixture(tmp_path)
    write(provider / "AGENTS.md", "# Shared operating base\n")
    bundle = bundle_fixture(tmp_path)
    monkeypatch.setattr(
        context,
        "repository_identity",
        lambda _repo: "coilyco-flight-deck/agentic-os",
    )

    snapshot = context.build_snapshot(
        bundle,
        provider,
        provider,
        provider,
        role="sysadmin",
        expected_personalities=FIXTURE_PERSONALITIES,
    )

    kinds = [item["kind"] for item in component_rows(snapshot)]
    assert "agents-cascade" in kinds
    assert "operating-base" not in kinds
