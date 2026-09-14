"""The readback comparator: normalise representation, still catch real loss."""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from agentic_os.teable_admin import mismatches, survived
from agentic_os.teable_personal import TeableError, _read_spec


# Every shape below was read off a live table rather than imagined, after ten
# writes that landed and reported failure (#7644).
def test_a_date_gains_a_time_on_write() -> None:
    assert survived("2026-09-13", "2026-09-13T00:00:00.000Z")


def test_a_link_gains_its_title_on_write() -> None:
    assert survived({"id": "recX"}, {"id": "recX", "title": "Julia Evans"})


# The singleSelect case from #6929: Teable assigns every choice a cho... id.
def test_a_choice_gains_a_server_id_on_write() -> None:
    assert survived(
        {"choices": [{"name": "approach", "color": "green"}]},
        {"choices": [{"id": "cho1", "name": "approach", "color": "green"}]},
    )


# The reason the readback exists: this instance accepts unknown properties and
# discards them silently, which a normalising comparator must still catch.
def test_a_discarded_property_still_fails() -> None:
    assert mismatches({"description": "x"}, {}) == [
        "description: requested, absent from the read-back"
    ]


def test_a_changed_value_still_fails() -> None:
    assert mismatches({"color": "green"}, {"color": "red"})


def test_a_dropped_choice_still_fails() -> None:
    assert not survived(
        {"choices": [{"name": "a"}, {"name": "b"}]}, {"choices": [{"name": "a"}]}
    )


def test_reordered_choices_still_fail() -> None:
    assert not survived(
        {"c": [{"name": "a"}, {"name": "b"}]}, {"c": [{"name": "b"}, {"name": "a"}]}
    )


def test_a_different_date_still_fails() -> None:
    assert not survived("2026-09-13", "2026-09-14T00:00:00.000Z")


def test_a_nested_discard_still_fails() -> None:
    assert not survived({"options": {"a": 1, "b": 2}}, {"options": {"a": 1}})


# A non-date string must not be coerced into one on either side.
def test_plain_strings_compare_literally() -> None:
    assert survived("approach", "approach")
    assert not survived("approach", "do not approach")


def test_a_type_change_fails(tmp_path: Path) -> None:
    assert not survived({"id": "recX"}, "recX")
    assert not survived([{"id": "a"}], {"id": "a"})


# The API answers this envelope with a 404 naming a field called "fields",
# which reads like a schema fault rather than a wrapper mistake (#7644).
def test_a_fields_envelope_is_named_rather_than_sent(tmp_path: Path) -> None:
    spec = tmp_path / "spec.json"
    spec.write_text(json.dumps({"fields": {"name": "x"}}), encoding="utf-8")
    with pytest.raises(TeableError) as caught:
        _read_spec(str(spec))
    assert "envelope" in str(caught.value)


def test_a_bare_field_object_is_accepted(tmp_path: Path) -> None:
    spec = tmp_path / "spec.json"
    spec.write_text(json.dumps({"fields": {"a": 1}, "name": "x"}), encoding="utf-8")
    assert _read_spec(str(spec)) == {"fields": {"a": 1}, "name": "x"}
