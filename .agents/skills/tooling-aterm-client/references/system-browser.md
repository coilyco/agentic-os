# Signing in through the system browser

The Treehouse apps open the accounts sign-in page in the system's own browser and hear when it comes back (COI-3657). A Custom Tab on Android and an `ASWebAuthenticationSession` on a Mac, so the passkey sheet and the saved passkeys are the system's (RFC 8252). The page that calls it lives in coilyco/treehouse-servers (`src/lib/system-browser.ts`). The plugin is [`aterm-android/plugins/tauri-plugin-systembrowser`](../../../../aterm-android/plugins/tauri-plugin-systembrowser/), beside the share target's.

**What the page calls**, all through `window.__TAURI__.core`:

* `invoke("plugin:systembrowser|open", {url})` resolves when the browser is up, not when it ends. It rejects `{code, message}` with a fixed phrase and never the address.
* `invoke("plugin:systembrowser|take_return")` gives the pending `dev.coilyco.treehouse:/signin?...` address once, then null. It holds the address until taken. Android needs it for a cold start, and a Mac always answers null because the session dies with the app.
* `addPluginListener("systembrowser", "return", cb)` with `{url}`, and `"closed"` with `{}`. The page registers both before `open`. Android's base plugin class answers the listener commands, and `listeners.rs` answers them on a Mac.

**One rule decides what opens, in `rules.rs`:** https, host exactly `accounts.coilyco.ai`, no port or credentials, path `/signin`. A testbed on `http://localhost` is refused on purpose, and the page's testbed fakes the bridge instead. The Rust command checks it before Kotlin is called, so Kotlin holds no copy. Each side keeps its own return match, because each hears a different thing: an intent on Android and a completion handler on a Mac.

**`closed` differs by platform.** A Mac reports a cancel itself. Custom Tabs has no dismissal callback, so Android calls it a dismissal when the app resumes after the browser covered it and no return arrived within one second. The page keeps its Cancel button and ten minute timeout for the cases that misses.

**The address is never logged**, since it carries a one-use code. `Debug` on the payload types hides it, a refusal never repeats it, and Kotlin rejects with its own phrases rather than an exception's text. `models.rs` pins the first.

**The scheme.** `dev.coilyco.treehouse` is the Android application id and the Mac bundle id is `dev.coilyco.treehouse.desktop`. Android's manifest filter matches the scheme only, since a filter cannot match a path on an address with no host, so `SignInRules.kt` checks `/signin`. A Mac needs no bundle registration because the session matches the scheme itself.

**The card needs two addresses at build time.** The page shows the sign-in card only when `VITE_TREEHOUSE_ACCOUNTS_URL` and `VITE_TREEHOUSE_BROKER_URL` are both set. `just aterm-client-build` sets them to `https://accounts.coilyco.ai` and `https://broker.coilyco.ai` for an app build (`VITE_ATERM_APP` set) unless the caller sets them, and a value set empty opts out. Android's CI build and the Mac build both go through that recipe, so the two names live there and nowhere else (COI-3657).

**Two dependencies are new.** `objc2-authentication-services` 0.3.2 on a Mac, from the repository that already supplies `objc2` (last commit 2026-10-16, two crate owners, Apache-2.0 or MIT or Zlib, 1.18 million downloads). `androidx.browser:browser` 1.8.0 on Android, a Google AndroidX library under Apache-2.0. It is not 1.10.0 because the plugin pins Android Gradle plugin 8.0.2.

## The page side

`system-browser.ts` and the hop in `app.svelte.ts` (COI-3653) call it, and `handoff.ts` makes the verifier, state and S256 challenge. The contract is `docs/contract-flows.md`, "Sign in from an app", in coilyco/treehouse-servers.

* **Opens** `https://accounts.<domain>/signin?redirect_uri=&state=&challenge=` with return `dev.coilyco.treehouse:/signin`. The card shows a waiting state with Cancel. A refused address shows the plugin's phrase after "This app could not open the sign-in page".
* **Returns** - the page checks `state`, trades `code` and the verifier at `POST /v1/auth/handoff` for a new session, and lists the machines. A return it is not waiting for is dropped.
* **Gives up** on Cancel, on `closed` (nothing said), and after ten minutes ("The sign-in took too long").
* **A killed app** - the verifier is kept in local storage for ten minutes. At every app start the page calls `take_return`, and a kept verifier with a return trades it. One without a return goes on waiting.
* **Build** - the app needs `VITE_TREEHOUSE_ACCOUNTS_URL` and `VITE_TREEHOUSE_BROKER_URL`. Without both, the card is absent.

## Checked and not checked

* **Checked:** `cargo test` in the plugin runs nine tests for the address rule, the listener registry and the hidden `Debug`. The app shell compiles with the plugin on a Mac, and the bundle builds and passes `codesign --verify`. The debug APK from dispatch 43126130895 (main 21d4b5a5), read with `aapt2 dump xmltree`, has the scheme filter on `MainActivity` (`singleTask`), and its dex holds `SystemBrowserPlugin`, so the Kotlin compiles.
* **Not yet checked on a device:** the Custom Tab opening, a return reaching a warm and a cold app, Android's `closed`, and the listener commands falling through to Kotlin as the notification plugin's do. `SignInRulesTest` has not been run.
* **Not yet checked on a Mac:** the sheet appearing over the Treehouse window, a completed sign-in arriving as `return`, a cancel arriving as `closed`, and the Mac needing no bundle registration.
