"""Capture and compare the fixed structural Goose context baseline."""

from __future__ import annotations

import hashlib
import json
import re
import shutil
import subprocess
import tempfile
import urllib.parse
from collections import defaultdict
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Iterable

from agentic_os.context_budget_tokens import TOKENIZER_NOTE, count_tokens
from agentic_os.generators.generate_agent_compose import _split_frontmatter

FORMAT = "agentic-os.goose-context.v1"
HARNESS = "goose"
ROLE = "ops"
INTENT = "operational-decision"
SOURCE_ID = "aos-public"
INSTRUCTIONS_LOAD_POINT = ".config/goose/.goosehints"
SKILLS_LOAD_POINT = ".agents/skills"
REPO_ROOT = Path(__file__).resolve().parents[1]


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
        raw = asdict(self)
        raw["bytes"] = raw.pop("byte_count")
        return raw


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


def _named_block(text: str, node: str, name: str, path: Path) -> str:
    lines = text.splitlines()
    opener = re.compile(
        rf'^\s*{re.escape(node)}\s+"?{re.escape(name)}"?\s*\{{\s*$'
    )
    openings = [index for index, line in enumerate(lines) if opener.fullmatch(line)]
    if len(openings) != 1:
        raise RuntimeError(f"{path}: expected one {node} {name} block, found {len(openings)}")

    start = openings[0]
    depth = 0
    for index in range(start, len(lines)):
        structural = lines[index].split("//", 1)[0]
        depth += structural.count("{")
        depth -= structural.count("}")
        if depth == 0:
            return "\n".join(lines[start + 1 : index])
    raise RuntimeError(f"{path}: {node} {name} block has no closing brace")


def validate_goose_route(roles_path: Path) -> None:
    """Fail unless the committed fixed role and intent still select Goose."""
    try:
        text = roles_path.read_text(encoding="utf-8")
    except OSError as exc:
        raise RuntimeError(f"read {roles_path}: {exc}") from exc
    role_body = _named_block(text, "role", ROLE, roles_path)
    intent_body = _named_block(role_body, "intent", INTENT, roles_path)
    harnesses = re.findall(r'(?m)^\s*harness\s+"?([a-z0-9-]+)"?\s*$', intent_body)
    if harnesses != [HARNESS]:
        raise RuntimeError(
            f"{roles_path}: {ROLE}/{INTENT} must select {HARNESS}, found {harnesses}"
        )


def agents_cascade(repo: Path, cwd: Path) -> list[Path]:
    """Return root-to-leaf AGENTS.md files effective for the declared CWD."""
    repo = repo.resolve()
    cwd = cwd.resolve()
    if not repo.is_dir():
        raise RuntimeError(f"repository root does not exist: {repo}")
    if not cwd.is_dir():
        raise RuntimeError(f"CWD does not exist: {cwd}")
    try:
        relative = cwd.relative_to(repo)
    except ValueError as exc:
        raise RuntimeError(f"CWD {cwd} is outside repository root {repo}") from exc

    directories = [repo]
    current = repo
    for part in relative.parts:
        current /= part
        directories.append(current)
    return [path / "AGENTS.md" for path in directories if (path / "AGENTS.md").is_file()]


def repository_identity(repo: Path) -> str:
    """Return a stable owner/name identity without retaining a remote host."""
    try:
        process = subprocess.run(
            ["git", "-C", str(repo), "config", "--get", "remote.origin.url"],
            capture_output=True,
            text=True,
            check=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return repo.name
    remote = process.stdout.strip().removesuffix("/")
    if "://" in remote:
        path = urllib.parse.urlparse(remote).path
    elif ":" in remote:
        path = remote.split(":", 1)[1]
    else:
        path = remote
    parts = [part for part in path.strip("/").removesuffix(".git").split("/") if part]
    return "/".join(parts[-2:]) if len(parts) >= 2 else repo.name


def _safe_bundle_path(bundle: Path, relative: object, label: str) -> Path:
    if not isinstance(relative, str) or not relative or Path(relative).is_absolute():
        raise RuntimeError(f"bundle manifest has invalid {label}")
    candidate = (bundle / relative).resolve()
    try:
        candidate.relative_to(bundle.resolve())
    except ValueError as exc:
        raise RuntimeError(f"bundle manifest {label} escapes the bundle") from exc
    return candidate


def _load_manifest(bundle: Path) -> tuple[dict[str, object], Path, Path]:
    manifest_path = bundle / "manifest.json"
    try:
        document = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"read bundle manifest {manifest_path}: {exc}") from exc
    if document.get("format") != "agent-compose.bundle":
        raise RuntimeError(f"{manifest_path}: unsupported bundle format")
    if document.get("role") != ROLE:
        raise RuntimeError(f"{manifest_path}: expected role {ROLE}")
    delivery = document.get("delivery")
    if not isinstance(delivery, dict) or delivery.get("mode") != "native-skills":
        raise RuntimeError(f"{manifest_path}: expected native-skills delivery")
    instructions = _safe_bundle_path(
        bundle, delivery.get("instructions"), "delivery.instructions"
    )
    skills_root = _safe_bundle_path(
        bundle, delivery.get("skills_root"), "delivery.skills_root"
    )
    if not instructions.is_file() or not skills_root.is_dir():
        raise RuntimeError(f"{manifest_path}: bundle entry points are missing")
    return document, instructions, skills_root


