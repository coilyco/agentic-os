from __future__ import annotations

import json
from pathlib import Path

import pytest

from agentic_os import agents_context_inventory as inventory


def _write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def _board(path: Path) -> None:
    _write(
        path,
        json.dumps(
            {
                "roles": [
                    {
                        "role": "engineer",
                        "intents": [
                            {"intent": "autonomous-coding", "harness": "codex"}
                        ],
                    },
                    {
                        "role": "advisor",
                        "intents": [
                            {"intent": "research", "harness": "claude"}
                        ],
                    },
                ]
            }
        ),
    )


@pytest.fixture
def context_fleet(tmp_path: Path) -> dict[str, Path]:
    projects = tmp_path / "projects"
    substrate = tmp_path / "substrate.txt"
    fleet = tmp_path / "fleet.txt"
    board = tmp_path / "board.json"
    _write(
        substrate,
        "\n".join(
            (
                "coilyco-flight-deck/agentic-os",
                "coilyco-flight-deck/substrate-helper",
            )
        )
        + "\n",
    )
    _write(
        fleet,
        "\n".join(
            (
                "coilyco-flight-deck/agentic-os public",
                "coilyco-bridge/agentic-os-hardware public",
                "coilyco-bridge/product private",
                "coilyco-bridge/missing private",
            )
        )
        + "\n",
    )
    _board(board)

    shared = "Every action sentence names the actor.\n"
    _write(
        projects / "coilyco-flight-deck" / "agentic-os" / "AGENTS.md",
        f"# Global\n\n{shared}\n## Commands\n\nUse ward.\n",
    )
    _write(
        projects / "coilyco-flight-deck" / "agentic-os" / "AGENTS.codex.md",
        "# Reading files\n\nRead a focused slice.\n",
    )
    _write(
        projects / "coilyco-flight-deck" / "substrate-helper" / "AGENTS.md",
        "# Helper\n\nKeep helper-specific rules.\n",
    )
    _write(
        projects / "coilyco-bridge" / "agentic-os-hardware" / "AGENTS.md",
        "# Hardware\n\nInspect hardware facts at runtime.\n",
    )
    _write(
        projects / "coilyco-bridge" / "product" / "AGENTS.md",
        f"# Product\n\n{shared}\n## Release\n\nRun the product release workflow.\n",
    )
    _write(
        projects / "coilyco-bridge" / "product" / "CLAUDE.md",
        "@AGENTS.md\n",
    )
    _write(
        projects / "coilyco-bridge" / "product" / "service" / "AGENTS.md",
        "# Service\n\nKeep service-specific rules.\n",
    )
    return {
        "projects": projects,
        "substrate": substrate,
        "fleet": fleet,
        "board": board,
    }


def _report(context_fleet: dict[str, Path]) -> dict:
    return inventory.build_report(
        context_fleet["substrate"],
        context_fleet["fleet"],
        context_fleet["projects"],
        board=context_fleet["board"],
        current_repo="coilyco-bridge/product",
        cwd="service",
    )


def test_repository_sets_include_missing_roots_and_separate_aosh(
    context_fleet: dict[str, Path],
) -> None:
    report = _report(context_fleet)
    repos = {repo["full_name"]: repo for repo in report["repositories"]}

    assert repos["coilyco-flight-deck/agentic-os"]["kind"] == "substrate"
    assert repos["coilyco-bridge/product"]["kind"] == "product"
    assert repos["coilyco-bridge/product"]["visibility"] == "private"
    assert repos["coilyco-bridge/missing"]["present"] is False
    assert repos["coilyco-bridge/missing"]["root_agents"] == "missing"
    assert repos["coilyco-bridge/agentic-os-hardware"]["kind"] == "aosh"
    assert repos["coilyco-bridge/agentic-os-hardware"]["global_load"] is False
    assert report["aosh"]["global_load"] is False


def test_active_cascade_orders_global_override_bridge_root_and_nested(
    context_fleet: dict[str, Path],
) -> None:
    report = _report(context_fleet)
    cascades = {
        (row["role"], row["harness"]): row for row in report["active_cascades"]
    }

    codex = cascades[("engineer", "codex")]
    assert [source["delivery_path"] for source in codex["sources"]] == [
        "global-composed",
        "global-harness-override",
        "repo-cascade",
        "repo-cascade",
    ]
    assert [source["source"] for source in codex["sources"]][-2:] == [
        "coilyco-bridge/product:AGENTS.md",
        "coilyco-bridge/product:service/AGENTS.md",
    ]

    claude = cascades[("advisor", "claude")]
    assert [source["delivery_path"] for source in claude["sources"]] == [
        "global-composed",
        "repo-cascade-bridge",
        "repo-cascade",
        "repo-cascade",
    ]


def test_duplicate_product_paragraph_points_to_global_owner_without_text(
    context_fleet: dict[str, Path],
) -> None:
    report = _report(context_fleet)
    candidates = report["clipping_candidates"]
    duplicate = next(
        candidate
        for candidate in candidates
        if candidate["classification"] == "duplicate"
    )

    assert duplicate["paragraph"].startswith("coilyco-bridge/product:AGENTS.md#")
    assert duplicate["duplicate_of"].startswith(
        "coilyco-flight-deck/agentic-os:AGENTS.md#"
    )
    rendered = inventory.render_json(report)
    assert "Every action sentence names the actor." not in rendered
    assert '"visibility": "private"' in rendered


