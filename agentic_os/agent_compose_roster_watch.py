"""Watch whether the release image ships the roles agent-compose has now.

The dev-base image bakes one agent-compose release, pinned by hand. A role added or
dropped upstream reaches no downstream image until that pin moves and the image is
republished, and nothing failed while it lagged. This reads the live roles on both
sides through agent-compose itself and exits non-zero when they differ. See
docs/build-file-headers.md.
"""

from __future__ import annotations

import argparse
import dataclasses
import hashlib
import json
import platform
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
from collections.abc import Sequence
from pathlib import Path

from agentic_os import shared_ssl_context


DEFAULT_BASE_URL = "https://forgejo.coilysiren.me"
DEFAULT_REPOSITORY = "coilyco/agent-compose"
DEFAULT_SHIPPED_ROSTER = Path("/usr/local/share/agent-compose/roster")
ROSTER_ASSET = "agent-compose-roster.tar.gz"
SUMS_ASSET = "SHA256SUMS"
SUPPORTED_BINARIES = frozenset({"linux-amd64", "linux-arm64", "darwin-arm64"})
MACHINES = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}
FETCH_ATTEMPTS = 3
FETCH_TIMEOUT_SECONDS = 60

EXIT_DRIFT = 1
# EX_UNAVAILABLE: the far side could not be read.
EXIT_UNCOMPARABLE = 69


class WatchError(RuntimeError):
    """The comparison could not be made, which must never read as agreement."""


@dataclasses.dataclass(frozen=True)
class RoleDrift:
    missing: tuple[str, ...]
    dropped: tuple[str, ...]

    @property
    def in_sync(self) -> bool:
        return not self.missing and not self.dropped


def compare_roles(shipped: Sequence[str], latest: Sequence[str]) -> RoleDrift:
    """Membership only, since role_order is display order and a consumer selects by slug.

    Prose drift is left alone on purpose, see docs/build-file-headers.md.
    """
    return RoleDrift(
        missing=tuple(sorted(set(latest) - set(shipped))),
        dropped=tuple(sorted(set(shipped) - set(latest))),
    )


def _archived(entry: object) -> bool:
    return isinstance(entry, dict) and bool(entry.get("archived"))


