"""Capture and compare structural context for one role and seat."""

from __future__ import annotations

import hashlib
import json
import re
import shutil
import subprocess
import tempfile
import urllib.parse
from collections import defaultdict
from dataclasses import dataclass
from functools import cache
from pathlib import Path
from typing import Iterable

import yaml

from agentic_os.agents_context_inventory import (
    InventoryError,
    Scenario,
    active_cascade,
    discover_repositories,
)
from agentic_os.context_budget_tokens import TOKENIZER_NOTE, count_tokens
from agentic_os.generators.generate_agent_compose import _split_frontmatter
from agentic_os.role_personality_sync import (
    PROJECTION_PATH as ROLE_PERSONALITY_PROJECTION,
    RolePersonalitySyncError,
    load_person_snapshot,
    load_projection as load_personality_projection,
    personality_skill_id,
)

FORMAT = "agentic-os.role-seat-context.v1"
SOURCE_ID = "aos-public"
PERSON_SOURCE_ID = "person:kai"
PERSON_SOURCE_SEGMENT = urllib.parse.quote(PERSON_SOURCE_ID, safe="")
SLUG_RE = re.compile(r"^[a-z][a-z0-9-]*$")
AOS_LAYOUT_MODEL_CLASSES_PATH = (
    Path(__file__).resolve().parents[1] / "aos" / "layout-model-classes.json"
)
AOS_LAYOUT_MODEL_CLASSES_FORMAT = "agentic-os.layout-model-classes.v1"

SKILL_CLASS_KINDS = {
    "ordinary": (
        "ordinary-skill-frontmatter",
        "ordinary-skill-body",
        "ordinary-skill-resource",
    ),
    "personality": (
        "personality-frontmatter",
        "personality-body",
        "personality-resource",
    ),
    "plugin": (
        "plugin-skill-frontmatter",
        "plugin-skill-body",
        "plugin-skill-resource",
    ),
    "role-composed": (
        "role-composed-frontmatter",
        "role-composed-body",
        "role-composed-resource",
    ),
    "role": (
        "role-skill-frontmatter",
        "role-skill-body",
        "role-skill-resource",
    ),
}


class _FlowMap(dict[str, object]):
    """A compact inline mapping in the generated YAML artifact."""


class _SnapshotDumper(yaml.SafeDumper):
    """Safe YAML dumper with compact measurement mappings."""


def _represent_flow_map(
    dumper: yaml.SafeDumper, value: _FlowMap
) -> yaml.nodes.MappingNode:
    return dumper.represent_mapping(
        "tag:yaml.org,2002:map", value, flow_style=True
    )


_SnapshotDumper.add_representer(_FlowMap, _represent_flow_map)


@dataclass(frozen=True)
class Component:
    """One stable eager or lazy context contribution."""

    id: str
    kind: str
    owner: str
    source: str
    delivery: str
    eager: bool
    byte_count: int
    tokens: int
    sha256: str

    def document(self) -> dict[str, object]:
        return {
            "id": self.id,
            "owner": self.owner,
            "source": self.source,
            "delivery": self.delivery,
            "bytes": self.byte_count,
            "tokens": self.tokens,
            "sha256": self.sha256,
        }


@dataclass(frozen=True)
class ProjectedLayout:
    """Seat-specific home projection entry points."""

    instructions: Path
    instructions_delivery: str
    skills_delivery: str


def _component(
    component_id: str,
    kind: str,
    owner: str,
    source: str,
    delivery: str,
    eager: bool,
    payload: bytes,
    *,
    token_text: str | None = None,
) -> Component:
    text = token_text if token_text is not None else payload.decode("utf-8", errors="replace")
    return Component(
        id=component_id,
        kind=kind,
        owner=owner,
        source=source,
        delivery=delivery,
        eager=eager,
        byte_count=len(payload),
        tokens=count_tokens(text),
        sha256=hashlib.sha256(payload).hexdigest(),
    )


def _validate_slug(value: str, label: str) -> None:
    if not SLUG_RE.fullmatch(value):
        raise RuntimeError(f"{label} must be a lowercase slug, found {value!r}")


