# Treehouse for the Mac

A windowed Tauri 2 app around the same client bundle the [Android app](android.md) ships (COI-3471). It is built from `aterm-android/`, whose name predates it (COI-3338). It opens at login on a `prod-director` seat and replaces the Portfolio Director kitty launcher as the login item.

**The identity is productName `Treehouse` and bundle id `dev.coilyco.treehouse.desktop`.** The signed CLI wrapper `Treehouse.app` that ships inside the aos formula already holds `dev.coilyco.treehouse`, and macOS privacy grants attach to that id (COI-50), so the windowed app must not take it. `tauri.conf.json` keeps `dev.coilyco.treehouse` for Android, where it is the permanent application id. `tauri.macos.conf.json` is merged over it on macOS only and sets the Mac name and id. The wrapper is untouched. The two bundles share a file name and sit in different folders, the wrapper in the Cellar and the app in `~/Applications`.

**Tauri's `"targets": "all"` does build a macOS target.** `just treehouse-mac-build` produced a 6 MiB arm64 `Treehouse.app` with only the Command Line Tools, no Xcode. There is no macOS CI path, so a Mac build is the proof (COI-3247 allows no macOS runner).

## What it does at launch

`src-tauri/src/desktop.rs` runs before the window exists.

* **It waits up to 30 seconds for the daemon**, since the daemon LaunchAgent and a Login Item start together, then asks `aterm agents --json` for a live `prod-director`.
* **It opens one headless when there is none**, with `aterm --headless prod-director`. A live director is never duplicated.
* **It opens the window with an initialization script** that adds `this Mac` (`ws://127.0.0.1:7419/`) to `aterm.hosts.v1` when absent, and on the first load of the window writes `aterm.last-open.v1` with the host and no seat, so the app rests on the seat overview and not on the director's terminal (COI-3518, COI-3579). The director is still kept live for the overview to list. A host the person added stays.

The loopback daemon accepts the webview's `tauri://localhost` origin, since its origin check reads the host and not the scheme. **A second target is a saved host**, typed in the host panel and kept on the device, so a cloud target needs no rebuild (COI-3469). Discovery, the broker and a `wss` host work as in any app build.

## Commands

**A seat does not run the install or the login swap.** The umbra fs guard refuses a write under `~/Applications`, and a login item is Kai's. The infrastructure `treehouse-mac` role does both, and `just mac-converge apply` in infrastructure runs it outside the session with the real `HOME` (COI-3234). It rebuilds only when the `aterm-android/` or `aterm-client/` tree on main changed, defers the install while Treehouse is open, and prints `login: changed` or `login: unchanged`.

* `just treehouse-mac-test` runs the shell's Rust tests.
* `just treehouse-mac-build` builds the client for the app, builds the bundle, and signs it under the bundle id. The version shown is the newest `aterm-android` release tag, or `TREEHOUSE_VERSION` where the tree has no tags.
* `just treehouse-mac-sign-setup` imports the signing identity from SSM. Run it once, from a session with AWS access.
* `just treehouse-mac-install` copies it to `~/Applications/Treehouse.app`. The role calls it.
* `just treehouse-mac-login on` adds Treehouse, then removes Portfolio Director, and takes Treehouse off again if the removal fails, so there is one. `off` reverses both. `status` lists them. The role calls it.

## Not yet

* **The signing identity is COI-50's self-signed certificate** (`Coilyco Code Signing`, in SSM under `/coilysiren/codesign/`), imported once by `just treehouse-mac-sign-setup` into a keychain of its own, so an unattended build needs no AWS session. A rebuild keeps the same designated requirement, which `treehouse-mac.sh build` prints. Rotating the certificate expires every grant. With no identity the build fails, and `TREEHOUSE_SIGN_ADHOC=1` allows a throwaway ad-hoc one.
* **The seed writes the client's storage keys** from outside it. A build-time local host and an open-this-role setting in the client would replace it.
* **The Cargo package is still `aterm-app`**, a code name nobody reads. The executable inside the bundle is `Treehouse` (`mainBinaryName` in `tauri.macos.conf.json`), so the process list says that (COI-3550).
* **The Android build was not re-run** after the `cfg(desktop)` split in `lib.rs`. Its next scheduled run is the check.