def _canonical_skill(
    provider: Path, source_id: str, skill_id: str
) -> tuple[str, str, Path | None]:
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
    skills_root: Path, provider: Path
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
            delivery_entrypoint = f"{SKILLS_LOAD_POINT}/{skill_id}/SKILL.md"
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
                        f"{SKILLS_LOAD_POINT}/{skill_id}/{relative}",
                        False,
                        resource.read_bytes(),
                    )
                )
    return components, selected_ids


def _plugin_skill_components(
    roots: Iterable[Path], selected_ids: set[str]
) -> list[Component]:
    components: list[Component] = []
    for index, root in enumerate(sorted(path.resolve() for path in roots)):
        if not root.is_dir():
            raise RuntimeError(f"Goose skill root does not exist: {root}")
        root_label = f"skill-root-{index}"
        for skill_dir in sorted(path for path in root.iterdir() if path.is_dir()):
            if not (skill_dir / "SKILL.md").is_file():
                continue
            skill_id = skill_dir.name
            if skill_id in selected_ids:
                raise RuntimeError(f"Goose skill id {skill_id} is delivered more than once")
            selected_ids.add(skill_id)
            entrypoint = f"{root_label}:{skill_id}/SKILL.md"
            components.extend(
                _skill_entrypoint(
                    skill_dir,
                    component_prefix=f"skill:{root_label}:{skill_id}",
                    kind_prefix="plugin-skill",
                    owner=root_label,
                    source_entrypoint=entrypoint,
                    delivery_entrypoint=f"{SKILLS_LOAD_POINT}/{skill_id}/SKILL.md",
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
                        f"{SKILLS_LOAD_POINT}/{skill_id}/{relative}",
                        False,
                        resource.read_bytes(),
                    )
                )
    return components


def read_mcporter_server_names(path: Path) -> list[str]:
    """Read only stable server names. Goose receives no eager schemas from this file."""
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