@cache
def load_aos_layout_model_classes() -> dict[str, str]:
    """Read and validate the AOS-owned layout model-class registry."""
    try:
        document = json.loads(AOS_LAYOUT_MODEL_CLASSES_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(
            f"read AOS layout model classes {AOS_LAYOUT_MODEL_CLASSES_PATH}: {exc}"
        ) from exc
    if (
        not isinstance(document, dict)
        or document.get("format") != AOS_LAYOUT_MODEL_CLASSES_FORMAT
        or set(document) != {"format", "layouts"}
        or not isinstance(document.get("layouts"), dict)
        or not document["layouts"]
    ):
        raise RuntimeError(
            f"{AOS_LAYOUT_MODEL_CLASSES_PATH}: malformed layout model class registry"
        )
    layouts: dict[str, str] = {}
    for raw_layout, raw_model_class in document["layouts"].items():
        if not isinstance(raw_layout, str):
            raise RuntimeError(
                f"{AOS_LAYOUT_MODEL_CLASSES_PATH}: layout names must be text"
            )
        _validate_slug(raw_layout, "AOS layout")
        if raw_model_class not in {"frontier", "low-context"}:
            raise RuntimeError(
                f"{AOS_LAYOUT_MODEL_CLASSES_PATH}: layout {raw_layout!r} has "
                f"unsupported model class {raw_model_class!r}"
            )
        layouts[raw_layout] = raw_model_class
    return layouts


def model_class_for_seat(seat: str) -> str:
    """Return the AOS-owned model class for one supported projection layout."""
    _validate_slug(seat, "seat")
    layouts = load_aos_layout_model_classes()
    try:
        return layouts[seat]
    except KeyError as exc:
        supported = ", ".join(sorted(layouts))
        raise RuntimeError(
            f"unsupported AOS seat {seat!r}, expected one of: {supported}"
        ) from exc


def validate_role_seat(person_snapshot_path: Path, role: str, seat: str) -> None:
    """Fail unless the role exists and the seat is an AOS projection layout."""
    _validate_slug(role, "role")
    model_class_for_seat(seat)
    snapshot = load_person_snapshot(person_snapshot_path)
    roles = {entry.role for entry in snapshot.roles}
    if not roles:
        raise RuntimeError(
            f"{person_snapshot_path}: generated person snapshot contains no roles"
        )
    if role not in roles:
        raise RuntimeError(
            f"{person_snapshot_path}: role {role} is absent, found {sorted(roles)}"
        )


def _load_projection(projected_root: Path, seat: str) -> ProjectedLayout:
    manifest_path = projected_root / ".agent-compose" / "projection.json"
    try:
        document = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"read agent-compose projection {manifest_path}: {exc}") from exc
    if document.get("layout") != seat:
        raise RuntimeError(f"{manifest_path}: expected layout {seat}")
    files = document.get("files")
    if not isinstance(files, list) or not all(isinstance(item, str) for item in files):
        raise RuntimeError(f"{manifest_path}: projection files must be text paths")

    instruction_paths = [
        item
        for item in files
        if Path(item).name in {"AGENTS.md", "CLAUDE.md", ".goosehints"}
        and "skills" not in Path(item).parts
    ]
    skill_roots = {
        Path(*Path(item).parts[: Path(item).parts.index("skills") + 1]).as_posix()
        for item in files
        if "skills" in Path(item).parts
    }
    if len(instruction_paths) != 1 or len(skill_roots) != 1:
        raise RuntimeError(
            f"{manifest_path}: expected one instruction file and one skill root"
        )
    instructions_delivery = instruction_paths[0]
    skills_delivery = next(iter(skill_roots))
    instructions = _safe_child_path(
        projected_root, instructions_delivery, "projection instructions"
    )
    projected_skills = _safe_child_path(
        projected_root, skills_delivery, "projection skills root"
    )
    if not instructions.is_file() or not projected_skills.is_dir():
        raise RuntimeError(f"{manifest_path}: projected entry points are missing")
    return ProjectedLayout(
        instructions=instructions,
        instructions_delivery=instructions_delivery,
        skills_delivery=skills_delivery,
    )


