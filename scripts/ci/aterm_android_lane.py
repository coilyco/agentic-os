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
import os
import re
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
APP = "dev.coilyco.treehouse"
FAKE_RECOGNIZER = f"{APP}/{APP}.dictation.FakeRecognitionService"
VOSK_RECOGNIZER = f"{APP}/{APP}.dictation.VoskRecognitionService"


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
            # A chatty system rolls the default log buffer in seconds.
            adb("logcat", "-G", "16M", check=False)
            for index, spec in enumerate(steps, start=1):
                self.do(index, spec)
        except (Fail, OSError, subprocess.TimeoutExpired, KeyError) as err:
            self.shot("FAILED")
            self.evidence()
            self.op_logcat({"name": "speech", "pattern": "speech|recogni|soda|dictation|language pack|MDD|vosk"})
            self.finish(f"{err!r}")
            print(f"FAILED: {err}", file=sys.stderr)
            raise SystemExit(1) from err
        self.evidence()
        self.finish(None)

    def finish(self, failure: str | None) -> None:
        body = {"name": self.plan.get("name", ""), "failure": failure, "steps": self.results, "saved": self.saved}
        (self.out / "lane-result.json").write_text(json.dumps(body, indent=2))

    def op_repeat(self, spec: dict) -> str:
        """Run `steps` `times` times and count the passes, never stopping the run. `$i` is the round."""
        times, passed, failures = spec.get("times", 5), 0, []
        for round_ in range(1, times + 1):
            self.saved["i"] = str(round_)
            try:
                for sub, inner in enumerate(spec["steps"], start=1):
                    self.do(f"{round_}.{sub}", inner)
                passed += 1
            except Fail as err:
                failures.append(f"round {round_}: {str(err)[:160]}")
                print(f"repeat: round {round_} of {times} failed: {str(err)[:200]}", flush=True)
        summary = f"{passed}/{times} passed"
        self.saved[spec.get("save", "repeat")] = summary
        if failures:
            self.saved[spec.get("save", "repeat") + "_failures"] = " | ".join(failures)
        return summary

    def op_retry(self, spec: dict) -> str:
        """Run `steps` up to `times` times, `every` seconds apart. For what needs a system to settle."""
        times, pause, last = spec.get("times", 5), spec.get("every", 30), None
        for attempt in range(1, times + 1):
            try:
                for sub, inner in enumerate(spec["steps"], start=1):
                    self.do(f"{attempt}.{sub}", inner)
                return f"passed on attempt {attempt}"
            except Fail as err:
                last = err
                print(f"retry: attempt {attempt} of {times} failed: {str(err)[:200]}", flush=True)
                time.sleep(pause)
        raise Fail(f"all {times} attempts failed, the last: {last}")

    def do(self, index: int | str, spec: dict) -> None:
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
            f"(()=>{{const e=document.querySelector({json.dumps(spec['selector'])});e.value={json.dumps(self.sub(spec['text']))};"
            "e.dispatchEvent(new Event('input',{bubbles:true}))})()"
        )
        self.page_().js(js)

    def op_attach_image(self, spec: dict) -> None:
        """Give the composer's file input the seeded PNG, the way a pick would."""
        self.add_image_without_picker()

    def op_eval(self, spec: dict) -> str:
        value = str(self.page_().js(self.sub(spec["js"])))
        if "save" in spec:
            self.saved[spec["save"]] = value
        if "expect" in spec and not re.search(spec["expect"], value, re.DOTALL | re.IGNORECASE):
            raise Fail(f"eval {spec['js'][:80]!r} gave {value[:200]!r}, expected /{spec['expect']}/")
        return value[:400]

    def sub(self, text: str) -> str:
        """Put saved values into a script: `$name` becomes what an earlier step saved as `name`."""
        return re.sub(r"\$(\w+)", lambda m: self.saved.get(m.group(1), m.group(0)), text)

    def op_assert(self, spec: dict) -> str:
        """A script over the page that must be true, with `$name` for saved values. `message` says why it matters."""
        value = str(self.page_().js(self.sub(spec["js"])))
        if value.lower() != "true":
            raise Fail(f"{spec.get('message', 'assert')}: {self.sub(spec['js'])[:160]!r} gave {value!r}")
        return value

    def op_wait_keyboard(self, spec: dict) -> None:
        want = spec.get("state", "shown") == "shown"
        deadline = time.monotonic() + spec.get("timeout", 15)
        while time.monotonic() < deadline:
            if ("mInputShown=true" in adb("shell", "dumpsys input_method | grep -m1 mInputShown", check=False)) == want:
                return
            time.sleep(0.5)
        raise Fail(f"the soft keyboard never became {spec.get('state', 'shown')}")

    def op_wait(self, spec: dict) -> None:
        self.wait_words(spec["pattern"], spec.get("timeout", 20), spec.get("name", "wait"))

    def op_wait_value(self, spec: dict) -> str:
        """Wait for a field's value to match a pattern, which is how speech or a restored draft shows."""
        deadline = time.monotonic() + spec.get("timeout", 30)
        value = ""
        while time.monotonic() < deadline:
            value = str(self.page_().js(f"(document.querySelector({json.dumps(spec['selector'])})||{{}}).value||''"))
            if re.search(self.sub(spec["pattern"]), value, re.IGNORECASE):
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
        if self.composer_visible():
            return
        try:
            page.tap("()=>document.querySelector('[id^=tab-host-]')", wait=spec.get("timeout", 8))
        except Fail:
            print("reopen: no host tab, carrying on with the page as it is")
        self.open_seat()

    def op_recognizer(self, spec: dict) -> None:
        """`fake` or `vosk` (`input` file or mic) name a debug service the default, `system` undoes it."""
        which = spec["service"]
        if which == "vosk":
            self.push_vosk_model()
            self.push_vosk_input(spec.get("input", "mic"))
        if which in ("fake", "vosk"):
            adb("shell", "settings", "put", "secure", "voice_recognition_service", VOSK_RECOGNIZER if which == "vosk" else FAKE_RECOGNIZER)
        elif self.recognizer_before:
            adb("shell", "settings", "put", "secure", "voice_recognition_service", self.recognizer_before)

    def push_vosk_input(self, source: str) -> None:
        """`file` plays the job's spoken phrase to the recogniser, `mic` removes it so the mic is read."""
        wav = os.environ.get("ATERM_SPEECH_WAV", "")
        if source == "file":
            if not wav or not Path(wav).is_file():
                raise Fail("no phrase WAV: the job did not run its `audio` step (ATERM_SPEECH_WAV)")
            adb("push", wav, "/data/local/tmp/vosk-input.wav", timeout=60)
            adb("shell", f"run-as {APP} sh -c 'cp /data/local/tmp/vosk-input.wav files/vosk-input.wav'")
        else:
            adb("shell", f"run-as {APP} rm -f files/vosk-input.wav", check=False)

    def push_vosk_model(self) -> None:
        """The model dir the job fetched (`audio` verb) goes into the app's files, where the service reads it."""
        model = os.environ.get("ATERM_VOSK_MODEL", "")
        if not model or not Path(model, "am").is_dir():
            raise Fail("no Vosk model: the job did not run its `audio` step (ATERM_VOSK_MODEL)")
        adb("shell", "rm", "-rf", "/data/local/tmp/vosk-model")
        adb("push", f"{model}/.", "/data/local/tmp/vosk-model", timeout=300)
        adb("shell", f"run-as {APP} sh -c 'rm -rf files/vosk-model && mkdir -p files && cp -r /data/local/tmp/vosk-model files/vosk-model'")
        have = adb("shell", f"run-as {APP} ls files/vosk-model").split()
        if "am" not in have:
            raise Fail(f"the model did not reach the app's files: {have}")

    def op_speak(self, spec: dict) -> None:
        self.speak()

    def op_ui_dump(self, spec: dict) -> None:
        """The system UI as uiautomator sees it, for steps that drive Android's own screens."""
        xml = self.ui_xml()
        (self.out / f"ui-{spec['name']}.xml").write_text(xml)
        tree = J.ET.fromstring(xml or "<hierarchy/>")
        words = [f"{n.get('text') or ''} | {n.get('content-desc') or ''} | {n.get('resource-id') or ''}" for n in tree.iter("node")]
        (self.out / f"ui-{spec['name']}.txt").write_text("\n".join(w for w in words if w.strip(" |")))

    def op_tap_ui(self, spec: dict) -> None:
        """Tap the first system UI node whose text or description matches the regex `pattern`."""
        pattern = re.compile(spec["pattern"], re.IGNORECASE)

        def wanted(node) -> bool:
            return bool(pattern.search(node.get("text") or "") or pattern.search(node.get("content-desc") or ""))

        if not self.tap_node(wanted, spec.get("timeout", 20)):
            raise Fail(f"no system UI node matched /{spec['pattern']}/")

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
