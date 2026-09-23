"""Each test pins a confirmed Teable defect rather than a happy path."""

from __future__ import annotations

import io
import json
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Callable
from email.message import Message

import pytest

from agentic_os import teable_admin as admin

EXIT_AUTH = admin.EXIT_CODES["authorization_failure"]


class FakeResponse(io.BytesIO):
    """Enough of an http response for urlopen's context-manager use."""

    def __enter__(self) -> "FakeResponse":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()


def _json_response(payload: object) -> FakeResponse:
    return FakeResponse(json.dumps(payload).encode("utf-8"))


def _install(
    monkeypatch: pytest.MonkeyPatch,
    handler: Callable[[urllib.request.Request], FakeResponse],
) -> tuple[admin.TeableAPI, list[urllib.request.Request]]:
    calls: list[urllib.request.Request] = []

    def fake_urlopen(request: urllib.request.Request, **_: object) -> FakeResponse:
        calls.append(request)
        return handler(request)

    monkeypatch.setattr(admin.urllib.request, "urlopen", fake_urlopen)
    return admin.TeableAPI(base_url="http://teable.example/api", token="secret"), calls


# --- create-field, against the silent-discard defect ------------------------


def test_create_field_refuses_when_a_requested_property_is_discarded(monkeypatch):
    """POST /field accepts unknown properties and stores them nowhere."""
    stored = {"id": "fld1", "name": "priority", "type": "singleSelect"}

    def handler(request):
        if request.get_method() == "POST":
            return _json_response(stored)
        return _json_response([stored])

    api, _ = _install(monkeypatch, handler)

    with pytest.raises(admin.TeableAdminError) as caught:
        admin.create_field(
            api, "tbl1", {"name": "priority", "type": "singleSelect", "notNull": True}
        )

    assert caught.value.kind == "readback_mismatch"
    assert "notNull" in str(caught.value)
    # The field stayed and a blind retry duplicates it, so the refusal says both.
    assert "exists" in str(caught.value)
    assert "list-fields" in str(caught.value) and "duplicate" in str(caught.value)


def test_create_field_succeeds_when_every_property_survives(monkeypatch):
    stored = {"id": "fld1", "name": "priority", "type": "singleSelect"}

    def handler(request):
        if request.get_method() == "POST":
            return _json_response(stored)
        return _json_response([stored])

    api, _ = _install(monkeypatch, handler)

    got = admin.create_field(api, "tbl1", {"name": "priority", "type": "singleSelect"})
    assert got == stored


def test_create_field_reads_back_through_a_separate_request(monkeypatch):
    """The create response is the thing under suspicion, so it is not the evidence."""
    def handler(request):
        if request.get_method() == "POST":
            # A create response that lies: it echoes what was asked for.
            return _json_response({"id": "fld1", "name": "priority", "notNull": True})
        # The stored truth disagrees.
        return _json_response([{"id": "fld1", "name": "priority"}])

    api, calls = _install(monkeypatch, handler)

    with pytest.raises(admin.TeableAdminError) as caught:
        admin.create_field(api, "tbl1", {"name": "priority", "notNull": True})

    assert caught.value.kind == "readback_mismatch"
    assert [c.get_method() for c in calls] == ["POST", "GET"]


def test_create_field_refuses_when_the_field_does_not_appear_at_all(monkeypatch):
    def handler(request):
        if request.get_method() == "POST":
            return _json_response({"id": "fld1"})
        return _json_response([])

    api, _ = _install(monkeypatch, handler)

    with pytest.raises(admin.TeableAdminError) as caught:
        admin.create_field(api, "tbl1", {"name": "priority"})

    assert caught.value.kind == "readback_mismatch"


def test_create_field_refuses_a_response_carrying_no_id(monkeypatch):
    api, _ = _install(monkeypatch, lambda request: _json_response({"ok": True}))

    with pytest.raises(admin.TeableAdminError) as caught:
        admin.create_field(api, "tbl1", {"name": "priority"})

    assert caught.value.kind == "api_contract"


# --- create-table -----------------------------------------------------------


def test_create_table_reads_back_and_ignores_the_nested_fields_key(monkeypatch):
    """`fields` is a creation payload, not a stored table property."""
    stored = {"id": "tbl9", "name": "platform"}

    def handler(request):
        if request.get_method() == "POST":
            return _json_response(stored)
        return _json_response([stored])

    api, _ = _install(monkeypatch, handler)

    got = admin.create_table(api, "baseone", {"name": "platform", "fields": [{"name": "x"}]})
    assert got == stored


# --- the diff itself --------------------------------------------------------


def test_mismatches_names_both_absent_and_differing_properties():
    problems = admin.mismatches(
        {"name": "a", "notNull": True, "type": "number"},
        {"name": "a", "type": "singleLineText"},
    )
    assert any("notNull" in p and "absent" in p for p in problems)
    assert any("type" in p and "singleLineText" in p for p in problems)
    assert not any(p.startswith("name") for p in problems)