def test_classification_recommends_narrower_destination(
    context_fleet: dict[str, Path],
) -> None:
    report = _report(context_fleet)
    candidates = report["clipping_candidates"]

    assert any(
        candidate["classification"] == "task-specific"
        and candidate["destination"] == "ordinary skill"
        for candidate in candidates
    )
    product = next(
        repo
        for repo in report["repositories"]
        if repo["full_name"] == "coilyco-bridge/product"
    )
    bridge = next(
        document
        for document in product["documents"]
        if document["kind"] == "load-point-bridge"
    )
    assert bridge["paragraphs"][0]["classification"] == "generated-pointer"
    assert bridge["paragraphs"][0]["destination"] == "validator/code"


def test_report_is_stable_and_markdown_uses_flat_prose(
    context_fleet: dict[str, Path],
) -> None:
    first = _report(context_fleet)
    second = _report(context_fleet)

    assert inventory.render_json(first) == inventory.render_json(second)
    markdown = inventory.render_markdown(first)
    assert "## Active cascades" in markdown
    assert "| --- |" not in markdown
    assert "coilyco-bridge/missing" in markdown
    assert first["active_cascades"][0]["payload_hash"][:12] not in markdown


def test_build_report_rejects_current_repo_outside_inventory(
    context_fleet: dict[str, Path],
) -> None:
    with pytest.raises(
        inventory.InventoryError,
        match="current repository 'example/not-managed' is not present",
    ):
        inventory.build_report(
            context_fleet["substrate"],
            context_fleet["fleet"],
            context_fleet["projects"],
            board=context_fleet["board"],
            current_repo="example/not-managed",
        )


def test_bare_manifest_entry_resolves_only_a_named_repo(tmp_path: Path) -> None:
    projects = tmp_path / "projects"
    _write(projects / "org" / "aos" / "AGENTS.md", "# AOS\n\nRules.\n")
    _write(projects / "org" / "product" / "AGENTS.md", "# Product\n\nRules.\n")
    substrate = tmp_path / "substrate.txt"
    fleet = tmp_path / "fleet.txt"
    _write(substrate, "org/aos\n")
    _write(fleet, "product private\n")

    repos = inventory.discover_repositories(substrate, fleet, projects)

    assert {repo.full_name for repo in repos} == {"org/aos", "org/product"}
    product = next(repo for repo in repos if repo.full_name == "org/product")
    assert product.visibility == "private"


def test_ambiguous_bare_manifest_entry_fails_closed(tmp_path: Path) -> None:
    projects = tmp_path / "projects"
    (projects / "one" / "same").mkdir(parents=True)
    (projects / "two" / "same").mkdir(parents=True)
    substrate = tmp_path / "substrate.txt"
    fleet = tmp_path / "fleet.txt"
    _write(substrate, "org/aos\n")
    _write(fleet, "same private\n")

    with pytest.raises(inventory.InventoryError, match="ambiguous"):
        inventory.discover_repositories(substrate, fleet, projects)


def test_cli_check_reports_incomplete_inventory(
    context_fleet: dict[str, Path], capsys: pytest.CaptureFixture[str]
) -> None:
    rc = inventory.main(
        [
            "--fleet-manifest",
            str(context_fleet["fleet"]),
            "--substrate-manifest",
            str(context_fleet["substrate"]),
            "--projects-root",
            str(context_fleet["projects"]),
            "--board",
            str(context_fleet["board"]),
            "--current-repo",
            "coilyco-bridge/product",
            "--cwd",
            "service",
            "--format",
            "json",
            "--check",
        ]
    )

    assert rc == 1
    captured = capsys.readouterr()
    assert '"format": "agentic-os.agents-context-inventory.v1"' in captured.out
    assert "incomplete: coilyco-bridge/missing" in captured.err


def test_cli_rejects_current_repo_outside_inventory(
    context_fleet: dict[str, Path], capsys: pytest.CaptureFixture[str]
) -> None:
    rc = inventory.main(
        [
            "--fleet-manifest",
            str(context_fleet["fleet"]),
            "--substrate-manifest",
            str(context_fleet["substrate"]),
            "--projects-root",
            str(context_fleet["projects"]),
            "--board",
            str(context_fleet["board"]),
            "--current-repo",
            "example/not-managed",
        ]
    )

    assert rc == 2
    captured = capsys.readouterr()
    assert captured.out == ""
    assert (
        "current repository 'example/not-managed' is not present in the inventory"
        in captured.err
    )


def test_manifest_rejects_conflicting_visibility(tmp_path: Path) -> None:
    manifest = tmp_path / "repos.txt"
    _write(manifest, "org/repo public\norg/repo private\n")

    with pytest.raises(inventory.InventoryError, match="conflicting visibility"):
        inventory.load_manifest(manifest)
