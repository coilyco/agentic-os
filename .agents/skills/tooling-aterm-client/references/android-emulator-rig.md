# Starting the emulator rig, and checking a card tap

How a Frontend seat brings up the whole Android rig and runs COI-3334's one-tap check in the real app. The verb, the lock and what each command does are in [android-emulator.md](android-emulator.md) (COI-3494).

## Start and stop

The host runs three units: the emulator, a test daemon (`aos-v0.540.0`, loopback port 7420, no tailnet listener, `SHELL=/usr/sbin/nologin`) and this repo's relay on 7419. Starting the daemon unit starts the emulator, and stopping the emulator stops all three. They also stop themselves at a runtime cap and when the host's guard fires, and the next command says why.

* **Start:** `TREEHOUSE_EMULATOR_SSH=<user>@<host> TREEHOUSE_EMULATOR_UNIT=treehouse-test-daemon.service just aterm-android-emulator up`. It checks the host's pressure first, starts the unit and waits for boot.
* **Then:** `acquire`, `reverse 7419`, `install --reinstall APK` (a debug build from GitHub, see android-emulator.md), `open`. In the app add the host `ws://127.0.0.1:7419` once. A bare name would become `wss://`. The installed app and its saved host survive a stop and start of the rig, and `install --reinstall` and `clear-app` clear them. Measured 2026-11-09: `down` 17 s, `up` 28 s.
* **Stop:** `release`, then `down` with the default unit, which stops the emulator and so all three. Stop it when finished, since other services share the host.

## Check a card tap

The test daemon holds two fixture seats. `fixture-cards` asks a three-option card again after each answer.

* Open the seat with `tap-selector 'button.tab.seat'` and **stay on the Messages tab. Never open the Terminal tab:** the rig refuses a shell, but taps meant for the card land on its keyboard.
* `eval` a capture-phase recorder for `pointerdown`, `touchend`, `click` and `focus*`, plus a `MutationObserver` counting `button.option`.
* Before each tap, check the option is under its own centre with `document.elementFromPoint`, then send one `tap-selector 'li:nth-child(N) button.option'`.
* Read the record. One tap that works shows `click` and then the options removed. A tap that needed a second one shows no `click`, or options that stay.
* The seat's screen, `.xterm-rows` in the page, holds each pick that `aterm ask` printed. Compare it with the options tapped.

Result on 2026-11-09, build with #276, #280 and #287: six single taps in the real app, six picks received in order. That is the after-fix half only, on an emulator and not on Kai's phone.
