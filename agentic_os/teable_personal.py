"""Teable records over one pinned base, with a read-back assertion on every write.

The schema surface is `agentic_os.teable_admin`; this module reaches records
only, so the personal PAT never has a module that can create or convert a
field. The base is pinned from the environment rather than taken as an
argument, because a token scoped more broadly than intended would otherwise
reach whatever base a caller named. See references/teable-personal.md.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Any

from agentic_os.teable_admin import (
    DEFAULT_BASE_URL,
    EXIT_CODES,
    TeableAPI,
    TeableAdminError as TeableError,
    mismatches,
)

# Hand-authored rows, no undo, nothing upstream to re-derive them from. Named
# rather than absent, because absence reads as unimplemented just as readily.
REFUSALS = {
    "delete-record": (
        "a connections row is hand-authored data with no upstream to rebuild it from, and "
        "Teable offers no undo or archive for a deleted record, so the row is gone outside a "
        "restic PVC restore. Clear the fields with edit-record, or delete it in the Teable UI "
        "with the row in front of you."
    ),
}


class PinnedBase:
    """The one base this surface may touch, and the tables inside it.

    Every table id a caller supplies is checked against this base before any
    request carrying it goes out. The check is the pin: the guardfile resolves
    the base id from SSM, so the caller never names a base at all.
    """

    def __init__(self, api: TeableAPI, base_id: str) -> None:
        self.api = api
        self.base_id = base_id
        self._tables: list[dict[str, Any]] | None = None

    def tables(self) -> list[dict[str, Any]]:
        if self._tables is None:
            self._tables = self.api.list_tables(self.base_id)
        return self._tables

    def check(self, table_id: str) -> str:
        if any(table.get("id") == table_id for table in self.tables()):
            return table_id
        raise TeableError(
            "refused",
            f"table {table_id} is not in the pinned base. This surface reaches one base and "
            f"cannot be pointed at another; use aosguard ops teable-admin for other bases.",
        )


def list_records(
    api: TeableAPI,
    table_id: str,
    take: int | None = None,
    skip: int | None = None,
    view_id: str | None = None,
    projection: list[str] | None = None,
    search: str | None = None,
    field_key_type: str = "name",
) -> Any:
    query: dict[str, Any] = {"fieldKeyType": field_key_type}
    if take is not None:
        query["take"] = take
    if skip is not None:
        query["skip"] = skip
    if view_id:
        query["viewId"] = view_id
    # Declared as an array and sent as repeated query keys, which is the shape
    # Teable wants. An unprojected read pulls every field of every row.
    if projection:
        query["projection"] = projection
    if search:
        query["search"] = search
    return api.request("GET", f"/table/{table_id}/record", query=query)


def get_record(
    api: TeableAPI,
    table_id: str,
    record_id: str,
    projection: list[str] | None = None,
    field_key_type: str = "name",
) -> Any:
    query: dict[str, Any] = {"fieldKeyType": field_key_type}
    if projection:
        query["projection"] = projection
    return api.request("GET", f"/table/{table_id}/record/{record_id}", query=query)


def _stored_fields(api: TeableAPI, table_id: str, record_id: str, field_key_type: str) -> dict:
    """Re-read one record and return its fields, through a separate request."""
    stored = get_record(api, table_id, record_id, field_key_type=field_key_type)
    if not isinstance(stored, dict):
        raise TeableError("api_contract", f"record {record_id} read back as a non-object")
    fields = stored.get("fields")
    if not isinstance(fields, dict):
        raise TeableError("api_contract", f"record {record_id} read back with no fields object")
    return fields


def create_record(
    api: TeableAPI, table_id: str, fields: dict[str, Any], field_key_type: str = "name"
) -> dict[str, Any]:
    """Create one record, then prove it through an independent read."""
    body = {"fieldKeyType": field_key_type, "records": [{"fields": fields}]}
    created = api.request("POST", f"/table/{table_id}/record", body=body)
    records = created.get("records") if isinstance(created, dict) else created
    if not isinstance(records, list) or not records or not isinstance(records[0], dict):
        raise TeableError("api_contract", "create returned no record, so it cannot be read back")
    record_id = records[0].get("id")
    if not record_id:
        raise TeableError("api_contract", "create returned no record id, so it cannot be read back")
    # The create response is the thing under suspicion, so it is never the evidence.
    problems = mismatches(fields, _stored_fields(api, table_id, record_id, field_key_type))
    if problems:
        raise TeableError(
            "readback_mismatch",
            f"record {record_id} exists and does not match what was requested:\n  "
            + "\n  ".join(problems)
            + f"\nNothing was rolled back: there is no delete-record verb. Fix it with "
            f"edit-record {table_id} {record_id}, or remove it in the Teable UI.",
        )
    return get_record(api, table_id, record_id, field_key_type=field_key_type)


def edit_record(
    api: TeableAPI,
    table_id: str,
    record_id: str,
    fields: dict[str, Any],
    field_key_type: str = "name",
) -> dict[str, Any]:
    """Update one record, then prove it through an independent read."""
    body = {"fieldKeyType": field_key_type, "record": {"fields": fields}}
    api.request("PATCH", f"/table/{table_id}/record/{record_id}", body=body)
    problems = mismatches(fields, _stored_fields(api, table_id, record_id, field_key_type))
    if problems:
        raise TeableError(
            "readback_mismatch",
            f"record {record_id} was reported updated and did not store what was requested:\n  "
            + "\n  ".join(problems)
            + "\nThe previous values are not recoverable from here; the record is in the "
            "Teable UI with its own revision history.",
        )
    return get_record(api, table_id, record_id, field_key_type=field_key_type)


def _read_spec(path: str) -> dict[str, Any]:
    try:
        with open(path, encoding="utf-8") as handle:
            spec = json.load(handle)
    except OSError as exc:
        raise TeableError("invalid_identifier", f"read spec {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise TeableError("invalid_identifier", f"parse spec {path}: {exc}") from exc
    if not isinstance(spec, dict):
        raise TeableError("invalid_identifier", f"{path}: spec must be a JSON object of fields")
    return spec


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="teable-personal",
        description="Teable records over one pinned base, read-back asserted on every write.",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("list-base", help="show the pinned base")
    sub.add_parser("list-base-table", help="list the tables in the pinned base")

    for verb, helptext in (
        ("list-table-field", "list a table's fields"),
        ("list-table-view", "list a table's views"),
    ):
        listing = sub.add_parser(verb, help=helptext)
        listing.add_argument("table", help="table id, which must be in the pinned base")

    records = sub.add_parser("list-record", help="list records, projected")
    records.add_argument("table", help="table id, which must be in the pinned base")
    records.add_argument("--take", type=int, help="page size")
    records.add_argument("--skip", type=int, help="records to skip")
    records.add_argument("--view", help="view id to read through")
    records.add_argument("--projection", action="append", help="field to return; repeatable")
    records.add_argument("--search", help="search term")

    one = sub.add_parser("get-record", help="read one record")
    one.add_argument("table", help="table id, which must be in the pinned base")
    one.add_argument("record", help="record id")
    one.add_argument("--projection", action="append", help="field to return; repeatable")

    create = sub.add_parser("create-record", help="create a record and prove it stored")
    create.add_argument("table", help="table id, which must be in the pinned base")
    create.add_argument("--spec", required=True, help="path to a JSON object of field values")

    edit = sub.add_parser("edit-record", help="update a record and prove it stored")
    edit.add_argument("table", help="table id, which must be in the pinned base")
    edit.add_argument("record", help="record id")
    edit.add_argument("--spec", required=True, help="path to a JSON object of field values")

    for verb in ("list-record", "get-record", "create-record", "edit-record"):
        sub.choices[verb].add_argument(
            "--field-key-type", default="name", choices=("name", "id"), help="field key type"
        )

    for refused in REFUSALS:
        sub.add_parser(refused, help="NOT AVAILABLE: refused by policy, run it for the reason")

    return parser.parse_args(argv)


def _dispatch(args: argparse.Namespace, api: TeableAPI, pinned: PinnedBase) -> Any:
    key = getattr(args, "field_key_type", "name")
    if args.command == "list-base":
        return api.request("GET", f"/base/{pinned.base_id}")
    if args.command == "list-base-table":
        return pinned.tables()
    if args.command == "list-table-field":
        return api.list_fields(pinned.check(args.table))
    if args.command == "list-table-view":
        return api.request("GET", f"/table/{pinned.check(args.table)}/view")
    if args.command == "list-record":
        return list_records(
            api,
            pinned.check(args.table),
            take=args.take,
            skip=args.skip,
            view_id=args.view,
            projection=args.projection,
            search=args.search,
            field_key_type=key,
        )
    if args.command == "get-record":
        return get_record(api, pinned.check(args.table), args.record, args.projection, key)
    if args.command == "create-record":
        return create_record(api, pinned.check(args.table), _read_spec(args.spec), key)
    return edit_record(api, pinned.check(args.table), args.record, _read_spec(args.spec), key)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)

    if args.command in REFUSALS:
        print(
            f"teable-personal-error: refused: {args.command} is not available. "
            f"{REFUSALS[args.command]}",
            file=sys.stderr,
        )
        return EXIT_CODES["refused"]

    token = os.environ.get("TEABLE_API_TOKEN")
    if not token:
        print(
            "teable-personal-error: authorization_failure: TEABLE_API_TOKEN is required",
            file=sys.stderr,
        )
        return EXIT_CODES["authorization_failure"]
    base_id = os.environ.get("TEABLE_BASE_ID")
    if not base_id:
        print(
            "teable-personal-error: invalid_identifier: TEABLE_BASE_ID is required; the "
            "guardfile resolves it from /coilysiren/teable/connections-base-id",
            file=sys.stderr,
        )
        return EXIT_CODES["invalid_identifier"]

    api = TeableAPI(os.environ.get("TEABLE_BASE_URL", DEFAULT_BASE_URL), token)
    try:
        result = _dispatch(args, api, PinnedBase(api, base_id))
    except TeableError as exc:
        print(f"teable-personal-error: {exc.kind}: {exc}", file=sys.stderr)
        return exc.exit_code

    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