def repository_identity(repo: Path) -> str:
    """Return a stable owner/name identity without retaining a remote host."""
    try:
        process = subprocess.run(
            ["git", "-C", str(repo), "config", "--get", "remote.origin.url"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return f"local/{repo.name}"
    remote = process.stdout.strip().removesuffix("/")
    if "://" in remote:
        path = urllib.parse.urlparse(remote).path
    elif ":" in remote:
        path = remote.split(":", 1)[1]
    else:
        path = remote
    parts = [part for part in path.strip("/").removesuffix(".git").split("/") if part]
    return "/".join(parts[-2:]) if len(parts) >= 2 else f"local/{repo.name}"


def _agents_components(
    provider: Path,
    repo: Path,
    cwd: Path,
    scenario: Scenario,
    provider_identity: str,
    repo_identity: str,
) -> list[Component]:
    """Adapt the shared AGENTS inventory cascade into snapshot components."""
    if not provider.is_dir():
        raise RuntimeError(f"provider root does not exist: {provider}")
    if not repo.is_dir():
        raise RuntimeError(f"repository root does not exist: {repo}")
    if not cwd.is_dir():
        raise RuntimeError(f"CWD does not exist: {cwd}")
    try:
        cwd_label = cwd.relative_to(repo).as_posix()
    except ValueError as exc:
        raise RuntimeError(f"CWD {cwd} is outside repository root {repo}") from exc

    sources = {
        provider_identity: provider,
        repo_identity: repo,
    }
    if provider_identity == repo_identity and provider != repo:
        raise RuntimeError(
            f"provider and repository both resolve to {provider_identity} "
            "but have different roots"
        )

    with tempfile.TemporaryDirectory(prefix="aos-role-seat-agents-") as temp:
        inventory_root = Path(temp)
        projects_root = inventory_root / "projects"
        try:
            for identity, source_root in sources.items():
                target = projects_root / identity
                target.parent.mkdir(parents=True, exist_ok=True)
                target.symlink_to(source_root, target_is_directory=True)
            substrate_manifest = inventory_root / "substrate.txt"
            fleet_manifest = inventory_root / "fleet.txt"
            substrate_manifest.write_text(
                f"{provider_identity} public\n", encoding="utf-8"
            )
            fleet_manifest.write_text(f"{repo_identity} public\n", encoding="utf-8")
            repositories = discover_repositories(
                substrate_manifest,
                fleet_manifest,
                projects_root,
            )
            cascade = active_cascade(
                repositories,
                scenario,
                current_repo=repo_identity,
                cwd=cwd_label,
                include_global_composed=False,
            )
        except (InventoryError, OSError) as exc:
            raise RuntimeError(f"inventory role-seat AGENTS cascade: {exc}") from exc

    components: list[Component] = []
    for index, source in enumerate(cascade["sources"]):
        source_id = str(source["source"])
        owner, separator, relative = source_id.partition(":")
        if not separator or owner not in sources:
            raise RuntimeError(f"AGENTS inventory returned unknown source {source_id}")
        path = sources[owner] / relative
        components.append(
            _component(
                f"agents:{index:03}:{relative}",
                "agents-cascade",
                owner,
                source_id,
                str(source["delivery_path"]),
                True,
                path.read_bytes(),
            )
        )
    return components


def _safe_child_path(root: Path, relative: object, label: str) -> Path:
    if not isinstance(relative, str) or not relative or Path(relative).is_absolute():
        raise RuntimeError(f"invalid {label}")
    candidate = (root / relative).resolve()
    try:
        candidate.relative_to(root.resolve())
    except ValueError as exc:
        raise RuntimeError(f"{label} escapes its root") from exc
    return candidate


def _load_manifest(
    bundle: Path, role: str, model_class: str
) -> tuple[dict[str, object], Path, Path]:
    manifest_path = bundle / "manifest.json"
    try:
        document = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"read bundle manifest {manifest_path}: {exc}") from exc
    if document.get("format") != "agent-compose.bundle":
        raise RuntimeError(f"{manifest_path}: unsupported bundle format")
    if document.get("role") != role:
        raise RuntimeError(f"{manifest_path}: expected role {role}")
    if document.get("model_class") != model_class:
        raise RuntimeError(f"{manifest_path}: expected model class {model_class}")
    delivery = document.get("delivery")
    if not isinstance(delivery, dict) or delivery.get("mode") != "native-skills":
        raise RuntimeError(f"{manifest_path}: expected native-skills delivery")
    instructions = _safe_child_path(
        bundle, delivery.get("instructions"), "delivery.instructions"
    )
    skills_root = _safe_child_path(
        bundle, delivery.get("skills_root"), "delivery.skills_root"
    )
    if not instructions.is_file() or not skills_root.is_dir():
        raise RuntimeError(f"{manifest_path}: bundle entry points are missing")
    return document, instructions, skills_root


def _validate_manifest_personalities(
    provider: Path,
    manifest: dict[str, object],
    role: str,
) -> tuple[str, ...]:
    projection_path = provider / ROLE_PERSONALITY_PROJECTION
    try:
        role_personalities = load_personality_projection(projection_path)
    except RolePersonalitySyncError as exc:
        raise RuntimeError(
            f"read AOS role personality projection: {exc}"
        ) from exc
    expected = role_personalities.get(role)
    if expected is None:
        raise RuntimeError(
            f"{projection_path}: role personality projection lacks role {role}"
        )
    actual = manifest.get("personalities")
    if (
        not isinstance(actual, list)
        or not actual
        or not all(isinstance(value, str) and value for value in actual)
    ):
        raise RuntimeError("agent-compose bundle manifest has malformed personalities")
    if tuple(actual) != expected:
        raise RuntimeError(
            "agent-compose bundle personalities differ from the AOS projection "
            f"for role {role}: bundle={actual}, aos={list(expected)}"
        )
    return expected


def _canonical_skill(
    provider: Path, source_id: str, skill_id: str
) -> tuple[str, str, Path | None]:
    if urllib.parse.unquote(source_id) == PERSON_SOURCE_ID:
        if skill_id.startswith("personality-"):
            return "personality", PERSON_SOURCE_ID, None
        if skill_id.startswith("role-"):
            return "role-skill", PERSON_SOURCE_ID, None
        raise RuntimeError(f"bundle person source contains unknown skill {skill_id}")
    if source_id != SOURCE_ID:
        return "external-skill", source_id, None
    ordinary = provider / ".agents" / "skills" / skill_id
    composed = provider / ".agents" / "composed" / skill_id
    if (composed / "COMPOSED.md").is_file():
        return "role-composed", SOURCE_ID, composed
    if (ordinary / "SKILL.md").is_file():
        kind = "personality" if skill_id.startswith("personality-") else "ordinary-skill"
        return kind, SOURCE_ID, ordinary
    raise RuntimeError(f"bundle skill {source_id}/{skill_id} has no provider source")


def _skill_entrypoint(
    skill_dir: Path,
    *,
    component_prefix: str,
    kind_prefix: str,
    owner: str,
    source_entrypoint: str,
    delivery_entrypoint: str,
) -> list[Component]:
    entrypoint = skill_dir / "SKILL.md"
    try:
        text = entrypoint.read_text(encoding="utf-8")
    except OSError as exc:
        raise RuntimeError(f"read selected skill {entrypoint}: {exc}") from exc
    metadata, body = _split_frontmatter(text)
    name = metadata.get("name")
    description = metadata.get("description")
    if not isinstance(name, str) or not name.strip():
        raise RuntimeError(f"{entrypoint}: missing skill name")
    if not isinstance(description, str) or not description.strip():
        raise RuntimeError(f"{entrypoint}: missing skill description")

    eager_text = f"{name}: {description}"
    components = [
        _component(
            f"{component_prefix}:frontmatter",
            f"{kind_prefix}-frontmatter",
            owner,
            source_entrypoint,
            delivery_entrypoint + "#frontmatter",
            True,
            eager_text.encode("utf-8"),
            token_text=eager_text,
        )
    ]
    if body:
        components.append(
            _component(
                f"{component_prefix}:body",
                f"{kind_prefix}-body",
                owner,
                source_entrypoint,
                delivery_entrypoint + "#body",
                False,
                body.encode("utf-8"),
            )
        )
    return components


def _bundle_skill_components(
    skills_root: Path, provider: Path, skills_delivery: str
) -> tuple[list[Component], set[str]]:
    components: list[Component] = []
    selected_ids: set[str] = set()
    for source_dir in sorted(path for path in skills_root.iterdir() if path.is_dir()):
        for skill_dir in sorted(path for path in source_dir.iterdir() if path.is_dir()):
            source_id = source_dir.name
            skill_id = skill_dir.name
            if skill_id in selected_ids:
                raise RuntimeError(f"bundle selects duplicate skill id {skill_id}")
            selected_ids.add(skill_id)
            kind, owner, canonical = _canonical_skill(provider, source_id, skill_id)
            if canonical is None:
                source_entrypoint = f"bundle:{source_id}/{skill_id}/SKILL.md"
            elif kind == "role-composed":
                source_entrypoint = (
                    f"provider:.agents/composed/{skill_id}/COMPOSED.md"
                )
            else:
                source_entrypoint = f"provider:.agents/skills/{skill_id}/SKILL.md"
            delivery_entrypoint = f"{skills_delivery}/{skill_id}/SKILL.md"
            components.extend(
                _skill_entrypoint(
                    skill_dir,
                    component_prefix=f"skill:{source_id}:{skill_id}",
                    kind_prefix=kind,
                    owner=owner,
                    source_entrypoint=source_entrypoint,
                    delivery_entrypoint=delivery_entrypoint,
                )
            )
            for resource in sorted(
                path for path in skill_dir.rglob("*") if path.is_file() and path.name != "SKILL.md"
            ):
                relative = resource.relative_to(skill_dir).as_posix()
                if canonical is None:
                    source = f"bundle:{source_id}/{skill_id}/{relative}"
                else:
                    source = (
                        f"provider:{canonical.relative_to(provider).as_posix()}/{relative}"
                    )
                components.append(
                    _component(
                        f"skill:{source_id}:{skill_id}:resource:{relative}",
                        f"{kind}-resource",
                        owner,
                        source,
                        f"{skills_delivery}/{skill_id}/{relative}",
                        False,
                        resource.read_bytes(),
                    )
                )
    return components, selected_ids


def _plugin_skill_components(
    roots: Iterable[Path], selected_ids: set[str], skills_delivery: str
) -> list[Component]:
    components: list[Component] = []
    for index, root in enumerate(sorted(path.resolve() for path in roots)):
        if not root.is_dir():
            raise RuntimeError(f"skill root does not exist: {root}")
        root_label = f"skill-root-{index}"
        for skill_dir in sorted(path for path in root.iterdir() if path.is_dir()):
            if not (skill_dir / "SKILL.md").is_file():
                continue
            skill_id = skill_dir.name
            if skill_id in selected_ids:
                raise RuntimeError(f"skill id {skill_id} is delivered more than once")
            selected_ids.add(skill_id)
            entrypoint = f"{root_label}:{skill_id}/SKILL.md"
            components.extend(
                _skill_entrypoint(
                    skill_dir,
                    component_prefix=f"skill:{root_label}:{skill_id}",
                    kind_prefix="plugin-skill",
                    owner=root_label,
                    source_entrypoint=entrypoint,
                    delivery_entrypoint=f"{skills_delivery}/{skill_id}/SKILL.md",
                )
            )
            for resource in sorted(
                path for path in skill_dir.rglob("*") if path.is_file() and path.name != "SKILL.md"
            ):
                relative = resource.relative_to(skill_dir).as_posix()
                components.append(
                    _component(
                        f"skill:{root_label}:{skill_id}:resource:{relative}",
                        "plugin-skill-resource",
                        root_label,
                        f"{root_label}:{skill_id}/{relative}",
                        f"{skills_delivery}/{skill_id}/{relative}",
                        False,
                        resource.read_bytes(),
                    )
                )
    return components


def read_mcporter_server_names(path: Path) -> list[str]:
    """Read only stable server names. Seats receive no eager schemas from this file."""
    if not path.is_file():
        return []
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"read mcporter inventory {path}: {exc}") from exc
    servers = document.get("mcpServers")
    if not isinstance(servers, dict):
        raise RuntimeError(f"{path}: mcpServers must be an object")
    return sorted(str(name) for name in servers)


