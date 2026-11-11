# Saving a file in the app

A Tauri WebView drops a blob download without a word, and has no Web Share (checked on the emulator, COI-3593). So the app saves through its own `savefile` plugin, `aterm-android/plugins/tauri-plugin-savefile`, and the media viewer's Download calls it.

## What a person sees

* **In the app with the plugin** - Download on a file card writes it into Downloads and the line under the buttons says `Saved print-log.gcode to your Downloads folder.` A name already taken is numbered by the system, `print-log (1).gcode`, and the line names the saved one. Progress reads `Saving print-log.gcode, 40%.` and the button is off until it ends.
* **When a write fails** - `print-log.gcode (1 MB) was not saved: No space left on device. Nothing was left in Downloads.` The half file is cancelled and Download works again.
* **In an app without the plugin** - the older card, with no button and the sentence that the app cannot save files yet. The page asks `available` once, so an old APK reads the same as before.
* **In a browser** - unchanged, the `<a download>` path.

## The plugin

Five commands over the page's `invoke`: `available`, `begin {name, mime}`, `write {id, data}`, `finish {id}`, `cancel {id}`. A save is pieces, never one blob, so a video does not sit in one bridge message. The page reads the bytes it already holds, 384 KB at a time, base64.

* **Android** - `MediaStore.Downloads` with `IS_PENDING` set until `finish`, so a half file never shows in Files. It needs no storage permission on Android 10 and later, and the app's minSdk is 30.
* **Mac** - the same commands write into `~/Downloads` with a created-new file, numbered on a clash.
* **The type** - the Android writer stores the type the name's extension maps to, or plain bytes. MediaStore appends an extension to match a stored type, and the emulator turned `print-log.gcode` sent as text/plain into `print-log.gcode.txt`.
* **The name** - `safe_name` in `rules.rs` and `SaveRules.kt` hold one rule: no folders, no control characters, no leading dots, never empty, 120 bytes with the extension kept. Both tests carry the same cases.
* **Limits** - four saves open at once, then `busy`. Errors reach the page as `{code, message}`.

## What is checked

Rust and Kotlin rule tests, the plugin's Mac writer against a temp folder, `save-file.test.ts`, and the `media-viewer` journey with a faked bridge. The Android writer is run on the emulator rig, and the result is on COI-3593.
