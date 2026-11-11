"""Tests for the CI emulator lane's step runner (COI-3591), with no emulator."""

from __future__ import annotations

import argparse
import importlib.util
import json
from pathlib import Path

import pytest

CI = Path(__file__).resolve().parent.parent / "scripts" / "ci"


def _load():
    spec = importlib.util.spec_from_file_location("aterm_android_lane", CI / "aterm_android_lane.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakePage:
    def __init__(self, answers: list[object] | None = None) -> None:
        self.scripts: list[str] = []
        self.answers = answers or []

    def wait_selector(self, css: str, wait: float = 20) -> None:
        self.waited = css

    def js(self, expression: str) -> object:
        self.scripts.append(expression)
        return self.answers.pop(0) if self.answers else None

    def text(self) -> str:
        return ""


def _lane(tmp_path: Path, page: FakePage):
    lane_mod = _load()
    args = argparse.Namespace(out=str(tmp_path), apk="a.apk", socket="s", seat="x", seat_log="l", refuse_file="r", steps="p")
    lane = lane_mod.Lane(args, {"steps": []})
    lane.page = page
    return lane_mod, lane


def test_fill_sends_a_script_with_balanced_braces_that_ends_in_an_input_event(tmp_path: Path) -> None:
    _, lane = _lane(tmp_path, FakePage())
    lane.op_fill({"selector": "#composer-input", "text": "hello"})
    script = lane.page.scripts[0]
    assert "}}" not in script.replace("}))})()", "")
    assert script.count("{") == script.count("}")
    assert script.count("(") == script.count(")")
    assert script.endswith("e.dispatchEvent(new Event('input',{bubbles:true}))})()")
    assert json.dumps("hello") in script


def test_a_saved_value_is_put_into_a_later_script(tmp_path: Path) -> None:
    _, lane = _lane(tmp_path, FakePage(["915"]))
    lane.op_eval({"js": "innerHeight", "save": "closed"})
    assert lane.saved == {"closed": "915"}
    assert lane.sub("innerHeight < $closed - 150") == "innerHeight < 915 - 150"
    assert lane.sub("a $missing b") == "a $missing b"


def test_an_assert_that_is_false_fails_with_its_reason(tmp_path: Path) -> None:
    lane_mod, lane = _lane(tmp_path, FakePage(["False"]))
    with pytest.raises(lane_mod.Fail, match="the page did not shrink"):
        lane.op_assert({"js": "false", "message": "the page did not shrink"})


def test_an_assert_that_is_true_passes_in_any_case(tmp_path: Path) -> None:
    _, lane = _lane(tmp_path, FakePage(["True"]))
    assert lane.op_assert({"js": "true"}) == "True"


def test_eval_expect_matches_case_insensitively(tmp_path: Path) -> None:
    lane_mod, lane = _lane(tmp_path, FakePage(["True", "False"]))
    lane.op_eval({"js": "x", "expect": "true"})
    with pytest.raises(lane_mod.Fail):
        lane.op_eval({"js": "x", "expect": "true"})


def test_every_op_in_every_sample_step_file_exists() -> None:
    lane_mod = _load()
    for path in sorted((CI / "lane-steps").glob("*.json")):
        plan = json.loads(path.read_text())
        assert plan["start"] in ("seat", "launch", "none"), path.name
        for step in plan["steps"]:
            assert hasattr(lane_mod.Lane, f"op_{step['op'].replace('-', '_')}"), f"{path.name}: no op {step['op']!r}"