def _totals(components: list[Component], eager: bool) -> dict[str, int]:
    selected = [component for component in components if component.eager is eager]
    return {
        "components": len(selected),
        "bytes": sum(component.byte_count for component in selected),
        "tokens": sum(component.tokens for component in selected),
    }


def _group_components(
    components: list[Component],
) -> dict[str, dict[str, list[dict[str, object]]]]:
    """Group stable component rows by delivery class, then component kind."""
    grouped: dict[str, dict[str, list[dict[str, object]]]] = {}
    for eager, delivery_class in ((True, "eager"), (False, "lazy")):
        kinds = sorted({component.kind for component in components if component.eager is eager})
        grouped[delivery_class] = {
            kind: [
                component.document()
                for component in components
                if component.eager is eager and component.kind == kind
            ]
            for kind in kinds
        }
    return grouped


def _skill_component_identity(component_id: object) -> tuple[str, str, str]:
    if not isinstance(component_id, str):
        raise RuntimeError("skill component has no id")
    parts = component_id.split(":", 3)
    if len(parts) != 4 or parts[0] != "skill":
        raise RuntimeError(f"malformed skill component id: {component_id}")
    owner, skill_id, stage = parts[1:]
    return owner, skill_id, stage


def _skill_class(kind: object) -> str:
    if not isinstance(kind, str):
        raise RuntimeError("skill component has no kind")
    for skill_class, kinds in SKILL_CLASS_KINDS.items():
        if kind in kinds:
            return skill_class
    raise RuntimeError(f"unsupported skill component kind: {kind}")


