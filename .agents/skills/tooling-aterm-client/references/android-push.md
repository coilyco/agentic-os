# Push to a closed Android app

The Treehouse Android app shows a seat's push with the app swiped away, through FCM (COI-3748, COI-3750). A Tauri webview cannot hold a Web Push subscription, so the app carries its own plugin, [`aterm-android/plugins/tauri-plugin-push`](../../../../aterm-android/plugins/tauri-plugin-push/). The broker sends the message (COI-3751), and the page turns push on and routes a tap (COI-3752).

## What the page calls

All through `window.__TAURI__.core`, like the [system browser](system-browser.md):

* `invoke("plugin:push|register")` resolves `{token, device}`, the body of the broker's `treehouse.push.app-register.v1` less `platform`. It makes the FCM token on first call. It rejects `{code: "push_off"}` in a build with no Firebase config and on a Mac, and `{code: "no_token"}` when the push service gives none. The page calls it at every launch with push on, then `POST /v1/push/apps`. `device` is the phone's model, at most 64 characters.
* `invoke("plugin:push|token")` gives the last token or null, with no network.
* `invoke("plugin:push|take_tap")` gives the tap that started or woke the app once, then null: `{kind, daemon_id, session, tag}`. `session` is null for a `reachable` push, whose tap opens that machine's list rather than a seat.
* `addPluginListener("push", "tokenRefresh", cb)` with `{token}` when FCM rotates it, so the page registers it again. `addPluginListener("push", "tap", cb)` with the same shape as `take_tap`, for a warm app.

## What the phone does with a message

The message is data only, so FCM wakes the app's own `PushService` even when the app is killed, and no page is involved. The data map is the broker's `treehouse.push.payload.v1` with string values. The contract is in coilyco/treehouse-servers `contract/` (main 319fcae): `app-register.v1`, `app-unregister.v1`, `fcm.v1` (data only, priority HIGH, TTL 86400 s, no `collapse_key`) and `payload.v1`.

* **Dropped unshown:** a message without `daemon_id` or `tag`, or a `prompt`, `ask` or `done` without `session`, because a tap could not open anything.
* **Shown:** `title` (or "Treehouse") and `body` under the notification tag `tag`, so a second push for a seat replaces the first. `done` goes to the low-importance "Seats done" channel and stays silent. Every other kind, an unknown one included, goes to the high-importance "Seats asking" channel.
* **Not shown while the app is on screen**, which shows the seat itself, as the web worker does for a focused window.
* **The small icon** is `ic_stat_treehouse`, the canopy the build copies in for the [notification plugin](native-notify.md) (COI-3613).
* **A tap** opens the app with the tap in the launch intent, under its own action, because an intent that matches the launcher's resumes the task without its extras. A cold start sends it through `onNewIntent` before any plugin is registered, so `MainActivity` keeps it as the activity's intent and the plugin reads it at load. The plugin holds it for `take_tap` and sends `tap`, then clears it so a recreated activity does not see it again.
* **Permission** is the notification plugin's one ask. Without it nothing shows.

Nothing in the plugin logs the token or a message, and a rejection never repeats an exception's text. `models.rs` pins the hidden `Debug`.

## Config per deployment

The Firebase project is per deployment, so nothing of one deployment is compiled in.

* **The source** is the deployment's `google-services.json` in the build environment as `ATERM_ANDROID_GOOGLE_SERVICES_JSON`. For coilyco that is SSM `/coilysiren/treehouse/fcm/google-services` (COI-3753), synced to the Actions secret of the same name. It is never committed.
* **The build** (`push_patch` in `scripts/ci/aterm-android.sh`) turns it into `res/values/treehouse_push.xml` with `scripts/ci/aterm_google_services.py`, the values the google-services Gradle plugin would write. So the generated Android project needs no Gradle plugin, and `FirebaseInitProvider` starts Firebase from those values.
* **Unset means push off.** The build says so and goes on, Firebase does not start, and `register` rejects `push_off`. A JSON with no Android client for `dev.coilyco.treehouse` fails the build instead.
* **No token until asked.** `firebase_messaging_auto_init_enabled` is false, so a person who never turns push on never gets a token. `register` turns auto init on for rotation.
* **Firebase Messaging and nothing else.** The one Firebase dependency is `firebase-messaging`. The rig APK of 70d64860 holds its own transitive pieces (common, installations, datatransport, iid-interop, and `measurement-connector`, an interface with no Analytics behind it), and no Analytics, Crashlytics or other Firebase SDK. The manifest still turns Analytics collection off and deactivated, which [Google's FCM setup](https://firebase.google.com/docs/cloud-messaging/android/client) asks for beside auto init, and keeps the [BigQuery delivery export](https://firebase.google.com/docs/cloud-messaging/understand-delivery?platform=android) off.
* **The fixture.** A debug build dispatched with `push_fixture` takes a made-up config (`push-fixture`) when no real one is set. Firebase starts, so a message delivered on the device renders and taps, but it reaches no FCM and gives no token. A release build refuses it.

## Why a plugin of our own

The three community plugins were audited first (2026-11-25):

* **tauri-plugin-remote-push** - its crate names `YOUR_USERNAME` as its repository, the owner's repository answers 404, and ten releases landed in two days of June 2025 with none since.
* **tauri-plugin-mobile-push** - one maintainer, eleven commits, nothing since April 2026.
* **tauri-plugin-notifications** (`push-notifications`) - active, but one maintainer, only release candidates, and a fork that would replace the official notification plugin.

All three hand a message to the live plugin instance, which is null in a killed app, so none shows a data message with the app closed. The one new dependency is `com.google.firebase:firebase-messaging` 25.1.3 from Google's Maven, Google's own FCM client under Apache-2.0.

## Checked and not checked

* **Checked:** `just aterm-android-plugin-test push` (three tests: the hidden token, the Kotlin answers, both tap shapes) and `tests/test_aterm_android_push.py` (the config step on and off, a broken config, the fixture, and a release refusing it).
* **Checked on the emulator rig** (2026-11-26, Android 15 `google_apis`, the `push_fixture` debug build of 70d64860, commands and output on agentic-os PR #466). With the process killed, an FCM data message sent by root broadcast to `FirebaseInstanceIdReceiver` started the app and showed "Fixture Cards is asking" under its tag, on the alert channel, with `ic_stat_treehouse`. A tap opened the app, and `take_tap` gave that seat once, then null. With the app alive in the background, a tap sent `tap` and `take_tap` both. Nothing showed with the app on screen. `done` landed on the quiet channel, `reachable` showed with no seat, and an `ask` without `session` was dropped. Logcat held neither the body nor a seat name.
* **Not checked:** a message delivered by FCM itself, and a real token. The fixture reaches no FCM (`register` rejects `no_token`), so both wait for Kai's Firebase project (COI-3753) and the broker's send (COI-3751). Nor a phone, or Firebase's own log line, which prints the project's options (not the token) when it reaches its servers.
* **Not checked by CI:** `PushMessageTest.kt`, like the other plugins' Kotlin tests (COI-3503).
