"""Collect bounded read-only Forgejo storage measurements through kubectl."""

from __future__ import annotations

import argparse
import subprocess
import sys
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from datetime import datetime, timezone


NAMESPACE = "forgejo"
FORGEJO_TARGET = "deployment/forgejo"
FORGEJO_DB_TARGET = "statefulset/forgejo-db"
MEASUREMENT_TIMEOUT_SECONDS = 120

RunCommand = Callable[..., subprocess.CompletedProcess[bytes]]


@dataclass(frozen=True)
class Measurement:
    title: str
    target: str
    argv: tuple[str, ...]


def _psql(sql: str) -> tuple[str, ...]:
    return ("psql", "-U", "forgejo", "-d", "forgejo", "-P", "pager=off", "-c", sql)


PACKAGE_BLOB_REFERENCES_SQL = (
    "WITH blob_refs AS ("
    "SELECT pb.id, pb.size, pb.created_unix, count(pf.id) AS reference_count "
    "FROM package_blob pb LEFT JOIN package_file pf ON pf.blob_id = pb.id "
    "GROUP BY pb.id, pb.size, pb.created_unix"
    ") SELECT count(*) AS total_blobs, "
    "pg_size_pretty(COALESCE(sum(size), 0)::bigint) AS total_blob_bytes, "
    "count(*) FILTER (WHERE reference_count > 0) AS referenced_blobs, "
    "pg_size_pretty(COALESCE(sum(size) FILTER (WHERE reference_count > 0), 0)::bigint) "
    "AS referenced_blob_bytes, "
    "count(*) FILTER (WHERE reference_count = 0) AS unreferenced_blobs, "
    "pg_size_pretty(COALESCE(sum(size) FILTER (WHERE reference_count = 0), 0)::bigint) "
    "AS unreferenced_blob_bytes, "
    "pg_size_pretty(COALESCE(sum(size) FILTER (WHERE reference_count = 0 "
    "AND created_unix < extract(epoch FROM now())::bigint - 86400), 0)::bigint) "
    "AS expired_unreferenced_blob_bytes FROM blob_refs;"
)

PACKAGE_OWNERSHIP_SQL = (
    "WITH owner_type_blobs AS ("
    "SELECT DISTINCT p.owner_id, p.type, pb.id, pb.size FROM package p "
    "JOIN package_version pv ON pv.package_id = p.id "
    "JOIN package_file pf ON pf.version_id = pv.id "
    "JOIN package_blob pb ON pb.id = pf.blob_id"
    ") SELECT u.name AS owner, otb.type, count(*) AS referenced_blobs, "
    "pg_size_pretty(sum(otb.size)::bigint) AS referenced_blob_bytes "
    "FROM owner_type_blobs otb JOIN \"user\" u ON u.id = otb.owner_id "
    "GROUP BY u.name, otb.type ORDER BY sum(otb.size) DESC LIMIT 50;"
)

LARGEST_PACKAGES_SQL = (
    "WITH package_blobs AS ("
    "SELECT DISTINCT p.id AS package_id, p.owner_id, p.type, p.name, "
    "pb.id AS blob_id, pb.size FROM package p "
    "JOIN package_version pv ON pv.package_id = p.id "
    "JOIN package_file pf ON pf.version_id = pv.id "
    "JOIN package_blob pb ON pb.id = pf.blob_id"
    "), version_counts AS ("
    "SELECT package_id, count(*) FILTER (WHERE NOT is_internal) AS external_versions, "
    "min(created_unix) FILTER (WHERE NOT is_internal) AS oldest_external_created_unix, "
    "max(created_unix) FILTER (WHERE NOT is_internal) AS newest_external_created_unix "
    "FROM package_version GROUP BY package_id"
    ") SELECT u.name AS owner, pb.type, pb.name AS package, vc.external_versions, "
    "vc.oldest_external_created_unix, vc.newest_external_created_unix, "
    "count(*) AS referenced_blobs, pg_size_pretty(sum(pb.size)::bigint) "
    "AS referenced_blob_bytes FROM package_blobs pb "
    "JOIN version_counts vc ON vc.package_id = pb.package_id "
    "JOIN \"user\" u ON u.id = pb.owner_id "
    "GROUP BY u.name, pb.type, pb.name, vc.external_versions, "
    "vc.oldest_external_created_unix, vc.newest_external_created_unix "
    "ORDER BY sum(pb.size) DESC LIMIT 50;"
)

PACKAGE_VERSION_AGE_SQL = (
    "SELECT u.name AS owner, p.type, count(DISTINCT p.id) AS packages, "
    "count(*) FILTER (WHERE NOT pv.is_internal) AS external_versions, "
    "count(*) FILTER (WHERE NOT pv.is_internal "
    "AND pv.created_unix < extract(epoch FROM now())::bigint - 86400) AS older_than_1d, "
    "count(*) FILTER (WHERE NOT pv.is_internal "
    "AND pv.created_unix < extract(epoch FROM now())::bigint - 604800) AS older_than_7d, "
    "count(*) FILTER (WHERE NOT pv.is_internal "
    "AND pv.created_unix < extract(epoch FROM now())::bigint - 2592000) AS older_than_30d, "
    "min(pv.created_unix) FILTER (WHERE NOT pv.is_internal) AS oldest_external_created_unix, "
    "max(pv.created_unix) FILTER (WHERE NOT pv.is_internal) AS newest_external_created_unix "
    "FROM package p JOIN package_version pv ON pv.package_id = p.id "
    "JOIN \"user\" u ON u.id = p.owner_id "
    "GROUP BY u.name, p.type ORDER BY external_versions DESC LIMIT 50;"
)