def _skill_records_from_rows(
    rows: Iterable[dict[str, object]],
) -> dict[str, dict[str, object]]:
    buckets: dict[str, list[dict[str, object]]] = defaultdict(list)
    for raw in rows:
        _source, skill_id, _stage = _skill_component_identity(raw.get("id"))
        owner = raw.get("owner")
        if not isinstance(owner, str) or not owner:
            raise RuntimeError("skill component has no owner")
        buckets[f"{owner}/{skill_id}"].append(raw)

    records: dict[str, dict[str, object]] = {}
    for skill_key in sorted(buckets):
        components = sorted(buckets[skill_key], key=lambda raw: str(raw["id"]))
        eager = [raw for raw in components if raw.get("eager") is True]
        lazy = [raw for raw in components if raw.get("eager") is False]
        if len(eager) != 1 or not lazy:
            raise RuntimeError(
                f"skill {skill_key} must have one eager frontmatter and lazy content"
            )
        frontmatter = eager[0]
        skill_class = _skill_class(frontmatter.get("kind"))
        if any(_skill_class(raw.get("kind")) != skill_class for raw in components):
            raise RuntimeError(f"skill {skill_key} spans multiple component classes")
        body = [raw for raw in lazy if str(raw.get("kind", "")).endswith("-body")]
        resources = [
            raw for raw in lazy if str(raw.get("kind", "")).endswith("-resource")
        ]
        if len(body) != 1 or len(body) + len(resources) != len(lazy):
            raise RuntimeError(f"skill {skill_key} has malformed lazy components")
        records[skill_key] = _FlowMap(
            {
                "class": skill_class,
                "eager": int(frontmatter.get("tokens", 0)),
                "lazy": sum(int(raw.get("tokens", 0)) for raw in lazy),
                "resources": len(resources),
            }
        )
    return records


def _group_skills(components: list[Component]) -> dict[str, dict[str, object]]:
    rows = [
        {
            **component.document(),
            "kind": component.kind,
            "eager": component.eager,
        }
        for component in components
    ]
    return _skill_records_from_rows(rows)


def _component_breakdown(
    components: list[Component],
) -> dict[str, dict[str, _FlowMap]]:
    breakdown: dict[str, dict[str, _FlowMap]] = {}
    for eager, delivery_class in ((True, "eager"), (False, "lazy")):
        kinds = sorted({component.kind for component in components if component.eager is eager})
        breakdown[delivery_class] = {}
        for kind in kinds:
            selected = [
                component
                for component in components
                if component.eager is eager and component.kind == kind
            ]
            breakdown[delivery_class][kind] = _FlowMap(
                {
                    "components": len(selected),
                    "bytes": sum(component.byte_count for component in selected),
                    "tokens": sum(component.tokens for component in selected),
                }
            )
    return breakdown


