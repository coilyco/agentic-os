"""Teable schema administration, with a read-back assertion on every write.

This instance reports success without doing the thing in several confirmed
ways, so no write here trusts its own response. Each mutating verb re-reads
through a separate request and asserts the stored object matches what was
sent, failing loudly when it does not. See docs/teable-admin.md.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from typing import Any

from agentic_os import shared_ssl_context


DEFAULT_BASE_URL = "http://teable:3000/api"

SELECT_TYPES = ("singleSelect", "multipleSelect")
RECORD_PAGE = 1000

# Teable's colour vocabulary, walked in order for an added choice that names none.
CHOICE_COLORS = [
    f"{hue}{shade}"
    for hue in ("blue", "cyan", "gray", "green", "orange", "pink", "purple", "red", "teal", "yellow")
    for shade in ("Light2", "Light1", "Bright", "", "Dark1")
]

EXIT_CODES = {
    "authorization_failure": 77,
    "api_failure": 69,
    "api_contract": 70,
    "readback_mismatch": 65,
    "refused": 64,
    "invalid_identifier": 64,
}

# Refused by name rather than by absence: an absent verb reads as
# "unimplemented" as readily as "refused". See docs for the reasoning.
REFUSALS = {
    "convert-field": (
        "convert is the one verb that destroys data while reporting the opposite: it emptied "
        "all 6,536 values in a column it declared required, returning 200 with notNull true. "
        "Do it in the Teable UI with an export in hand, and read the column back before "
        "trusting the response. To rename or add select choices, use edit-choices, which "
        "snapshots every value first and proves each one survived."
    ),
    "delete-table": (
        "Teable has no archive verb for a table, so a delete is unrecoverable outside a restic "
        "PVC restore. Rename the table in the Teable UI instead, which is reversible."
    ),
}


class TeableAdminError(RuntimeError):
    """A typed failure rendered without disturbing stdout."""

    def __init__(self, kind: str, message: str) -> None:
        super().__init__(message)
        self.kind = kind
        self.exit_code = EXIT_CODES.get(kind, EXIT_CODES["api_failure"])


class TeableAPI:
    """The subset of Teable's REST API schema administration needs."""

    def __init__(self, base_url: str, token: str) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token

    def request(
        self,
        method: str,
        path: str,
        query: dict[str, Any] | None = None,
        body: Any | None = None,
    ) -> Any:
        url = self.base_url + path
        if query:
            url += "?" + urllib.parse.urlencode(query, doseq=True)
        data = None
        headers = {"Authorization": f"Bearer {self.token}", "Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(  # noqa: S310 - operator-supplied base
                request, context=shared_ssl_context()
            ) as response:
                raw = response.read()
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", "replace").strip()
            raise TeableAdminError(
                "api_failure", f"{method} {path} -> {exc.code}: {detail}"
            ) from exc
        except urllib.error.URLError as exc:
            raise TeableAdminError("api_failure", f"{method} {path}: {exc.reason}") from exc
        if not raw:
            return None
        try:
            return json.loads(raw)
        except json.JSONDecodeError as exc:
            raise TeableAdminError(
                "api_contract", f"{method} {path}: response was not JSON"
            ) from exc

    def list_fields(self, table_id: str) -> list[dict[str, Any]]:
        fields = self.request("GET", f"/table/{table_id}/field")
        if not isinstance(fields, list):
            raise TeableAdminError("api_contract", "field list was not an array")
        return fields

    def field_values(self, table_id: str, field_id: str) -> dict[str, Any]:
        """Every record's value for one field, keyed by record id."""
        values: dict[str, Any] = {}
        skip = 0
        while True:
            page = self.request(
                "GET",
                f"/table/{table_id}/record",
                query={
                    "fieldKeyType": "id",
                    "projection": [field_id],
                    "take": RECORD_PAGE,
                    "skip": skip,
                },
            )
            records = page.get("records") if isinstance(page, dict) else None
            if not isinstance(records, list):
                raise TeableAdminError("api_contract", "record page carried no records array")
            for record in records:
                values[record["id"]] = (record.get("fields") or {}).get(field_id)
            if len(records) < RECORD_PAGE:
                return values
            skip += RECORD_PAGE

    def list_tables(self, base_id: str) -> list[dict[str, Any]]:
        tables = self.request("GET", f"/base/{base_id}/table")
        if not isinstance(tables, list):
            raise TeableAdminError("api_contract", "table list was not an array")
        return tables