# --- refusals ---------------------------------------------------------------


@pytest.mark.parametrize("verb", sorted(admin.REFUSALS))
def test_a_refused_verb_names_its_defect_and_reaches_no_upstream(verb, monkeypatch, capsys):
    """Absence reads as unimplemented as easily as refused, so each one speaks."""
    def explode(_request, **_kwargs):  # pragma: no cover - reaching this is the failure
        raise AssertionError("a refused verb reached the network")

    monkeypatch.setattr(admin.urllib.request, "urlopen", explode)
    monkeypatch.setenv("TEABLE_API_TOKEN", "secret")

    assert admin.main([verb]) == admin.EXIT_CODES["refused"]
    assert "NOT AVAILABLE" not in capsys.readouterr().out


def test_convert_refusal_cites_the_destroyed_values(monkeypatch, capsys):
    monkeypatch.setenv("TEABLE_API_TOKEN", "secret")
    admin.main(["convert-field"])
    assert "6,536" in capsys.readouterr().err


# --- credential handling ----------------------------------------------------


def test_a_missing_token_is_a_typed_refusal_rather_than_a_traceback(monkeypatch, capsys):
    monkeypatch.delenv("TEABLE_API_TOKEN", raising=False)
    assert admin.main(["list-fields", "tbl1"]) == EXIT_AUTH
    assert "TEABLE_API_TOKEN is required" in capsys.readouterr().err


def test_the_token_travels_as_a_bearer_header_never_in_the_url(monkeypatch):
    api, calls = _install(monkeypatch, lambda request: _json_response([]))
    api.list_fields("tbl1")
    assert calls[0].get_header("Authorization") == "Bearer secret"
    assert "secret" not in calls[0].full_url


# --- upstream failures ------------------------------------------------------


def test_an_http_error_surfaces_its_status_and_body(monkeypatch):
    def handler(request):
        raise urllib.error.HTTPError(
            request.full_url, 401, "fixture", Message(), io.BytesIO(b'{"message":"Unauthorized"}')
        )

    api, _ = _install(monkeypatch, handler)

    with pytest.raises(admin.TeableAdminError) as caught:
        api.list_fields("tbl1")

    assert caught.value.kind == "api_failure"
    assert "401" in str(caught.value)
    assert "Unauthorized" in str(caught.value)


def test_a_non_array_field_list_is_a_contract_failure(monkeypatch):
    api, _ = _install(monkeypatch, lambda request: _json_response({"not": "an array"}))

    with pytest.raises(admin.TeableAdminError) as caught:
        api.list_fields("tbl1")

    assert caught.value.kind == "api_contract"


# --- edit-choices, against the convert that emptied a column ----------------


class FakeTracker:
    """One select field and its records, converted the way Teable's source does it.

    A convert renames a choice by id on every record and strips a choice whose id
    is absent. `emptying` reproduces the 6,536-value defect instead.
    """

    def __init__(self, records: dict[str, object], emptying: bool = False) -> None:
        self.field = {
            "id": "fldRoles",
            "name": "roles",
            "type": "multipleSelect",
            "notNull": True,
            "options": {
                "choices": [
                    {"id": "cho1", "name": "platform", "color": "blueLight2"},
                    {"id": "cho2", "name": "science", "color": "blueBright"},
                ]
            },
        }
        self.records = dict(records)
        self.emptying = emptying
        self.converts: list[dict] = []

    def handle(self, request: urllib.request.Request) -> FakeResponse:
        url = request.full_url
        if request.get_method() == "PUT":
            body = json.loads(request.data)
            self.converts.append(body)
            old = {c["id"]: c["name"] for c in self.field["options"]["choices"]}
            new = {c["id"]: c["name"] for c in body["options"]["choices"] if "id" in c}
            mapping = {name: new.get(cid) for cid, name in old.items()}
            for rid, value in self.records.items():
                if self.emptying:
                    self.records[rid] = None
                else:
                    self.records[rid] = [mapping[v] for v in value if mapping.get(v)]
            self.field = {**self.field, "options": body["options"]}
            return _json_response(self.field)
        if "/record" in url:
            query = urllib.parse.parse_qs(urllib.parse.urlparse(url).query)
            skip, take = int(query["skip"][0]), int(query["take"][0])
            page = list(self.records.items())[skip : skip + take]
            return _json_response(
                {"records": [{"id": rid, "fields": {"fldRoles": v}} for rid, v in page]}
            )
        return _json_response([self.field])