def build_snapshot(
    bundle: Path,
    projected_root: Path,
    provider: Path,
    repo: Path,
    cwd: Path,
    *,
    role: str,
    seat: str,
    plugin_roots: Iterable[Path] = (),
    mcp_servers: Iterable[str] = (),
) -> dict[str, object]:
    """Build one deterministic snapshot from a verified role-seat projection."""
    _validate_slug(role, "role")
    _validate_slug(seat, "seat")
    provider = provider.resolve()
    repo = repo.resolve()
    cwd = cwd.resolve()
    provider_identity = repository_identity(provider)
    repo_identity = repository_identity(repo)
    model_class = model_class_for_seat(seat)
    scenario = Scenario(role=role, intent="context-measurement", harness=seat)
    manifest, instructions, skills_root = _load_manifest(
        bundle.resolve(), role, model_class
    )
    personalities = _validate_manifest_personalities(provider, manifest, role)
    projection = _load_projection(projected_root.resolve(), seat)
    if projection.instructions.read_bytes() != instructions.read_bytes():
        raise RuntimeError("projected role instructions differ from the verified bundle")

    components = [
        _component(
            "instructions:role",
            "role-instructions",
            "agent-compose+person:kai+aos-public",
            "bundle:content/instructions.md",
            projection.instructions_delivery,
            True,
            instructions.read_bytes(),
        )
    ]
    components.extend(
        _agents_components(
            provider,
            repo,
            cwd,
            scenario,
            provider_identity,
            repo_identity,
        )
    )

    bundle_components, selected_ids = _bundle_skill_components(
        skills_root,
        provider,
        projection.skills_delivery,
    )
    expected_personality_skills = {
        personality_skill_id(personality) for personality in personalities
    }
    selected_personality_skills = {
        skill_id
        for skill_id in selected_ids
        if skill_id.startswith("personality-")
    }
    if selected_personality_skills != expected_personality_skills:
        raise RuntimeError(
            "agent-compose bundle personality skills differ from the AOS "
            f"projection: bundle={sorted(selected_personality_skills)}, "
            f"aos={sorted(expected_personality_skills)}"
        )
    components.extend(bundle_components)
    components.extend(
        _plugin_skill_components(
            plugin_roots,
            selected_ids,
            projection.skills_delivery,
        )
    )
    servers = sorted(set(mcp_servers))
    if servers:
        components.append(
            _component(
                "mcp:deferred",
                "mcp-server-registration",
                "mcporter",
                "mcporter:inventory",
                "deferred",
                False,
                b"",
            )
        )
    components.sort(key=lambda component: component.id)

    try:
        cwd_label = cwd.relative_to(repo).as_posix() or "."
    except ValueError as exc:
        raise RuntimeError(f"CWD {cwd} is outside repository root {repo}") from exc
    skill_components = [
        component for component in components if component.id.startswith("skill:")
    ]
    other_components = [
        component for component in components if not component.id.startswith("skill:")
    ]
    document: dict[str, object] = {
        "format": FORMAT,
        "subject": {"role": role, "seat": seat},
        "provider": provider_identity,
        "repository": repo_identity,
        "cwd": cwd_label,
        "tokenizer": TOKENIZER_NOTE,
        "bundle": {
            "format": manifest["format"],
            "model_class": model_class,
            "personalities": list(personalities),
            "sources": manifest.get("sources", []),
        },
        "mcp": {
            "delivery": "deferred",
            "server_count": len(servers),
            "eager_schema_count": 0,
        },
        "totals": {
            "eager": _totals(components, True),
            "lazy": _totals(components, False),
        },
        "breakdown": _component_breakdown(components),
        "components": _group_components(other_components),
        "skills": _group_skills(skill_components),
    }
    payload_material = {
        "snapshot": document,
        "component_hashes": [
            {"id": component.id, "sha256": component.sha256}
            for component in components
        ],
    }
    canonical = json.dumps(
        payload_material, sort_keys=True, separators=(",", ":")
    ).encode()
    document["payload_hash"] = hashlib.sha256(canonical).hexdigest()
    return document


def _request_text(provider_root: str, role: str, seat: str) -> str:
    root = json.dumps(provider_root)
    model_class = model_class_for_seat(seat)
    return (
        "compose {\n"
        f'    role "{role}"\n'
        '    delivery "native-skills"\n'
        f'    model-class "{model_class}"\n'
        f'    source "{SOURCE_ID}" root={root} required=#true\n'
        "}\n"
    )


