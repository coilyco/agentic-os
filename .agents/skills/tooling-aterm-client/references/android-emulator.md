# The Android emulator for Frontend seats

A seat drives one live Android emulator running the real Treehouse app, beside the browser test bed (COI-3494). Browser emulation is not Android's WebView, and COI-3334's two-tap bug did not reproduce there. Code: `scripts/aterm_android_emulator.py`, tests `tests/test_aterm_android_emulator.py`.

## Verb

`just aterm-android-emulator <command>`. Run `acquire` first. Every command that touches the device needs the lock.

* `up` and `down` - start or stop the emulator. `up` refuses while the host is under pressure (exit 7), and does nothing when it is already up.
* `acquire [--wait S]` and `release [--force]` - the lock. `status` and `tunnel` need none.
* `install APK`, `open`, `stop-app`, `clear-app` - the app, `dev.coilyco.treehouse`.
* `tap X Y`, `type TEXT`, `key NAME`, `keyboard state|close`, `shot OUT.png` - input and screenshots. A tap is an `adb shell input` touch, so the page sees what a finger sends. Open the keyboard by tapping a field.
* `devtools`, `eval JS`, `dom SELECTOR`, `tap-selector SELECTOR` - the app's WebView over DevTools. `tap-selector` reads the element's box over DevTools, finds the WebView on screen from the UI tree, and sends one real tap.
* `reverse PORT` - the guest's `localhost:PORT` reaches that port on the host running the adb server, as loopback.

## The lock

**It lives on the emulator**, as a directory in `/data/local/tmp`, so it binds every seat wherever the emulator runs. The holder is `TREEHOUSE_EMULATOR_HOLDER`, or `AOS_NATIVE_SESSION` in a native session, and the verb refuses with exit 2 when neither is set. Each driving command counts as activity. A lock idle past `TREEHOUSE_EMULATOR_LOCK_TTL` (900 s) is taken over by the next `acquire`, which says so. A held lock names its holder and exits 6.

## Where the emulator runs

**The address is a setting, so moving it is an environment change.** `TREEHOUSE_EMULATOR_SSH` set means a remote host. The verb opens one ssh tunnel from `ADB_PORT` (15037) to the host's adb server and from `DEVTOOLS_PORT` (19222) to its DevTools port, both loopback on each end, because adb has no authentication. It starts and stops the host's unit with `ssh <target> sudo -n /bin/systemctl start|stop <UNIT>`, the exact argv the host's sudoers matches. With no ssh target it uses the local adb server and `android emulator start --headless`. The other settings are `ADB`, `ADB_HOST`, `SSH_ADB_PORT`, `SSH_DEVTOOLS_PORT`, `SERIAL`, `AVD`, `UNIT`, `STOP_REASON` and `BOOT_TIMEOUT`, all `TREEHOUSE_EMULATOR_` variables. The adb client and the host's adb server must be one platform-tools version, because a mismatch restarts the server.

**The host can stop it.** A runtime cap and a pressure guard stop the emulator, and the guard writes why to the `STOP_REASON` file. The next command says the host stopped it and gives that reason, and `up` starts it again.

**The pressure guard** reads swap used and load per core on the emulator's host before `up` boots, and refuses above `MAX_SWAP_PCT` (75) or `MAX_LOAD_PER_CORE` (2.0). On 2026-11-09 at 19:43Z the Mac read 90% swap and load 24 to 30 on 11 cores, so the first host is not the Mac.

## The debuggable APK

**A release build has no WebView DevTools.** wry 0.57.0 compiles `setWebContentsDebuggingEnabled` only under `debug_assertions` or the `devtools` feature, and the release profile has neither. `devtools` says so when the app exposes no socket. Build a debug APK on GitHub, never on the Mac:

* Dispatch `gh workflow run aterm-android.yml -f debug_abi=x86_64` (or `aarch64`) and take the `aterm-android-debug-<abi>-<run id>` artifact with `gh run download`. A debug dispatch builds that and nothing else, so nothing publishes.
* **An x86_64 emulator needs an x86_64 build.** The app has no arch-specific native code (no `target_arch`, `abiFilters`, `jniLibs`, `ring` or `openssl` in `aterm-android`), so the same sources build for either ABI. The release path stays arm64-only.
* The debug APK is signed by Gradle's debug key and is not the Play or Obtainium build. Installing it over a release build needs an uninstall first.

## Reaching a daemon

**Loopback types with no passkey** ([tailnet-access.md](tailnet-access.md)), and a device key cannot be enrolled from an emulator, so a guest that reaches a daemon as loopback is the route. An emulator host with its own test daemon is reached at `10.0.2.2`. Never `reverse` to a daemon that holds real seats, since every process in the guest could then type into them.

## Not verified yet

* An APK installed and opened in the emulator, and `devtools` reading the DOM.
* The COI-3334 one-tap check in the real app.
* A seat attached to a daemon from the emulator. The test daemon is an open decision on COI-3494.