def _instant(value: Any) -> datetime | None:
    """A date-ish string as a UTC instant, or None when it is not one."""
    if not isinstance(value, str):
        return None
    try:
        parsed = datetime.fromisoformat(value.strip().replace("Z", "+00:00"))
    except ValueError:
        return None
    return parsed if parsed.tzinfo else parsed.replace(tzinfo=timezone.utc)


def survived(want: Any, stored: Any) -> bool:
    """Whether `stored` carries everything `want` asked for.

    Subset over dicts and positional over lists, so a discarded key or a
    reordering still fails while server-added keys do not. See the normalising
    cases in docs/pre-commit-hygiene.md.
    """
    if isinstance(want, dict):
        return isinstance(stored, dict) and all(
            key in stored and survived(value, stored[key]) for key, value in want.items()
        )
    if isinstance(want, list):
        if not isinstance(stored, list) or len(want) != len(stored):
            return False
        return all(survived(w, s) for w, s in zip(want, stored))
    want_at, stored_at = _instant(want), _instant(stored)
    if want_at is not None and stored_at is not None:
        return want_at == stored_at
    return want == stored


def mismatches(requested: dict[str, Any], stored: dict[str, Any]) -> list[str]:
    """Every requested property that did not survive the round trip.

    This is the whole of the create defect: unknown properties are accepted
    and silently discarded, so a field asked for with five properties can be
    stored with three and return 200 either way. Only a key-by-key diff
    against a re-read finds the two that vanished.

    Compares on what was asked for rather than byte-equality, because this
    instance normalises what it stores: a date gains a time, a link gains its
    title, a choice gains a server id. Those are representation rather than
    loss, and failing on them trained the operator to retry a write that had
    already landed (agentic-os#6929, #7644).
    """
    problems = []
    for key, want in requested.items():
        if key not in stored:
            problems.append(f"{key}: requested, absent from the read-back")
        elif not survived(want, stored[key]):
            problems.append(f"{key}: requested {want!r}, stored {stored[key]!r}")
    return problems


def create_field(api: TeableAPI, table_id: str, spec: dict[str, Any]) -> dict[str, Any]:
    """Create one field, then prove it through an independent read."""
    created = api.request("POST", f"/table/{table_id}/field", body=spec)
    if not isinstance(created, dict) or not created.get("id"):
        raise TeableAdminError(
            "api_contract", "create returned no field id, so it cannot be read back"
        )
    field_id = created["id"]
    # A separate request on purpose. The create response is the thing under
    # suspicion, so it is never the evidence.
    stored = next((f for f in api.list_fields(table_id) if f.get("id") == field_id), None)
    if stored is None:
        raise TeableAdminError(
            "readback_mismatch",
            f"the tracker reported field {field_id} created, and a fresh field list does not carry it",
        )
    problems = mismatches(spec, stored)
    if problems:
        raise TeableAdminError(
            "readback_mismatch",
            f"field {field_id} exists and does not match what was requested:\n  "
            + "\n  ".join(problems)
            + "\nThe field exists. Read it back with list-fields before creating it "
            + "again: a second create leaves a duplicate column only the UI can remove.",
        )
    return stored


def create_table(api: TeableAPI, base_id: str, spec: dict[str, Any]) -> dict[str, Any]:
    """Create one table, then prove it through an independent read."""
    created = api.request("POST", f"/base/{base_id}/table", body=spec)
    if not isinstance(created, dict) or not created.get("id"):
        raise TeableAdminError(
            "api_contract", "create returned no table id, so it cannot be read back"
        )
    table_id = created["id"]
    stored = next((t for t in api.list_tables(base_id) if t.get("id") == table_id), None)
    if stored is None:
        raise TeableAdminError(
            "readback_mismatch",
            f"the tracker reported table {table_id} created, and a fresh table list does not carry it",
        )
    problems = mismatches({k: v for k, v in spec.items() if k != "fields"}, stored)
    if problems:
        raise TeableAdminError(
            "readback_mismatch",
            f"table {table_id} exists and does not match what was requested:\n  "
            + "\n  ".join(problems),
        )
    return stored


def _resolve_field(fields: list[dict[str, Any]], ref: str) -> dict[str, Any]:
    matches = [f for f in fields if ref in (f.get("id"), f.get("name"))]
    if len(matches) != 1:
        raise TeableAdminError("invalid_identifier", f"no single field has id or name {ref!r}")
    return matches[0]