def capture_snapshot(
    provider: Path,
    repo: Path,
    cwd: Path,
    *,
    role: str,
    seat: str,
    agent_compose: str,
    plugin_roots: Iterable[Path] = (),
    mcporter_path: Path,
) -> dict[str, object]:
    """Materialize and project one role-seat context without invoking an agent."""
    _validate_slug(role, "role")
    _validate_slug(seat, "seat")
    executable = shutil.which(agent_compose)
    if executable is None:
        raise RuntimeError(f"agent-compose executable not found: {agent_compose}")
    with tempfile.TemporaryDirectory(prefix="aos-role-seat-context-") as temp:
        root = Path(temp)
        staged_provider = root / "provider"
        try:
            shutil.copytree(provider / ".agents", staged_provider / ".agents")
        except OSError as exc:
            raise RuntimeError(f"stage AOS provider {provider}: {exc}") from exc
        request = root / "request.kdl"
        output = root / "bundles"
        roster = root / "roster"
        projected = root / "projected"
        request.write_text(_request_text("provider", role, seat), encoding="utf-8")
        try:
            subprocess.run(
                [executable, "roster", "--out", str(roster), str(staged_provider)],
                capture_output=True,
                text=True,
                encoding="utf-8",
                check=True,
            )
            validate_role_seat(roster / "person.json", role, seat)
            process = subprocess.run(
                [executable, "compose", "--out", str(output), str(request)],
                capture_output=True,
                text=True,
                encoding="utf-8",
                check=True,
                cwd=root,
            )
        except (OSError, subprocess.CalledProcessError) as exc:
            detail = exc.stderr.strip() if isinstance(exc, subprocess.CalledProcessError) else str(exc)
            raise RuntimeError(f"agent-compose failed to resolve role-seat context: {detail}") from exc
        manifests = sorted(output.glob("*/manifest.json"))
        if len(manifests) != 1:
            raise RuntimeError(
                "agent-compose did not produce exactly one bundle manifest "
                f"(found {len(manifests)}; output={process.stdout.strip()!r})"
            )
        bundle = manifests[0].parent
        try:
            subprocess.run(
                [
                    executable,
                    "project",
                    "--layout",
                    seat,
                    "--target",
                    str(projected),
                    "--scope",
                    "home",
                    str(bundle),
                ],
                capture_output=True,
                text=True,
                encoding="utf-8",
                check=True,
            )
        except (OSError, subprocess.CalledProcessError) as exc:
            detail = exc.stderr.strip() if isinstance(exc, subprocess.CalledProcessError) else str(exc)
            raise RuntimeError(f"agent-compose failed to project seat {seat}: {detail}") from exc
        return build_snapshot(
            bundle,
            projected,
            provider,
            repo,
            cwd,
            role=role,
            seat=seat,
            plugin_roots=plugin_roots,
            mcp_servers=read_mcporter_server_names(mcporter_path),
        )


def write_snapshot(path: Path, snapshot: dict[str, object]) -> None:
    if path.suffix.lower() not in {".yaml", ".yml"}:
        raise RuntimeError("role-seat context snapshots must use a .yaml or .yml path")
    if snapshot.get("format") != FORMAT:
        raise RuntimeError("only the current role-seat snapshot format can be written")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        yaml.dump(
            snapshot,
            Dumper=_SnapshotDumper,
            allow_unicode=False,
            default_flow_style=False,
            sort_keys=False,
            width=4096,
        ),
        encoding="utf-8",
    )


