# Dictating into the composer on Android

A mic button in the composer turns speech into text in the draft, to fix before Send (COI-3460). Wispr Flow pasted into whichever box had focus and lost it mid-sentence. Here Treehouse owns the audio and the box.

**What a person sees.** A mic sits beside Add image, shown only where the app has a speech recognizer. A tap asks for the microphone once, then listens, and the mic turns brand-coloured. Words show in the draft as they are heard, at the caret, spaced from the text around them, replacing a selection. A second tap stops, and the last words arrive. Typing mid-dictation keeps the typed text and drops later speech. Send ends a session first, so late speech cannot bring a sent message back. A refused microphone, no recognizer, no speech heard and a dead network each say so in words, and the draft stays.

**The plugin is `aterm-android/plugins/tauri-plugin-dictation`.** Kotlin holds one `SpeechRecognizer`, made and called on the main thread, with partial results on. The page calls `available`, `start`, `poll`, `stop` and `cancel`, and **polls every 150 ms. No event channel exists**, as in the [share target](android-sharetarget.md), so the page needs no event API and the bridge stays `invoke` only. `poll` answers a phase (`idle`, `listening`, `done`, `failed`), the words so far, Android's error code and a `seq` that rises on each change. The words heard before an error stay. Kotlin passes the error code through, and `lib/dictation.ts` says each in words, so the wording is tested in vitest. `RECORD_AUDIO` and the `RecognitionService` visibility query come from the plugin's manifest, and the grant is asked on the first `start`.

**The Mac app answers `available: false`, so it shows no mic.** Apple's Speech framework needs a Swift bridge on a surface that waits on COI-3471.

## The engine, and what reverses it

**Picked, 2026-11-10: the phone's own `SpeechRecognizer`, with no server in the path** (Jev jev-1.14.0, `on_device_first` 1.00 over `server_first` and `both_now`). Kai's ruling on 2026-11-09 set the order: the operating system's recognition first, a setting that switches to ElevenLabs later, Wispr Flow out. The evidence for building the first slice that way:

* **Agent Proxy has no speech route** at origin/main `d122685` (2026-11-04). It serves chat completions, completions, embeddings, `systemone` and models, and a search for audio, whisper, speech and transcription finds nothing. ElevenLabs Scribe v2 Realtime is a WebSocket stream, which none of those HTTP routes carry.
* A server path needs three new pieces before one word appears: a streaming route, a daemon audio frame, and capture in the app. The recognizer path is one plugin.
* **The CI emulator image carries two recognition services** (Android 14 `google_apis` x86_64: Android System Intelligence and Google's text-to-speech app), and a phone with Google's app has the same.

**Case against.** The recognizer ends a session at a pause, so this is one utterance per tap and not continuous dictation. It sends audio to Google's service unless an on-device pack is installed, and accuracy on code words is unmeasured. **Reverses to the server engine when** Kai's use on her Pixel shows the cutoffs or accuracy are not good enough, or when the daemon or Agent Proxy carries a streaming speech route. The seam is `startDictation` in `lib/dictation.ts`: the page already speaks only `onText` and `onEnd`, so a WebSocket engine returns the same session and the Composer does not change. The setting is the Composer's to add.

## Not yet checked

* Any run on a phone: the microphone prompt, Google's recognizer, and a real sentence. **Kai's Pixel is the check.**
* Continuous dictation, a spoken end that sends, and answering a choice card by voice. None is built.
* The Rust and Kotlin tests: `cargo test` ran green in the plugin directory on the Mac, and CI builds the Kotlin but runs no unit test for any plugin.