def plan_choices(
    field: dict[str, Any], renames: dict[str, str], additions: list[str]
) -> list[dict[str, Any]]:
    """The whole new choice list: every existing id kept, renamed in place, additions appended.

    Teable deletes a choice, and strips it from every record, when its id is absent
    from a convert, so nothing here can drop one.
    """
    if field.get("type") not in SELECT_TYPES:
        raise TeableAdminError(
            "refused", f"{field.get('name')!r} is {field.get('type')}, not a select field"
        )
    choices = (field.get("options") or {}).get("choices") or []
    names: list[str] = [str(c["name"]) for c in choices]
    if not renames and not additions:
        raise TeableAdminError("invalid_identifier", "nothing to do: pass --rename or --add")
    for old in renames:
        if old not in names:
            raise TeableAdminError("invalid_identifier", f"no choice named {old!r} to rename")
    final: list[str] = [renames.get(n, n) for n in names] + list(additions)
    duplicates = sorted({n for n in final if final.count(n) > 1})
    if duplicates:
        raise TeableAdminError(
            "invalid_identifier", f"the result would carry duplicate choices: {', '.join(duplicates)}"
        )
    used = {c.get("color") for c in choices}
    spare = [c for c in CHOICE_COLORS if c not in used] or CHOICE_COLORS
    planned = [{**c, "name": renames.get(c["name"], c["name"])} for c in choices]
    planned += [{"name": n, "color": spare[i % len(spare)]} for i, n in enumerate(additions)]
    return planned


def _expected(value: Any, renames: dict[str, str]) -> Any:
    if isinstance(value, list):
        return sorted(renames.get(str(v), str(v)) for v in value)
    return renames.get(value, value) if value is not None else None


def _normalised(value: Any) -> Any:
    return sorted(value) if isinstance(value, list) else value


def edit_choices(
    api: TeableAPI,
    table_id: str,
    field_ref: str,
    renames: dict[str, str],
    additions: list[str],
    dry_run: bool = False,
) -> dict[str, Any]:
    """Rename or add select choices, then prove no record value was lost.

    The write is the convert endpoint, the one that once emptied 6,536 values while
    returning 200. So every record's value is snapshotted to disk first, and re-read
    afterwards against the same snapshot mapped through the renames.
    """
    field = _resolve_field(api.list_fields(table_id), field_ref)
    planned = plan_choices(field, renames, additions)
    before = api.field_values(table_id, field["id"])
    # A projection defect once returned every value empty with a 200. An all-empty
    # snapshot would then "prove" an emptied column, so it proves nothing.
    if before and all(v in (None, [], "") for v in before.values()):
        raise TeableAdminError(
            "api_contract",
            f"all {len(before)} records read back empty for {field['name']!r}, "
            "so the snapshot cannot prove preservation; nothing was written",
        )
    in_use = {
        v for value in before.values() for v in (value if isinstance(value, list) else [value]) if v
    }
    summary = {
        "field": field["id"],
        "records": len(before),
        "renamed_in_use": sorted(old for old in renames if old in in_use),
        "choices": [c["name"] for c in planned],
    }
    if dry_run:
        return {"dry_run": True, **summary}

    with tempfile.NamedTemporaryFile(
        "w", prefix=f"teable-choices-{table_id}-{field['id']}-", suffix=".json", delete=False
    ) as handle:
        json.dump({"field": field, "values": before}, handle, ensure_ascii=False)
        snapshot = handle.name
    print(f"teable-admin: snapshot of {len(before)} values at {snapshot}", file=sys.stderr)

    options = {**(field.get("options") or {}), "choices": planned}
    api.request(
        "PUT",
        f"/table/{table_id}/field/{field['id']}/convert",
        body={"type": field["type"], "options": options},
    )

    stored = _resolve_field(api.list_fields(table_id), field["id"])
    problems = mismatches(
        {k: field[k] for k in ("name", "type", "dbFieldName", "notNull") if k in field}, stored
    )
    stored_choices = (stored.get("options") or {}).get("choices") or []
    if [c.get("name") for c in stored_choices] != [c["name"] for c in planned]:
        problems.append(
            f"choices: requested {[c['name'] for c in planned]}, "
            f"stored {[c.get('name') for c in stored_choices]}"
        )
    after = api.field_values(table_id, field["id"])
    lost = [rid for rid in before if rid not in after]
    changed = [
        rid
        for rid, value in before.items()
        if rid in after and _normalised(after[rid]) != _normalised(_expected(value, renames))
    ]
    if lost:
        problems.append(f"{len(lost)} records missing after the write, first {lost[:5]}")
    if changed:
        problems.append(
            f"{len(changed)} record values differ from the renamed snapshot, first {changed[:5]}"
        )
    if problems:
        raise TeableAdminError(
            "readback_mismatch",
            "the convert did not store as requested:\n  "
            + "\n  ".join(problems)
            + f"\nEvery prior value is in {snapshot}. A concurrent record edit also reads as a "
            + "difference, so check the named records before restoring anything.",
        )
    return {**summary, "snapshot": snapshot}