def load_snapshot(path: Path) -> dict[str, object]:
    try:
        document = yaml.safe_load(path.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as exc:
        raise RuntimeError(f"read role-seat context snapshot {path}: {exc}") from exc
    if not isinstance(document, dict):
        raise RuntimeError(f"{path}: role-seat context snapshot must be a mapping")
    if document.get("format") != FORMAT:
        raise RuntimeError(f"{path}: unsupported role-seat context snapshot format")
    subject = document.get("subject")
    if (
        not isinstance(subject, dict)
        or not isinstance(subject.get("role"), str)
        or not isinstance(subject.get("seat"), str)
    ):
        raise RuntimeError(f"{path}: snapshot subject must name a role and seat")
    _validate_slug(subject["role"], "snapshot role")
    _validate_slug(subject["seat"], "snapshot seat")
    components = document.get("components")
    if not isinstance(components, dict):
        raise RuntimeError(f"{path}: grouped snapshot components must be a mapping")
    if not isinstance(document.get("skills"), dict):
        raise RuntimeError(f"{path}: collapsed snapshot skills must be a mapping")
    if not isinstance(document.get("breakdown"), dict):
        raise RuntimeError(f"{path}: snapshot breakdown must be a mapping")
    list(_snapshot_component_rows(document))
    _snapshot_skill_records(document)
    return document


def _snapshot_base_component_rows(
    snapshot: dict[str, object],
) -> Iterable[dict[str, object]]:
    components = snapshot.get("components")
    if not isinstance(components, dict) or set(components) != {"eager", "lazy"}:
        raise RuntimeError("snapshot component groups must be eager and lazy")
    for eager, delivery_class in ((True, "eager"), (False, "lazy")):
        kinds = components[delivery_class]
        if not isinstance(kinds, dict):
            raise RuntimeError(f"snapshot {delivery_class} components must be grouped by kind")
        for kind in sorted(kinds):
            rows = kinds[kind]
            if not isinstance(kind, str) or not isinstance(rows, list):
                raise RuntimeError(f"snapshot {delivery_class} component group is malformed")
            for raw in rows:
                if not isinstance(raw, dict):
                    raise RuntimeError("snapshot contains a malformed component")
                row = dict(raw)
                row["kind"] = kind
                row["eager"] = eager
                yield row


def _snapshot_skill_records(
    snapshot: dict[str, object],
) -> dict[str, dict[str, object]]:
    skills = snapshot.get("skills")
    if not isinstance(skills, dict):
        raise RuntimeError("snapshot skills must be a mapping")
    records: dict[str, dict[str, object]] = {}
    for skill_key in sorted(skills):
        raw = skills[skill_key]
        if (
            not isinstance(skill_key, str)
            or "/" not in skill_key
            or not isinstance(raw, dict)
        ):
            raise RuntimeError(f"snapshot skill {skill_key!r} is malformed")
        if (
            raw.get("class") not in SKILL_CLASS_KINDS
            or not isinstance(raw.get("eager"), int)
            or not isinstance(raw.get("lazy"), int)
            or not isinstance(raw.get("resources"), int)
        ):
            raise RuntimeError(f"snapshot skill {skill_key!r} is malformed")
        records[skill_key] = raw
    return records


def _snapshot_component_rows(
    snapshot: dict[str, object],
) -> Iterable[dict[str, object]]:
    yield from _snapshot_base_component_rows(snapshot)


def _component_kind_totals(snapshot: dict[str, object], eager: bool) -> list[tuple[str, int, int]]:
    breakdown = snapshot.get("breakdown")
    delivery_class = "eager" if eager else "lazy"
    if isinstance(breakdown, dict):
        kinds = breakdown.get(delivery_class)
        if not isinstance(kinds, dict):
            raise RuntimeError(f"snapshot {delivery_class} breakdown is malformed")
        return [
            (kind, int(values["bytes"]), int(values["tokens"]))
            for kind, values in sorted(kinds.items())
            if isinstance(kind, str) and isinstance(values, dict)
        ]
    totals: dict[str, list[int]] = defaultdict(lambda: [0, 0])
    for raw in _snapshot_component_rows(snapshot):
        if raw.get("eager") is not eager:
            continue
        kind = str(raw.get("kind"))
        totals[kind][0] += int(raw.get("bytes", 0))
        totals[kind][1] += int(raw.get("tokens", 0))
    return [(kind, values[0], values[1]) for kind, values in sorted(totals.items())]


def render_snapshot(snapshot: dict[str, object]) -> str:
    subject = snapshot["subject"]
    totals = snapshot["totals"]
    mcp = snapshot["mcp"]
    assert (
        isinstance(subject, dict)
        and isinstance(totals, dict)
        and isinstance(mcp, dict)
    )
    lines = [
        "Role-seat context baseline",
        f"  subject    {subject['role']} / {subject['seat']}",
        f"  repository {snapshot['repository']}  cwd {snapshot['cwd']}",
        f"  payload    {snapshot['payload_hash']}",
    ]
    for eager, label in ((True, "eager"), (False, "lazy")):
        total = totals[label]
        assert isinstance(total, dict)
        lines.append(
            f"  {label:10} {total['tokens']:6} tok  {total['bytes']:8} bytes  "
            f"{total['components']} components"
        )
        for kind, byte_count, tokens in _component_kind_totals(snapshot, eager):
            lines.append(f"    {kind:32} {tokens:6} tok  {byte_count:8} bytes")
    lines.append(
        "  mcp        "
        f"{mcp['eager_schema_count']} eager schemas, {mcp['server_count']} deferred servers"
    )
    return "\n".join(lines)


def _snapshot_components(snapshot: dict[str, object]) -> dict[str, dict[str, object]]:
    out: dict[str, dict[str, object]] = {}
    for raw in _snapshot_component_rows(snapshot):
        if not isinstance(raw.get("id"), str):
            raise RuntimeError("snapshot contains a malformed component")
        if str(raw["id"]).startswith("skill:"):
            continue
        out[str(raw["id"])] = raw
    for skill_key, record in _snapshot_skill_records(snapshot).items():
        component_id = f"skill:{skill_key}"
        out[component_id] = {
            "id": component_id,
            "kind": "skill",
            "class": record["class"],
            "eager": record["eager"],
            "lazy": record["lazy"],
            "resources": record["resources"],
            "tokens": int(record["eager"]) + int(record["lazy"]),
        }
    return out


def render_delta(before: dict[str, object], after: dict[str, object]) -> str:
    """Render stable component and total changes for the same role and seat."""
    for key in ("subject", "repository", "cwd", "tokenizer"):
        if before.get(key) != after.get(key):
            raise RuntimeError(f"cannot compare snapshots with different {key}")
    before_totals = before["totals"]
    after_totals = after["totals"]
    assert isinstance(before_totals, dict) and isinstance(after_totals, dict)
    lines = ["Role-seat context delta"]
    for label in ("eager", "lazy"):
        left = before_totals[label]
        right = after_totals[label]
        assert isinstance(left, dict) and isinstance(right, dict)
        delta = int(right["tokens"]) - int(left["tokens"])
        lines.append(
            f"  {label:5} {left['tokens']:6} -> {right['tokens']:6} tok  ({delta:+d})"
        )

    left_components = _snapshot_components(before)
    right_components = _snapshot_components(after)
    added = sorted(right_components.keys() - left_components.keys())
    removed = sorted(left_components.keys() - right_components.keys())
    changed = sorted(
        component_id
        for component_id in left_components.keys() & right_components.keys()
        if left_components[component_id] != right_components[component_id]
    )
    lines.append(
        f"  components +{len(added)}  -{len(removed)}  ~{len(changed)}"
    )
    for marker, ids, source in (
        ("+", added, right_components),
        ("-", removed, left_components),
        ("~", changed, right_components),
    ):
        for component_id in ids:
            lines.append(f"    {marker} {component_id}  {source[component_id]['tokens']} tok")
    return "\n".join(lines)
