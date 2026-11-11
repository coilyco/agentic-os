# Native notifications in the Mac and Android apps

A coordinator's `notify_kai` reaches the page as a `notify` frame ([web-push.md](web-push.md), [seat-state.md](seat-state.md)). In a browser the page raises a `Notification`. The apps have no web notifications, so the shell carries Tauri's official notification plugin (COI-3578).

## What it does

* **Shell** - `tauri-plugin-notification` is pinned in `aterm-android/src-tauri/Cargo.toml` and registered in `lib.rs`. `capabilities/default.json` grants only `is-permission-granted`, `request-permission`, `notify`, `get-active`, `register-listener` and `remove-listener`.
* **Page** - `aterm-client/src/lib/native-notify.ts` reaches the plugin through `window.__TAURI__.core` (`withGlobalTauri`), so no npm package is added. `handleNotify` in `push.svelte.ts` plays the chime, then shows the system notification only while the person is not on that seat and the system allows it.
* **Asking** - the page asks the system once per launch, once a host is attached and the page is on screen, so Android's prompt has an activity to show over. A person who said no is not asked again from here, and the chime still plays.
* **Tap** - on Android a tap on a plain notification comes back as the plugin's `actionPerformed` event, but with no notification or extras in it: the plugin never fills `sourceJson` (checked in 2.7.0, 2.8.0 and 2.8.1, and seen on the emulator as `{actionId: "tap"}`). The page therefore gives each seat its own notification id, records what it raised in `aterm.notified.v1`, and on a tap asks the plugin what still shows (`get_active`). The newest raised notification that is gone is the one tapped, and that seat opens. Swiping away a newer notification and then tapping an older one can open the wrong seat. On macOS the plugin delivers no click to the page, so a click only brings the app forward.

## Not verified

* The Mac app, which needs a Mac run. Its ad-hoc signature also resets macOS privacy grants (COI-3477).
* A phone. The Android check is on the emulator rig, with `POST_NOTIFICATIONS` granted through the prompt.
* A brokered phone shows the broker's wording (COI-3557).

## Supply chain

The plugin is `tauri-apps/plugins-workspace`, owner `tauri-bot`. It adds `notify-rust` and `mac-notification-sys` (owner `hoodie`, 2015 and 2017) for desktop, and `zbus` 6 with `zbus_utils` and `zruntime` (owner `zeenix`, `z-galaxy` org) for Linux. None is compiled into the Android build.