def _parse_renames(pairs: list[str]) -> dict[str, str]:
    renames: dict[str, str] = {}
    for pair in pairs:
        old, sep, new = pair.partition("=")
        if not sep or not old or not new:
            raise TeableAdminError("invalid_identifier", f"--rename takes OLD=NEW, got {pair!r}")
        if old in renames:
            raise TeableAdminError("invalid_identifier", f"{old!r} is renamed twice")
        renames[old] = new
    return renames


def _read_spec(path: str) -> dict[str, Any]:
    try:
        with open(path, encoding="utf-8") as handle:
            spec = json.load(handle)
    except OSError as exc:
        raise TeableAdminError("invalid_identifier", f"read spec {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise TeableAdminError("invalid_identifier", f"parse spec {path}: {exc}") from exc
    if not isinstance(spec, dict):
        raise TeableAdminError("invalid_identifier", f"{path}: spec must be a JSON object")
    return spec


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="teable-admin",
        description="Teable schema administration with a read-back assertion on every write.",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    create = sub.add_parser("create-field", help="create a field and prove it stored as requested")
    create.add_argument("table", help="table id")
    create.add_argument("--spec", required=True, help="path to a JSON file of field properties")

    listing = sub.add_parser("list-fields", help="list a table's fields")
    listing.add_argument("table", help="table id")

    table = sub.add_parser("create-table", help="create a table and prove it stored as requested")
    table.add_argument("base", help="base id")
    table.add_argument("--spec", required=True, help="path to a JSON file of table properties")

    describe = sub.add_parser("describe-base", help="list the tables in a base")
    describe.add_argument("base", help="base id")

    choices = sub.add_parser(
        "edit-choices",
        help="rename or add select choices, carrying every record value, and prove none was lost",
    )
    choices.add_argument("table", help="table id")
    choices.add_argument("field", help="field id or exact field name")
    choices.add_argument("--rename", action="append", default=[], metavar="OLD=NEW")
    choices.add_argument("--add", action="append", default=[], metavar="NAME")
    choices.add_argument("--dry-run", action="store_true", help="plan and count, write nothing")

    for refused in REFUSALS:
        sub.add_parser(refused, help="NOT AVAILABLE: refused by policy, run it for the reason")

    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)

    if args.command in REFUSALS:
        print(
            f"teable-admin-error: refused: {args.command} is not available. {REFUSALS[args.command]}",
            file=sys.stderr,
        )
        return EXIT_CODES["refused"]

    token = os.environ.get("TEABLE_API_TOKEN")
    if not token:
        print(
            "teable-admin-error: authorization_failure: TEABLE_API_TOKEN is required",
            file=sys.stderr,
        )
        return EXIT_CODES["authorization_failure"]
    api = TeableAPI(os.environ.get("TEABLE_BASE_URL", DEFAULT_BASE_URL), token)

    try:
        if args.command == "create-field":
            result: Any = create_field(api, args.table, _read_spec(args.spec))
        elif args.command == "list-fields":
            result = api.list_fields(args.table)
        elif args.command == "edit-choices":
            result = edit_choices(
                api, args.table, args.field, _parse_renames(args.rename), args.add, args.dry_run
            )
        elif args.command == "create-table":
            result = create_table(api, args.base, _read_spec(args.spec))
        else:
            result = api.list_tables(args.base)
    except TeableAdminError as exc:
        print(f"teable-admin-error: {exc.kind}: {exc}", file=sys.stderr)
        return exc.exit_code

    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
