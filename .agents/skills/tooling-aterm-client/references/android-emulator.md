# The Android emulator for Frontend seats

A seat drives one live Android emulator running the real Treehouse app, beside the browser test bed (COI-3494). Browser emulation is not Android's WebView, and COI-3334's two-tap bug did not reproduce there. Code: `scripts/aterm_android_emulator.py`, tests `tests/test_aterm_android_emulator.py`.

Starting the whole rig and the card-tap recipe: [android-emulator-rig.md](android-emulator-rig.md).

## Verb

`just aterm-android-emulator <command>`. Run `acquire` first. Every command that touches the device needs the lock.

* `up` and `down` - start or stop the emulator. `up` refuses while the host is under pressure (exit 7), and does nothing when it is already up.
* `acquire [--wait S]` and `release [--force]` - the lock. `status` and `tunnel` need none.
* `install APK [--reinstall]`, `open`, `stop-app`, `clear-app` - the app, `dev.coilyco.treehouse`.
* `tap X Y`, `type TEXT`, `key NAME`, `keyboard state|close`, `shot OUT.png` - input and screenshots. A tap is an `adb shell input` touch, so the page sees what a finger sends. Open the keyboard by tapping a field.
* `devtools`, `eval JS`, `dom SELECTOR`, `tap-selector SELECTOR` - the app's WebView over DevTools, through `scripts/aterm_devtools.py`, the client the CI journey shares. The app's socket is named by its pid, never taken from the list, because a Google-APIs image runs other WebView hosts whose sockets can sort first. A starting app is waited for up to `TREEHOUSE_EMULATOR_DEVTOOLS_WAIT` seconds (15), and a refusal lists the pids and sockets (COI-3600). `tap-selector` reads the element's box over DevTools, finds the WebView on screen from the UI tree, and sends one real tap.
* `photo [--name NAME.png]` - puts a 512 pixel picture in the library and waits for MediaStore to list it, so the Photo Picker has something to show.
* `reverse PORT` - the guest's `localhost:PORT` reaches that port on the host running the adb server, as loopback.

## The lock

**It lives on the emulator**, as a directory in `/data/local/tmp`, so it binds every seat wherever the emulator runs. The holder is `TREEHOUSE_EMULATOR_HOLDER`, or `AOS_NATIVE_SESSION` in a native session, and the verb refuses with exit 2 when neither is set. Each driving command counts as activity. A lock idle past `TREEHOUSE_EMULATOR_LOCK_TTL` (900 s) is taken over by the next `acquire`, which says so. A held lock names its holder and exits 6.

## Where the emulator runs

**The address is a setting, so moving it is an environment change.** `TREEHOUSE_EMULATOR_SSH` set means a remote host. The verb opens one ssh tunnel from `ADB_PORT` (15037) to the host's adb server and from `DEVTOOLS_PORT` (19222) to its DevTools port, both loopback on each end, because adb has no authentication. It starts and stops the host's unit with `ssh <target> sudo -n /bin/systemctl start|stop <UNIT>`, the exact argv the host's sudoers matches. With no ssh target it uses the local adb server and `android emulator start --headless`. The other settings are `ADB`, `ADB_HOST`, `SSH_ADB_PORT`, `SSH_DEVTOOLS_PORT`, `SERIAL`, `AVD`, `UNIT`, `STOP_REASON` and `BOOT_TIMEOUT`, all `TREEHOUSE_EMULATOR_` variables. The adb client and the host's adb server must be one platform-tools version, because a mismatch restarts the server.

**The host can stop it.** A runtime cap and a pressure guard stop the emulator, and the guard writes why to the `STOP_REASON` file. The next command says the host stopped it and gives that reason, and `up` starts it again.

**The pressure guard** reads swap used and load per core on the emulator's host before `up` boots, and refuses above `MAX_SWAP_PCT` (75) or `MAX_LOAD_PER_CORE` (2.0). On 2026-11-09 at 19:43Z the Mac read 90% swap and load 24 to 30 on 11 cores, so the first host is not the Mac.

## The debuggable APK

**A release build may have no WebView DevTools.** wry 0.57.0 compiles `setWebContentsDebuggingEnabled` only under `debug_assertions` or the `devtools` feature. The 2026-11-09 release library had no such call, but the published v0.38.0 APK exposed a `webview_devtools_remote` socket and `eval` worked on 2026-11-10 (COI-2484), so run `devtools` before building anything. It says so when the app exposes no socket. For a debug APK, build it on GitHub, never on the Mac:

