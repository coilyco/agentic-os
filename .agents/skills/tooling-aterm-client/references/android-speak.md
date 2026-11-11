# Reading a reply aloud on Android

COI-3466's first slice, "Read it to me". `aterm-android/plugins/tauri-plugin-speak` wraps the phone's `TextToSpeech`. The page calls it with `window.__TAURI__.core.invoke('plugin:speak|<command>')`. There is no event channel, so the page polls, as it does for [dictation](android-dictation.md). The client loop (speaker toggle, stop control, which replies) is frontend-eng's. ElevenLabs is deferred.

## The shape

* `available` - `{available, engine, voices}`. Waits up to 4 s for the engine to start. `engine` is the default engine's package, `voices` the count it lists.
* `speak {id, text, language?, rate?}` - resolves at once with `{id}`. A new call flushes whatever is speaking. `rate` is 0.5 to 2.0, default 1.0. Rejects `empty` for blank text, `unavailable` with no engine, `focus` when another app holds the audio.
* `stop` - stops the utterance and gives the audio focus back. A speaking phase becomes `idle`.
* `poll` - `{seq, phase, id, startedMs, doneMs, error}`. `phase` is `idle`, `speaking`, `done` or `error`. `startedMs` and `doneMs` count from the `speak` call, so a lane can assert timing from the page. `seq` rises on every change. `error` is Android's `TextToSpeech.ERROR_*` code.

Desktop answers `available:false` and rejects `speak` with `unavailable`.

## What the native side does

* One `TextToSpeech`, kept for the app's life. Audio attributes are `USAGE_ASSISTANT` with `CONTENT_TYPE_SPEECH`.
* Transient audio focus while speaking (`GAIN_TRANSIENT_MAY_DUCK`). Losing focus to a call stops the reading and the phase goes to `idle`.
* Text longer than the engine's input limit is cut at sentence ends and queued, and `done` fires after the last piece.
* One `TreehouseSpeak` log line per transition, with the id and elapsed ms, so a lane step reads it without the page.
* No foreground service. The engine plays in its own process, so a long reply while the app is cached is a thing to measure, not assume.

Step file: `scripts/ci/lane-steps/speak.json`, run through the [lane](android-emulator-lane.md).