def build_snapshot(
    bundle: Path,
    provider: Path,
    repo: Path,
    cwd: Path,
    *,
    plugin_roots: Iterable[Path] = (),
    mcp_servers: Iterable[str] = (),
) -> dict[str, object]:
    """Build one deterministic snapshot from a verified native bundle."""
    provider = provider.resolve()
    repo = repo.resolve()
    cwd = cwd.resolve()
    provider_identity = repository_identity(provider)
    repo_identity = repository_identity(repo)
    validate_goose_route(provider / ".agents" / "roles.kdl")
    manifest, instructions, skills_root = _load_manifest(bundle.resolve())

    components = [
        _component(
            "instructions:goose",
            "role-instructions",
            "agent-compose+aos-public",
            "bundle:content/instructions.md",
            INSTRUCTIONS_LOAD_POINT,
            True,
            instructions.read_bytes(),
        )
    ]
    for index, agents_path in enumerate(agents_cascade(repo, cwd)):
        relative = agents_path.relative_to(repo).as_posix()
        components.append(
            _component(
                f"agents:{index:03}:{relative}",
                "agents-cascade",
                repo_identity,
                f"repo:{relative}",
                f"cwd:{relative}",
                True,
                agents_path.read_bytes(),
            )
        )

    bundle_components, selected_ids = _bundle_skill_components(skills_root, provider)
    components.extend(bundle_components)
    components.extend(_plugin_skill_components(plugin_roots, selected_ids))
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
    document: dict[str, object] = {
        "format": FORMAT,
        "lane": {"harness": HARNESS, "role": ROLE, "intent": INTENT},
        "provider": provider_identity,
        "repository": repo_identity,
        "cwd": cwd_label,
        "tokenizer": TOKENIZER_NOTE,
        "bundle": {
            "format": manifest["format"],
            "density": manifest.get("density"),
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
        "components": [component.document() for component in components],
    }
    canonical = json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
    document["payload_hash"] = hashlib.sha256(canonical).hexdigest()
    return document


def _request_text(provider_root: str) -> str:
    root = json.dumps(provider_root)
    return (
        "compose {\n"
        f'    role "{ROLE}"\n'
        '    delivery "native-skills"\n'
        '    density "full"\n'
        f'    source "{SOURCE_ID}" root={root} required=#true\n'
        "}\n"
    )


def capture_snapshot(
    provider: Path,
    repo: Path,
    cwd: Path,
    *,
    agent_compose: str,
    plugin_roots: Iterable[Path] = (),
    mcporter_path: Path,
) -> dict[str, object]:
    """Materialize the fixed bundle locally, then measure it without inference."""
    executable = shutil.which(agent_compose)
    if executable is None:
        raise RuntimeError(f"agent-compose executable not found: {agent_compose}")
    validate_goose_route(provider / ".agents" / "roles.kdl")
    with tempfile.TemporaryDirectory(prefix="aos-goose-context-") as temp:
        root = Path(temp)
        staged_provider = root / "provider"
        try:
            shutil.copytree(provider / ".agents", staged_provider / ".agents")
        except OSError as exc:
            raise RuntimeError(f"stage AOS provider {provider}: {exc}") from exc
        request = root / "request.kdl"
        output = root / "bundles"
        request.write_text(_request_text("provider"), encoding="utf-8")
        try:
            process = subprocess.run(
                [executable, "compose", "--out", str(output), str(request)],
                capture_output=True,
                text=True,
                check=True,
            )
        except (OSError, subprocess.CalledProcessError) as exc:
            detail = exc.stderr.strip() if isinstance(exc, subprocess.CalledProcessError) else str(exc)
            raise RuntimeError(f"agent-compose failed to build the Goose bundle: {detail}") from exc
        manifests = sorted(output.glob("*/manifest.json"))
        if len(manifests) != 1:
            raise RuntimeError(
                "agent-compose did not produce exactly one bundle manifest "
                f"(found {len(manifests)}; output={process.stdout.strip()!r})"
            )
        return build_snapshot(
            manifests[0].parent,
            provider,
            repo,
            cwd,
            plugin_roots=plugin_roots,
            mcp_servers=read_mcporter_server_names(mcporter_path),
        )


def write_snapshot(path: Path, snapshot: dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(snapshot, indent=2) + "\n", encoding="utf-8")


def load_snapshot(path: Path) -> dict[str, object]:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"read Goose context snapshot {path}: {exc}") from exc
    if document.get("format") != FORMAT:
        raise RuntimeError(f"{path}: unsupported Goose context snapshot format")
    if document.get("lane") != {"harness": HARNESS, "role": ROLE, "intent": INTENT}:
        raise RuntimeError(f"{path}: snapshot is not the fixed Goose lane")
    if not isinstance(document.get("components"), list):
        raise RuntimeError(f"{path}: snapshot components must be an array")
    return document


def _component_kind_totals(snapshot: dict[str, object], eager: bool) -> list[tuple[str, int, int]]:
    totals: dict[str, list[int]] = defaultdict(lambda: [0, 0])
    for raw in snapshot["components"]:  # type: ignore[index]
        if not isinstance(raw, dict) or raw.get("eager") is not eager:
            continue
        kind = str(raw.get("kind"))
        totals[kind][0] += int(raw.get("bytes", 0))
        totals[kind][1] += int(raw.get("tokens", 0))
    return [(kind, values[0], values[1]) for kind, values in sorted(totals.items())]


def render_snapshot(snapshot: dict[str, object]) -> str:
    lane = snapshot["lane"]
    totals = snapshot["totals"]
    mcp = snapshot["mcp"]
    assert isinstance(lane, dict) and isinstance(totals, dict) and isinstance(mcp, dict)
    lines = [
        "Goose context baseline",
        f"  lane       {lane['role']} / {lane['intent']} / {lane['harness']}",
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
    for raw in snapshot["components"]:  # type: ignore[index]
        if not isinstance(raw, dict) or not isinstance(raw.get("id"), str):
            raise RuntimeError("snapshot contains a malformed component")
        out[str(raw["id"])] = raw
    return out


def render_delta(before: dict[str, object], after: dict[str, object]) -> str:
    """Render stable component and total changes for the same fixed lane."""
    for key in ("lane", "repository", "cwd", "tokenizer"):
        if before.get(key) != after.get(key):
            raise RuntimeError(f"cannot compare snapshots with different {key}")
    before_totals = before["totals"]
    after_totals = after["totals"]
    assert isinstance(before_totals, dict) and isinstance(after_totals, dict)
    lines = ["Goose context delta"]
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