* Dispatch `gh workflow run aterm-android.yml -f debug_abi=x86_64` (or `aarch64`) and take the `aterm-android-debug-<abi>-<run id>` artifact with `gh run download`. A debug dispatch builds that and nothing else, so nothing publishes.
* **An x86_64 emulator needs an x86_64 build.** The app has no arch-specific native code (no `target_arch`, `abiFilters`, `jniLibs`, `ring` or `openssl` in `aterm-android`), so the same sources build for either ABI. The release path stays arm64-only, and the image's `ro.product.cpu.abilist` reads `x86_64,arm64-v8a`, so the arm64 release APK installs and runs by ARM translation (v0.38.0, `primaryCpuAbi=arm64-v8a`, COI-2484).
* The debug APK is signed by Gradle's debug key and is not the Play or Obtainium build. Each GitHub debug build has its own signing key, so a newer one, or a release build, needs `install --reinstall`, which clears the app's data.

## Reaching a daemon

**Loopback types with no passkey** ([tailnet-access.md](tailnet-access.md)), and a device key cannot be enrolled from an emulator, so the guest must reach a **test daemon** as loopback. Never `reverse` to a daemon that holds real seats, since every process in the guest could then type into them. The Android page is https, so `ws://` to `10.0.2.2` should be mixed content: expect `adb reverse` to the daemon's `ATERM_DAEMON_WS` port and a host of `localhost`. Unconfirmed until the first boot.

**A fixture seat is a `spawn` frame whose argv is a shell loop, with no agent harness.** The daemon gives it a fresh `ATERM_SESSION_TOKEN`, so `aterm ask` inside it raises a stamped choice card on every attached client and returns the pick. `just aterm-fixture-seats up|status|down` spawns `fixture-cards` (a three-option card, asked again after each answer), `fixture-quiet` (a heartbeat line) and the COI-3573 set. `fixture-reply` has a claude transcript beside the socket, so its session row carries `last_reply`. `fixture-free` asks with `--other` while it prints a line a second. `fixture-flip-a`, `-b` and `-c` raise a card and end it on their own every 7, 11 and 13 seconds, so seats flip between waiting and not. `fixture-wide` prints a 150 character line, and a silent 160 column client, started by `up` and stopped by `down`, holds its PTY wide while a phone is attached. It needs `ATERM_DAEMON_SOCKET` or `--socket` and refuses the default socket, which holds real seats. **The daemon's loopback listener refuses the app's origin**, `https://tauri.localhost`, with `X-Aterm-Refusal: origin` (observed against the real test daemon): it serves only loopback page origins. `scripts/aterm_test_rig_proxy.py --listen 127.0.0.1:7419 --upstream 127.0.0.1:7420` sits in front of the test daemon and rewrites that one Origin to a loopback one, so the shipped daemon is unchanged. It refuses a non-loopback address.

**The relay speaks websocket frames, because a session socket runs commands.** A `spawn` frame over the websocket runs any argv as the daemon's user, and the app's own Terminal tab is one (`kind: terminal`, a login shell). The relay refuses client `spawn` and `launch` in every form, fragmented or not and with any key case, and answers a daemon-shaped `error` with `reason: rig_refused`. It drops the connection on anything it cannot parse: binary, an unmasked frame, a reserved bit, a frame with no single string `type`. It serves only a websocket upgrade on `/` and `GET /_aterm/reach`, so the CDP and gateway paths are 403. Sent straight to a throwaway daemon, the same `spawn` is executed, which is why the refusal is the relay's. `SHELL=/usr/sbin/nologin` in the daemon unit also blocks the Terminal tab, and the relay does not depend on it.

**Dropping the daemon's TCP port by bridging its unix socket was judged and not done.** The socket speaks newline JSON and the websocket speaks frames, and the daemon adds `typing-guard`, `passkey` and `typing {allowed, reason, passkey}` to `welcome` for a websocket only (aterm-daemon.md). A bridge would show the app a different daemon from the real one. The residual is that any local process can reach the daemon's port, with a loopback Origin it sets itself.

The test daemon runs `aterm daemon --tailnet-port ""` with its websocket on the upstream port, a 0700 socket directory and `aterm` on the seats' PATH, from `aos-v0.540.0` (the `ask` frame is older, `aos-v0.376.0`, and the app origin is `aos-v0.462.0`).

## Verified on the kai-server emulator, 2026-11-09

Android 15 google_apis x86_64, WebView Chrome 124, 412 by 915 CSS px at 2.625 DPR, `hover: false`. A second holder is refused with exit 6 naming the first. The 130 MB debug APK installs in about 16 s over the tunnel. `devtools` reads `https://tauri.localhost/` and `tap-selector` taps it for real: a stray tap toggled a switch in the app. A tap on the host field opened Gboard, `type` filled it, `keyboard close` hid it. **The keyboard overlaid the page and `visualViewport.height` stayed at 915**, so this WebView did not resize the layout for it. One emulator, one read.

* `tap-selector` taps the first match, so pick the selector from `eval` of `document.querySelectorAll`. It refuses an element with no size, which a hidden `type=file` input is.

## Not verified yet

* The COI-3334 one-tap check in the real app.
* A seat attached to a test daemon from the emulator, and the `adb reverse` expectation above.
