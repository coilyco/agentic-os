"""Tests for structural role-seat context snapshots."""

from __future__ import annotations

import json
import shutil
from pathlib import Path

import pytest
import yaml

from agentic_os import context_budget_role_seat as context
from agentic_os import role_personality_sync


FIXTURE_PERSONALITIES = ("protective", "grounded", "reflective")


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def component_rows(snapshot: dict[str, object]) -> list[dict[str, object]]:
    return list(context._snapshot_component_rows(snapshot))


def provider_fixture(root: Path) -> Path:
    provider = root / "provider"
    write(
        provider / ".agents" / "roles.kdl",
        "roles {\n"
        "    role ops {\n"
        "        composed-skill tooling-ops-live-remediation\n"
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
    for personality in FIXTURE_PERSONALITIES:
        skill_id = role_personality_sync.personality_skill_id(personality)
        write(
            provider / ".agents" / "skills" / skill_id / "SKILL.md",
            "---\n"
            f"name: {skill_id}\n"
            f"description: Bring {personality} attention.\n"
            "---\n"
            f"# {personality}\n",
        )
    write(
        provider / role_personality_sync.PROJECTION_PATH,
        json.dumps(
            {
                "format": role_personality_sync.FORMAT,
                "role_count": 1,
                "personality_count": len(FIXTURE_PERSONALITIES),
                "roles": [
                    {
                        "role": "ops",
                        "personalities": list(FIXTURE_PERSONALITIES),
                    }
                ],
                "skills": [
                    {
                        "personality": personality,
                        "skill": role_personality_sync.personality_skill_id(
                            personality
                        ),
                    }
                    for personality in FIXTURE_PERSONALITIES
                ],
            }
        ),
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
                "personalities": list(FIXTURE_PERSONALITIES),
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
    for personality in FIXTURE_PERSONALITIES:
        skill_id = role_personality_sync.personality_skill_id(personality)
        write(
            bundle
            / "content"
            / "skills"
            / "aos-public"
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
        / "aos-public"
        / "alpha"
        / "references"
        / "detail.md",
        "Lazy detail.\n",
    )
    return bundle


def projection_fixture(
    root: Path,
    bundle: Path,
    *,
    seat: str = "codex",
) -> Path:
    projected = root / "projected"
    if seat == "claude":
        instructions = ".claude/CLAUDE.md"
        skills = ".claude/skills"
    else:
        instructions = f".{seat}/AGENTS.md"
        skills = ".agents/skills"
    write(
        projected / instructions,
        (bundle / "content" / "instructions.md").read_text(encoding="utf-8"),
    )
    write(projected / skills / "alpha" / "SKILL.md", "projected\n")
    write(
        projected / ".agent-compose" / "projection.json",
        json.dumps(
            {
                "layout": seat,
                "bundle": str(bundle),
                "files": [
                    instructions,
                    f"{skills}/alpha/SKILL.md",
                ],
            }
        ),
    )
    return projected


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
    seat: str = "codex",
    plugin_roots: list[Path] | None = None,
    mcp_servers: list[str] | None = None,
) -> dict[str, object]:
    provider = provider_fixture(root)
    bundle = bundle_fixture(root)
    projected = projection_fixture(root, bundle, seat=seat)
    repo, cwd = repo_fixture(root)
    return context.build_snapshot(
        bundle,
        projected,
        provider,
        repo,
        cwd,
        role="ops",
        seat=seat,
        plugin_roots=plugin_roots or [],
        mcp_servers=mcp_servers or [],
    )


def test_validate_role_seat_requires_generated_roster_pair(tmp_path: Path) -> None:
    roster = tmp_path / "AGENTS.COMPOSE.md"
    write(
        roster,
        "## ops - Operate\n\n"
        "- If you are codex running the ops role: your name is solar SRE.\n"
        "- If you are claude running the ops role: your name is fabled SRE.\n",
    )

    context.validate_role_seat(roster, "ops", "codex")

    with pytest.raises(RuntimeError, match="role ops has no goose seat"):
        context.validate_role_seat(roster, "ops", "goose")


def test_agents_inventory_rejects_outside_cwd(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    projected = projection_fixture(tmp_path, bundle)
    repo, _ = repo_fixture(tmp_path)
    with pytest.raises(RuntimeError, match="outside repository root"):
        context.build_snapshot(
            bundle,
            projected,
            provider,
            repo,
            tmp_path,
            role="ops",
            seat="codex",
        )


def test_agents_inventory_preserves_global_and_repo_delivery_paths(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    projected = projection_fixture(tmp_path, bundle)
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
        projected,
        provider,
        provider,
        cwd,
        role="ops",
        seat="codex",
    )
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
    assert first["subject"] == {"role": "ops", "seat": "codex"}
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
        "aos-public/"
        + role_personality_sync.personality_skill_id(personality)
        for personality in FIXTURE_PERSONALITIES
    ]
    assert list(skills) == sorted(
        [
            "aos-public/alpha",
            "aos-public/tooling-ops-live-remediation",
            *expected_personality_skills,
            "skill-root-0/plugin-tool",
        ]
    )
    alpha = skills["aos-public/alpha"]
    assert set(alpha) == {"class", "eager", "lazy", "resources"}
    assert alpha["class"] == "ordinary"
    assert alpha["eager"] > 0
    assert alpha["lazy"] > 0
    assert alpha["resources"] == 1
    assert skills["aos-public/tooling-ops-live-remediation"]["class"] == "role-composed"
    assert all(
        skills[skill_id]["class"] == "personality"
        for skill_id in expected_personality_skills
    )
    assert skills["skill-root-0/plugin-tool"]["class"] == "plugin"
    components = {item["id"]: item for item in component_rows(first)}
    assert components["instructions:role"]["delivery"] == ".codex/AGENTS.md"
    assert components["agents:000:AGENTS.md"]["eager"] is True
    assert components["agents:001:nested/AGENTS.md"]["eager"] is True
    assert components["mcp:deferred"]["tokens"] == 0
    assert first["breakdown"]["eager"]["ordinary-skill-frontmatter"]["tokens"] > 0
    assert first["breakdown"]["lazy"]["ordinary-skill-resource"]["tokens"] > 0
    assert first["totals"]["eager"]["tokens"] > 0
    assert first["totals"]["lazy"]["tokens"] > 0


def test_projection_changes_seat_specific_load_points(tmp_path: Path) -> None:
    snapshot = build_fixture_snapshot(tmp_path, seat="claude")
    components = {item["id"]: item for item in component_rows(snapshot)}

    assert snapshot["subject"] == {"role": "ops", "seat": "claude"}
    assert components["instructions:role"]["delivery"] == ".claude/CLAUDE.md"


def test_snapshot_rejects_wrong_bundle_role(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    projected = projection_fixture(tmp_path, bundle)
    repo, cwd = repo_fixture(tmp_path)
    manifest = json.loads((bundle / "manifest.json").read_text(encoding="utf-8"))
    manifest["role"] = "engineer"
    write(bundle / "manifest.json", json.dumps(manifest))

    with pytest.raises(RuntimeError, match="expected role ops"):
        context.build_snapshot(
            bundle,
            projected,
            provider,
            repo,
            cwd,
            role="ops",
            seat="codex",
        )


def test_snapshot_rejects_personality_drift_from_agent_compose(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    projected = projection_fixture(tmp_path, bundle)
    repo, cwd = repo_fixture(tmp_path)
    manifest = json.loads(
        (bundle / "manifest.json").read_text(encoding="utf-8")
    )
    manifest["personalities"] = list(reversed(FIXTURE_PERSONALITIES))
    write(bundle / "manifest.json", json.dumps(manifest))

    with pytest.raises(RuntimeError, match="personalities differ"):
        context.build_snapshot(
            bundle,
            projected,
            provider,
            repo,
            cwd,
            role="ops",
            seat="codex",
        )


def test_snapshot_rejects_missing_personality_skill_body(
    tmp_path: Path,
) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    projected = projection_fixture(tmp_path, bundle)
    repo, cwd = repo_fixture(tmp_path)
    missing = role_personality_sync.personality_skill_id(
        FIXTURE_PERSONALITIES[0]
    )
    shutil.rmtree(
        bundle / "content" / "skills" / "aos-public" / missing
    )

    with pytest.raises(RuntimeError, match="personality skills differ"):
        context.build_snapshot(
            bundle,
            projected,
            provider,
            repo,
            cwd,
            role="ops",
            seat="codex",
        )


def test_snapshot_round_trip_and_delta(tmp_path: Path) -> None:
    provider = provider_fixture(tmp_path)
    bundle = bundle_fixture(tmp_path)
    projected = projection_fixture(tmp_path, bundle)
    repo, cwd = repo_fixture(tmp_path)
    before = context.build_snapshot(
        bundle,
        projected,
        provider,
        repo,
        cwd,
        role="ops",
        seat="codex",
    )
    snapshot_path = tmp_path / "before.yaml"
    context.write_snapshot(snapshot_path, before)
    serialized_text = snapshot_path.read_text(encoding="utf-8")
    serialized = yaml.safe_load(serialized_text)
    assert list(serialized["components"]) == ["eager", "lazy"]
    assert "skills" in serialized
    assert "aos-public/alpha: {class: ordinary, eager:" in serialized_text
    assert len(serialized_text.splitlines()) < 100

    write(repo / "nested" / "AGENTS.md", "# Nested instructions\n" + "More context.\n" * 5)
    after = context.build_snapshot(
        bundle,
        projected,
        provider,
        repo,
        cwd,
        role="ops",
        seat="codex",
    )
    rendered = context.render_delta(context.load_snapshot(snapshot_path), after)

    assert "Role-seat context delta" in rendered
    assert "~ agents:001:nested/AGENTS.md" in rendered
    assert before["payload_hash"] != after["payload_hash"]
    assert int(after["totals"]["eager"]["tokens"]) > int(
        before["totals"]["eager"]["tokens"]
    )


def test_delta_rejects_different_role_or_seat(tmp_path: Path) -> None:
    before = build_fixture_snapshot(tmp_path / "before", seat="codex")
    after = build_fixture_snapshot(tmp_path / "after", seat="claude")
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
                "- If you are codex running the ops role: your name is solar SRE.\n",
            )
        elif operation == "compose":
            output = Path(command[command.index("--out") + 1])
            shutil.copytree(source_bundle, output / "bundle")
        elif operation == "project":
            target = Path(command[command.index("--target") + 1])
            projection_fixture(target.parent, source_bundle)
            generated = target.parent / "projected"
            if generated != target:
                shutil.copytree(generated, target)
        return Result()

    monkeypatch.setattr(context.shutil, "which", fake_which)
    monkeypatch.setattr(context.subprocess, "run", fake_run)

    snapshot = context.capture_snapshot(
        provider,
        repo,
        cwd,
        role="ops",
        seat="codex",
        agent_compose="agent-compose",
        mcporter_path=tmp_path / "missing-mcporter.json",
    )

    assert snapshot["subject"] == {"role": "ops", "seat": "codex"}
    assert executable_lookups == ["agent-compose"]
    assert operations == ["roster", "compose", "project"]


def test_plugin_skill_collision_fails_closed(tmp_path: Path) -> None:
    plugin = tmp_path / "plugin-skills"
    write(
        plugin / "alpha" / "SKILL.md",
        "---\nname: alpha\ndescription: Different alpha.\n---\n",
    )

    with pytest.raises(RuntimeError, match="delivered more than once"):
        build_fixture_snapshot(tmp_path, plugin_roots=[plugin])