PACKAGE_CLEANUP_RULES_SQL = (
    "SELECT u.name AS owner, pcr.type, pcr.enabled, pcr.keep_count, "
    "pcr.keep_pattern, pcr.remove_days, pcr.remove_pattern, pcr.match_full_name "
    "FROM package_cleanup_rule pcr JOIN \"user\" u ON u.id = pcr.owner_id "
    "ORDER BY u.name, pcr.type;"
)

MEASUREMENTS = (
    Measurement(
        "Forgejo application root",
        FORGEJO_TARGET,
        ("sh", "-c", "set -o pipefail; du -xhd1 /var/lib/gitea | sort -h"),
    ),
    Measurement(
        "Forgejo managed data",
        FORGEJO_TARGET,
        ("sh", "-c", "set -o pipefail; du -xhd1 /var/lib/gitea/data | sort -h"),
    ),
    Measurement(
        "largest Forgejo package directories",
        FORGEJO_TARGET,
        (
            "sh",
            "-c",
            "set -o pipefail; du -xhd2 /var/lib/gitea/packages | sort -h | tail -n 100",
        ),
    ),
    Measurement(
        "largest Forgejo repository directories",
        FORGEJO_TARGET,
        (
            "sh",
            "-c",
            "set -o pipefail; du -xhd2 /var/lib/gitea/git/repositories "
            "| sort -h | tail -n 50",
        ),
    ),
    Measurement(
        "largest Forgejo Git packfiles (KiB)",
        FORGEJO_TARGET,
        (
            "sh",
            "-c",
            "set -o pipefail; find /var/lib/gitea/git/repositories -name '*.pack' "
            "-exec du -k {} + | sort -n | tail -n 50",
        ),
    ),
    Measurement(
        "Forgejo PostgreSQL database",
        FORGEJO_DB_TARGET,
        (
            "psql",
            "-U",
            "forgejo",
            "-d",
            "forgejo",
            "-c",
            "select pg_size_pretty(pg_database_size(current_database()));",
        ),
    ),
    Measurement(
        "Forgejo package blob references",
        FORGEJO_DB_TARGET,
        _psql(PACKAGE_BLOB_REFERENCES_SQL),
    ),
    Measurement(
        "Forgejo package ownership by referenced blobs",
        FORGEJO_DB_TARGET,
        _psql(PACKAGE_OWNERSHIP_SQL),
    ),
    Measurement(
        "largest Forgejo packages by referenced blobs",
        FORGEJO_DB_TARGET,
        _psql(LARGEST_PACKAGES_SQL),
    ),
    Measurement(
        "Forgejo external package version age",
        FORGEJO_DB_TARGET,
        _psql(PACKAGE_VERSION_AGE_SQL),
    ),
    Measurement(
        "Forgejo package cleanup rules",
        FORGEJO_DB_TARGET,
        _psql(PACKAGE_CLEANUP_RULES_SQL),
    ),
)


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(prog="aosguard ops forgejo-storage measure")
    return parser.parse_args(argv)


def _section(title: str) -> None:
    print(f"\n===== {title} =====", flush=True)


def _run_fixed(
    argv: Sequence[str],
    *,
    title: str,
    runner: RunCommand,
) -> bool:
    try:
        completed = runner(
            list(argv),
            check=False,
            timeout=MEASUREMENT_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired:
        print(
            f"measurement exceeded {MEASUREMENT_TIMEOUT_SECONDS} seconds: {title}",
            file=sys.stderr,
        )
        return False
    except OSError as exc:
        print(f"measurement could not start: {title}: {exc}", file=sys.stderr)
        return False

    if completed.returncode == 0:
        return True
    print(
        f"measurement failed with exit code {completed.returncode}: {title}",
        file=sys.stderr,
    )
    return False


def main(
    argv: list[str] | None = None,
    *,
    runner: RunCommand | None = None,
) -> int:
    _parse_args(argv)
    run = runner or subprocess.run
    complete = True

    _section("capture metadata")
    print(f"captured_at={datetime.now(timezone.utc).isoformat()}")
    complete &= _run_fixed(
        ("kubectl", "config", "current-context"),
        title="Kubernetes context",
        runner=run,
    )

    _section("Forgejo PVC and pod ownership")
    complete &= _run_fixed(
        ("kubectl", "-n", NAMESPACE, "get", "pvc,pods", "-o", "wide"),
        title="Forgejo PVC and pod ownership",
        runner=run,
    )

    for measurement in MEASUREMENTS:
        _section(measurement.title)
        complete &= _run_fixed(
            (
                "kubectl",
                "-n",
                NAMESPACE,
                "exec",
                measurement.target,
                "--",
                *measurement.argv,
            ),
            title=measurement.title,
            runner=run,
        )

    _section("result")
    if complete:
        print("all Forgejo storage measurements completed")
        return 0
    print("one or more Forgejo storage measurements failed or timed out", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