def _tracker(monkeypatch, **kwargs) -> tuple[admin.TeableAPI, FakeTracker]:
    tracker = FakeTracker(
        {"rec1": ["platform"], "rec2": ["platform", "science"], "rec3": ["science"]}, **kwargs
    )
    api, _ = _install(monkeypatch, tracker.handle)
    return api, tracker


def test_edit_choices_renames_in_place_and_carries_every_record(monkeypatch, tmp_path):
    monkeypatch.setattr(admin.tempfile, "tempdir", str(tmp_path))
    api, tracker = _tracker(monkeypatch)

    got = admin.edit_choices(api, "tbl1", "roles", {"platform": "platform-eng"}, ["game-dev"])

    sent = tracker.converts[0]["options"]["choices"]
    assert [c.get("id") for c in sent] == ["cho1", "cho2", None]
    assert tracker.records["rec2"] == ["platform-eng", "science"]
    assert got["choices"] == ["platform-eng", "science", "game-dev"]
    assert json.loads(open(got["snapshot"]).read())["values"]["rec1"] == ["platform"]


def test_edit_choices_catches_a_convert_that_empties_the_column(monkeypatch, tmp_path):
    monkeypatch.setattr(admin.tempfile, "tempdir", str(tmp_path))
    api, _ = _tracker(monkeypatch, emptying=True)

    with pytest.raises(admin.TeableAdminError) as caught:
        admin.edit_choices(api, "tbl1", "fldRoles", {"platform": "platform-eng"}, [])

    assert caught.value.kind == "readback_mismatch"
    assert "3 record values differ" in str(caught.value)
    assert str(tmp_path) in str(caught.value)


def test_edit_choices_refuses_an_all_empty_snapshot_before_writing(monkeypatch):
    tracker = FakeTracker({"rec1": None, "rec2": []})
    api, _ = _install(monkeypatch, tracker.handle)

    with pytest.raises(admin.TeableAdminError, match="cannot prove preservation"):
        admin.edit_choices(api, "tbl1", "roles", {"platform": "platform-eng"}, [])
    assert tracker.converts == []


def test_edit_choices_dry_run_writes_nothing(monkeypatch):
    api, tracker = _tracker(monkeypatch)
    got = admin.edit_choices(api, "tbl1", "roles", {"science": "scientist"}, [], dry_run=True)
    assert got["records"] == 3 and got["renamed_in_use"] == ["science"]
    assert tracker.converts == []


def test_edit_choices_pages_through_every_record(monkeypatch, tmp_path):
    monkeypatch.setattr(admin.tempfile, "tempdir", str(tmp_path))
    monkeypatch.setattr(admin, "RECORD_PAGE", 2)
    api, tracker = _tracker(monkeypatch)
    admin.edit_choices(api, "tbl1", "roles", {"science": "scientist"}, [])
    assert tracker.records["rec3"] == ["scientist"]


@pytest.mark.parametrize(
    ("renames", "additions", "message"),
    [
        ({"nope": "x"}, [], "no choice named"),
        ({"platform": "science"}, [], "duplicate"),
        ({}, ["platform"], "duplicate"),
        ({}, [], "nothing to do"),
    ],
)
def test_plan_choices_refuses_what_would_collide_or_miss(renames, additions, message):
    field = FakeTracker({}).field
    with pytest.raises(admin.TeableAdminError, match=message):
        admin.plan_choices(field, renames, additions)


def test_plan_choices_refuses_a_field_that_is_not_a_select():
    with pytest.raises(admin.TeableAdminError, match="not a select"):
        admin.plan_choices({"name": "title", "type": "singleLineText"}, {"a": "b"}, [])


def test_edit_choices_main_parses_rename_pairs(monkeypatch, tmp_path, capsys):
    monkeypatch.setattr(admin.tempfile, "tempdir", str(tmp_path))
    monkeypatch.setenv("TEABLE_API_TOKEN", "secret")
    _, tracker = _tracker(monkeypatch)

    code = admin.main(
        ["edit-choices", "tbl1", "roles", "--rename", "platform=platform-eng", "--add", "x"]
    )

    assert code == 0
    assert tracker.records["rec1"] == ["platform-eng"]
    assert admin.main(["edit-choices", "tbl1", "roles", "--rename", "bad"]) == 64


def test_edit_choices_flags_a_convert_that_drops_not_null(monkeypatch, tmp_path):
    monkeypatch.setattr(admin.tempfile, "tempdir", str(tmp_path))
    api, tracker = _tracker(monkeypatch)
    original = tracker.handle

    def dropping(request):
        response = original(request)
        if request.get_method() == "PUT":
            tracker.field = {k: v for k, v in tracker.field.items() if k != "notNull"}
        return response

    _install(monkeypatch, dropping)
    with pytest.raises(admin.TeableAdminError, match="notNull"):
        admin.edit_choices(api, "tbl1", "roles", {"science": "scientist"}, [])