def live_roles(binary: Path, roster_root: Path) -> list[str]:
    """Ask agent-compose which roles a roster holds live instead of parsing its layout.

    A consumer bakes only roles the roster has not archived, and that flag moves without
    role_order changing, so role_order alone would miss a retirement.
    """
    with tempfile.TemporaryDirectory() as out:
        run = subprocess.run(
            [str(binary), "roster", "--person-source", str(roster_root), "--out", out],
            capture_output=True,
            text=True,
            check=False,
        )
        if run.returncode != 0:
            raise WatchError(f"{binary.name} roster failed on {roster_root}: {run.stderr.strip()}")
        try:
            person = json.loads((Path(out) / "person.json").read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise WatchError(f"no readable person.json from {roster_root}: {exc}") from exc
    roles = person.get("role_order") if isinstance(person, dict) else None
    meta = person.get("roles") if isinstance(person, dict) else None
    # Two empty reads compare equal, which is how an unplumbed read passes as agreement.
    if not isinstance(roles, list) or not roles or not all(isinstance(r, str) for r in roles):
        raise WatchError(f"person.json from {roster_root} has no usable role_order")
    if not isinstance(meta, dict):
        raise WatchError(f"person.json from {roster_root} has no roles map")
    live = [role for role in roles if not _archived(meta.get(role))]
    if not live:
        raise WatchError(f"person.json from {roster_root} has no live role")
    return live


def _fetch(url: str) -> bytes:
    last: Exception | None = None
    for attempt in range(1, FETCH_ATTEMPTS + 1):
        try:
            with urllib.request.urlopen(
                urllib.request.Request(url),
                timeout=FETCH_TIMEOUT_SECONDS,
                context=shared_ssl_context(),
            ) as response:
                return response.read()
        except (urllib.error.URLError, TimeoutError) as exc:
            last = exc
            if attempt < FETCH_ATTEMPTS:
                time.sleep(attempt)
    raise WatchError(f"could not fetch {url}: {last}")


def latest_tag(base_url: str, repository: str) -> str:
    body = _fetch(f"{base_url}/api/v1/repos/{repository}/releases/latest")
    try:
        tag = json.loads(body)["tag_name"]
    except (json.JSONDecodeError, KeyError, TypeError) as exc:
        raise WatchError(f"latest release of {repository} has no tag_name: {exc}") from exc
    if not isinstance(tag, str) or not tag:
        raise WatchError(f"latest release of {repository} has an unusable tag_name")
    return tag


def binary_asset() -> str:
    machine = MACHINES.get(platform.machine().lower(), platform.machine().lower())
    target = f"{platform.system().lower()}-{machine}"
    if target not in SUPPORTED_BINARIES:
        raise WatchError(f"agent-compose publishes no binary for {target}")
    return f"agent-compose-{target}"


def _checksums(text: str) -> dict[str, str]:
    sums: dict[str, str] = {}
    for line in text.splitlines():
        fields = line.split()
        if len(fields) == 2:
            sums[fields[1].lstrip("*")] = fields[0]
    return sums


def fetch_release(base_url: str, repository: str, tag: str, dest: Path) -> tuple[Path, Path]:
    """Download one release's binary and roster, each verified against its own SHA256SUMS."""
    asset = binary_asset()
    base = f"{base_url}/{repository}/releases/download/{tag}"
    sums = _checksums(_fetch(f"{base}/{SUMS_ASSET}").decode("utf-8", errors="replace"))
    blobs: dict[str, bytes] = {}
    for name in (asset, ROSTER_ASSET):
        blob = _fetch(f"{base}/{name}")
        got = hashlib.sha256(blob).hexdigest()
        if sums.get(name) != got:
            raise WatchError(f"{name} in {tag} has sha256 {got}, SHA256SUMS lists {sums.get(name)}")
        blobs[name] = blob
    binary = dest / asset
    binary.write_bytes(blobs[asset])
    binary.chmod(0o755)
    archive = dest / ROSTER_ASSET
    archive.write_bytes(blobs[ROSTER_ASSET])
    with tarfile.open(archive) as bundle:
        bundle.extractall(dest, filter="data")
    return binary, dest / "roster"


def check(
    shipped_root: Path, base_url: str, repository: str, workdir: Path
) -> tuple[str, list[str], RoleDrift]:
    """One binary reads both sides, since a newer agent-compose reads an older roster."""
    if not shipped_root.is_dir():
        raise WatchError(f"no shipped roster at {shipped_root}, run this inside agentic-os:release")
    tag = latest_tag(base_url, repository)
    binary, latest_root = fetch_release(base_url, repository, tag, workdir)
    latest = live_roles(binary, latest_root)
    shipped = live_roles(binary, shipped_root)
    return tag, latest, compare_roles(shipped, latest)


def drift_message(tag: str, drift: RoleDrift) -> str:
    lines = [f"agentic-os:release ships a roster that lags agent-compose {tag}."]
    if drift.missing:
        lines.append(f"  missing from the image: {', '.join(drift.missing)}")
    if drift.dropped:
        lines.append(f"  baked but dropped or archived upstream: {', '.join(drift.dropped)}")
    lines.append(
        f"Advance AGENT_COMPOSE_VERSION to {tag.removeprefix('v')} in "
        "docker/dev-base/full/Dockerfile and the role count in docker/dev-base/verify-common.sh, "
        "then let the push publish. See docs/dev-base-image.md."
    )
    return "\n".join(lines)


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").split("\n", 1)[0])
    parser.add_argument("--shipped-roster", type=Path, default=DEFAULT_SHIPPED_ROSTER)
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL)
    parser.add_argument("--repository", default=DEFAULT_REPOSITORY)
    args = parser.parse_args(argv)
    try:
        with tempfile.TemporaryDirectory() as workdir:
            tag, latest, drift = check(
                args.shipped_roster, args.base_url, args.repository, Path(workdir)
            )
    except WatchError as exc:
        print(f"agent-compose-roster-watch: could not compare: {exc}", file=sys.stderr)
        return EXIT_UNCOMPARABLE
    if drift.in_sync:
        print(f"release image roster matches agent-compose {tag} ({len(latest)} live roles)")
        return 0
    print(drift_message(tag, drift), file=sys.stderr)
    return EXIT_DRIFT


if __name__ == "__main__":
    sys.exit(main())
