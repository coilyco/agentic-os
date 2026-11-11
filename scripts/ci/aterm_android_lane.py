#!/usr/bin/env python3
"""Run a seat's scripted proof on the CI emulator (COI-3591).

The step file is JSON: {"name", "start": "seat" | "launch" | "none", "steps": [...]}. Each
step is {"op": ..., ...}. The ops, their fields and the result files are in
tooling-aterm-client references/android-emulator-lane.md. Standard library only.
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import re
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
APP = "dev.coilyco.treehouse"
FAKE_RECOGNIZER = f"{APP}/{APP}.dictation.FakeRecognitionService"


def _journey_module():
    """The journey file's helpers, loaded by path because `python3 -I` drops this directory."""
    spec = importlib.util.spec_from_file_location("aterm_android_journey", HERE / "aterm_android_journey.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


J = _journey_module()
Fail = J.Fail
adb = J.adb


class Lane(J.Journey):
    def __init__(self, args: argparse.Namespace, plan: dict) -> None:
        super().__init__(args)
        self.plan = plan
        self.results: list[dict] = []
        self.saved: dict[str, str] = {}
        self.recognizer_before = ""

    # A failed step stops the run, after a screenshot and the evidence files.
    def run(self) -> None:
        start = self.plan.get("start", "seat")
        steps = self.plan["steps"]
        try:
            if start in ("seat", "launch"):
                self.install_and_launch()
                self.attach_page()
            if start == "seat":
                self.add_host_and_open_seat()
            self.recognizer_before = adb("shell", "settings", "get", "secure", "voice_recognition_service", check=False).strip()
            for index, spec in enumerate(steps, start=1):
                self.do(index, spec)
        except (Fail, OSError, subprocess.TimeoutExpired, KeyError) as err:
            self.shot("FAILED")
            self.evidence()
            self.finish(f"{err!r}")
            print(f"FAILED: {err}", file=sys.stderr)
            raise SystemExit(1) from err
        self.evidence()
        self.finish(None)

    def finish(self, failure: str | None) -> None:
        body = {"name": self.plan.get("name", ""), "failure": failure, "steps": self.results, "saved": self.saved}
        (self.out / "lane-result.json").write_text(json.dumps(body, indent=2))

    def do(self, index: int, spec: dict) -> None:
        op = spec["op"]
        print(f"== {index} {op} {spec.get('name') or spec.get('selector') or spec.get('text') or ''}".rstrip(), flush=True)
        note = getattr(self, f"op_{op.replace('-', '_')}", None)
        if note is None:
            raise Fail(f"step {index}: no op named {op!r}")
        out = note(spec)
        self.results.append({"step": index, "op": op, "name": spec.get("name"), "out": out})

    def page_(self):
        if not self.page:
            raise Fail("no page: the app is not attached, so `launch` or `reopen` first")
        return self.page

    # ops
    def op_tap(self, spec: dict) -> None:
        self.page_().tap(f"()=>document.querySelector({json.dumps(spec['selector'])})", wait=spec.get("timeout", 20))

    def op_tap_text(self, spec: dict) -> None:
        self.page_().tap(J.by_text(spec["text"], spec.get("within", "button,[role=tab],[role=button],a,li,[aria-label]")), wait=spec.get("timeout", 20))

    def op_fill(self, spec: dict) -> None:
        self.page_().wait_selector(spec["selector"], spec.get("timeout", 30))
        js = (
            f"(()=>{{const e=document.querySelector({json.dumps(spec['selector'])});e.focus();e.value={json.dumps(spec['text'])};"
            "e.dispatchEvent(new Event('input',{bubbles:true}))}})()"
        )
        self.page_().js(js)

    def op_attach_image(self, spec: dict) -> None:
        """Give the composer's file input the seeded PNG, the way a pick would."""
        self.add_image_without_picker()

    def op_eval(self, spec: dict) -> str:
        value = str(self.page_().js(spec["js"]))
        if "save" in spec:
            self.saved[spec["save"]] = value
        if "expect" in spec and not re.search(spec["expect"], value, re.DOTALL | re.IGNORECASE):
            raise Fail(f"eval {spec['js'][:80]!r} gave {value[:200]!r}, expected /{spec['expect']}/")
        return value[:400]

    def op_wait(self, spec: dict) -> None:
        self.wait_words(spec["pattern"], spec.get("timeout", 20), spec.get("name", "wait"))

    def op_wait_value(self, spec: dict) -> str:
        """Wait for a field's value to match a pattern, which is how speech or a restored draft shows."""
        deadline = time.monotonic() + spec.get("timeout", 30)
        value = ""
        while time.monotonic() < deadline:
            value = str(self.page_().js(f"(document.querySelector({json.dumps(spec['selector'])})||{{}}).value||''"))
            if re.search(spec["pattern"], value, re.IGNORECASE):
                if "save" in spec:
                    self.saved[spec["save"]] = value
                return value
            time.sleep(0.5)
        raise Fail(f"{spec['selector']} never matched /{spec['pattern']}/. last value: {value!r}. page: {self.page_().text()[-300:]!r}")

    def op_adb(self, spec: dict) -> str:
        return adb(*spec["args"], check=spec.get("check", False))[:400]

    def op_sleep(self, spec: dict) -> None:
        time.sleep(float(spec["seconds"]))

    def op_shot(self, spec: dict) -> None:
        self.shot(spec["name"])

    def op_keyboard(self, spec: dict) -> None:
        shown = "mInputShown=true" in adb("shell", "dumpsys input_method | grep -m1 mInputShown", check=False)
        if spec.get("action", "close") == "close" and shown:
            adb("shell", "input", "keyevent", "4")
            time.sleep(1)

    def op_kill_app(self, spec: dict) -> None:
        adb("shell", "am", "force-stop", APP)
        self.page = None

    def op_clear_app(self, spec: dict) -> None:
        adb("shell", "pm", "clear", APP)
        self.page = None

    def op_launch(self, spec: dict) -> None:
        adb("shell", "am", "start", "-n", f"{APP}/.MainActivity")
        self.page = None
        self.attach_page()

    def op_reopen(self, spec: dict) -> None:
        """Launch and attach, then open the host and the seat if the page is not already on one."""
        self.op_launch(spec)
        page = self.page_()
        if page.js("!!document.querySelector('#composer-input')"):
            return
        try:
            page.tap("()=>document.querySelector('[id^=tab-host-]')", wait=spec.get("timeout", 8))
        except Fail:
            print("reopen: no host tab, carrying on with the page as it is")
        self.open_seat()

    def op_recognizer(self, spec: dict) -> None:
        """`fake` names the debug build's FakeRecognitionService the phone's default, `system` puts back what was there."""
        if spec["service"] == "fake":
            adb("shell", "settings", "put", "secure", "voice_recognition_service", FAKE_RECOGNIZER)
        elif self.recognizer_before:
            adb("shell", "settings", "put", "secure", "voice_recognition_service", self.recognizer_before)

    def op_speak(self, spec: dict) -> None:
        self.speak()

    def op_logcat(self, spec: dict) -> None:
        lines = [line for line in adb("logcat", "-d", "-t", "10000", check=False).splitlines() if re.search(spec["pattern"], line, re.IGNORECASE)]
        (self.out / f"logcat-{spec['name']}.txt").write_text("\n".join(lines[-400:]))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("apk", "out", "socket", "seat", "seat-log", "refuse-file", "steps"):
        parser.add_argument(f"--{name}", required=True)
    args = parser.parse_args()
    plan = json.loads(Path(args.steps).read_text())
    Lane(args, plan).run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
