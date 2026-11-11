# The CI emulator lane

A second way for a Frontend seat to prove something in the real Treehouse app, with no rig lock (COI-3591). The rig in [android-emulator.md](android-emulator.md) is one shared emulator on kai-server and four seats queued for it. The lane is a GitHub job any seat dispatches, so it costs no host memory and runs beside other lanes. It is not interactive. A seat hands it a ref, a step file and an image, and reads screenshots back.

**When each fits.** The rig answers a tap while you watch and holds state between commands. The lane runs one script start to finish on a fresh emulator, in 15 to 30 minutes, and returns files. Use the lane for a proof with a before and after, and the rig to explore.

## Dispatch

`gh workflow run aterm-android-lane.yml --repo coilyco/agentic-os -f ref=<branch> -f steps=<path in that ref>`, then `gh run watch` and `gh run download <id> -n aterm-android-lane-<id>`.

* `ref` - the git ref to prove. Its debug x86_64 APK is built there, and the step file is read from it, so a PR branch carries its own proof. Default `main`.
* `steps` - the step file, a path inside `ref`, or the lane's own copy when `ref` is too old to have one. Default `scripts/ci/lane-steps/smoke.json`.
* `apk_run` - a run id holding an `aterm-android-debug-x86_64-<id>` artifact, to skip the 7 minute build. Empty builds from `ref`.
* `image` - a system image: Android 14, 15 or 16, `google_apis`, or `google_apis_playstore` for 14 and 15 (the one with Google's speech app). **Default is Android 15**, because Android 14 predates edge-to-edge and cannot show a keyboard that covers the page (COI-3575).
* `audio` - a spoken phrase the emulator's microphone can hear, for steps that use `speak`.

The workflow file and the runner come from the ref you dispatch on, which is `main` unless you pass `--ref`. The proof runs against `ref`. Anyone who can dispatch can run any step file, so there are no secrets in the job.

## What the job has

The chosen image (Android 15 `google_apis` by default), the debug APK installed with every permission granted, a real `aterm daemon` on the runner behind the rig's relay, and one stub seat, `emulator-seat` (role `frontend-eng`), that asks no question. The app starts on a fresh install. A debug build is required, because DevTools is the way to read the page.

## The step file

JSON: `{"name", "start", "steps": [...]}`. `start` is `seat` (install, open, add the host, open the seat, the default), `launch` (open the app and attach) or `none`. Every step has an `op`. A failed step stops the run after a `FAILED` screenshot.

* `tap` - `selector`. A DevTools-measured tap at the element's centre.
* `tap-text` - `text`, optionally `within`. The first element whose text or label holds it.
* `fill` - `selector`, `text`. Waits for the element, sets the value and fires `input`.
* `attach-image` - gives the composer's file input the seeded PNG, the way a pick would.
* `eval` - `js`, optionally `expect` (a regex on the result) and `save` (a name in the result file).
* `wait` - `pattern`, `timeout`. Waits for the page text to match.
* `assert` - `js`, `message`. A script that must be true, with `$name` for a value an earlier `eval` saved. `eval` takes `$name` too.
* `wait-keyboard` - `state` (`shown` or `hidden`), `timeout`.
* `wait-value` - `selector`, `pattern`, `timeout`, optionally `save`. Waits for a field's value, which is how a restored draft or speech shows.
* `shot` - `name`. A named screenshot.
* `keyboard` - `action: close`. Hides the soft keyboard, which covers the composer.
* `kill-app`, `clear-app`, `launch`, `reopen` - the app's life. `reopen` launches, attaches and re-opens the host and seat.
* `adb` - `args`. Any adb call, output kept in the result file.
* `recognizer` - `service: fake` or `vosk` names a debug-build recogniser the phone's own (`vosk` pushes the model first, needs `audio`), `system` puts back the image's own.
* `speak` - feeds the spoken phrase to the emulator's microphone (needs `audio`).
* `logcat` - `name`, `pattern`. Matching logcat lines into `logcat-<name>.txt`.
* `ui-dump` - `name`. The system UI's uiautomator tree as `ui-<name>.xml` and a text list `ui-<name>.txt`, for Android's own screens.
* `tap-ui` - `pattern`, `timeout`. Taps the first system UI node whose text or description matches the regex.
* `repeat` - `steps`, `times`, `save`. Runs the steps that many times and counts the passes in `saved`, never stopping the run. `$i` is the round number, and `fill` text and `wait-value` patterns take it as `eval` does. For a rate.
* `retry` - `steps`, `times`, `every`. Runs the steps again, `every` seconds apart, until they pass. For what needs a system to settle, such as Google's speech pack.
* `sleep` - `seconds`.

Samples in `scripts/ci/lane-steps/`: `smoke`, `dictation-fake`, `dictation-spoken`, `draft-survives-kill` (settles 3 s first), `draft-kill-0s`, `-1s` and `-3s`, `draft-kill-rate` (5 rounds at each of 0.2, 1 and 3 s in one run), `draft-image-survives-kill`, `screen-off-timer` and `composer-above-keyboard`, which fails on a build where the keyboard covers the message box. Code: `scripts/ci/aterm_android_lane.py`. It subclasses the journey in `aterm_android_journey.py`, so attach, tap and evidence are shared.

## What comes back

The artifact `aterm-android-lane-<run id>` holds `screens/NN-name.png` for each `shot` (numbered in order), `screens/lane-result.json` (each step, its output, `saved` values and the failure if any), logcat extracts, `daemon.log` and `emulator.log`. A failed run adds `FAILED.png`, `ui.xml`, `window.txt`, `page.txt` and a logcat. Put the screenshots on the Linear record, by name.

## The microphone

`audio` makes a phrase with espeak (`ATERM_LANE_PHRASE`, default `hello world this is a test`), converts it to 16 kHz mono and boots the emulator with `-grpc 8554` instead of `-no-audio`. `speak` streams it to the emulator's `EmulatorController/injectAudio` with `grpcurl`, which the emulator feeds to its virtual microphone. **Measured on 2026-11-10:** the guest's own `tinycap` heard it at 0.99 peak amplitude, against 0.0002 with nothing injected. **PulseAudio did not work.** The emulator printed `Could not init 'pa' audio driver` with a null sink, `PULSE_SERVER`, `XDG_RUNTIME_DIR` and `libpulse-dev` all tried, and the mic stayed silent. `aterm_audio_stream.py` builds the stream and `tests/test_aterm_audio_stream.py` pins its shape.

**Google's recognizer does not recognise it on a CI emulator.** The phrase reaches the app's `SpeechRecognizer`, and the answer is always `No words recognised`. On `google_apis_playstore` (Android 14 and 15) the en-US pack never installed: a 7 minute run of retries logged `Returning no LP, as MDD has not downloaded this pack yet` every time, with Google's background download tasks cancelled. On `google_apis` there is no pack and the online path answers 401. So `dictation-spoken` fails on the lane. **`dictation-vosk` is the lane's real-audio proof:** the debug build carries `VoskRecognitionService` (vosk-android, offline), the `audio` step fetches the small en-US model by pinned hash, and `recognizer: vosk` pushes it into the app's files. It is not Google's recogniser, so it proves the audio path, the plugin and the page, and a Pixel is the check for Google's own. Release builds carry none of it, and `aterm-android.sh debug-speech` asserts that on every release build and the opposite on every debug build (COI-3591).

## Limits

* A fresh emulator every run, so no state carries over. Use `kill-app` and `reopen` to test what survives a close.
* The seat is a fixture, not a harness. Native dialogs of a real agent are not exercised.
* x86_64 only, an emulator and not Kai's phone, and the image's WebView, not hers.
* Actions minutes are watched (COI-3249), so the lane is dispatch only.
