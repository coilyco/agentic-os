# The aterm client test bed

Seats drive the real aterm client in a browser against a mock daemon, at desktop and phone sizes, so a quirk shows up on a screenshot before Kai has to report it. It lives in `aterm-client/testbed/` and needs no live seat and no phone.

## Run it

* `just aterm-client-testbed` - install, fetch browsers, build, run every journey. The first run writes the baselines. Later runs diff against them.
* `just aterm-client-testbed --project=iphone` - one project. Pass `--grep @quirk` for the reported quirks only, or `--grep-invert @quirk` for the journeys that hold today.
* `just aterm-client-testbed-update` - accept what the bed shows as the new baselines. Read `git diff --stat aterm-client/testbed/baselines` first.
* `just aterm-client-testbed-report` - open the last run's report with each failing step's baseline, new screenshot and diff.
* `just aterm-client-testbed-serve` - serve the mock on a port, to look at a screen by hand.

Every step also saves its screenshot, pass or fail, under `aterm-client/testbed/out/results/`.

## How it works

* **Mock daemon** - `mock-daemon.ts` serves `aterm-client/dist` at `/` and speaks `aterm.daemon.v1` on a websocket at the same origin, as the real daemon does, so the client needs no configuration. It scripts only frames the daemon has: `welcome`, `sessions`, `roster`, `attach` replay, `size`, `output`, `ask`, `asked`, `launch`, `subscribe` to `open_url`, and a seat's browser (`browser_watch`, `browser_control`, `browser_navigate`, `browser_state`, no frames). A frame it does not model answers with an `error`. It records every `input`, `answer` and `launch` for assertions.
* **Scenes** - `scenes.ts` draws a seat's screen at whatever size the client asks for, as a harness redraws on resize. Claude Code's AskUserQuestion dialog is emulated from real captures, so a click on a card walks the dialog.
* **Captures** - `fixtures/captures/` holds real Claude Code 2.1.318 screens, replayed verbatim at their own width on desktop. Add one by dropping a plain-text screen in that folder.
* **Roster** - the mock serves `src/lib/fixtures/roster.json`, so one fixture has one owner.
* **Another build** - `ATERM_TESTBED_DIST=<dir>` points the bed at any built client, to try a fix before it lands. Add `ATERM_TESTBED_COMPARE=0` to judge its behavior and ignore how it now looks, since a fix changes pixels. Run the update verb afterward to take the new baselines.

## Projects

* `desktop-1440` and `desktop-1920` - Chromium. The side panel starts open, which is itself COI-3238 item 2.
* `iphone` - iPhone 15 emulation on WebKit, Safari's engine.
* `android` - Pixel 7 emulation on Chromium.
* `phone-320` - the narrowest phone, Chromium.

## Quirks go green

A journey tagged `@quirk` is a reported quirk. It fails now and goes green when the fix lands, so `just aterm-client-testbed` ends red while any quirk stands. Run it before pushing a fix. A fix also changes screenshots, so run the update verb, read the diff, and commit the new baselines with the fix.

State on main after PRs 193, 199, 203 and the find control, as the bed shows it, not as assumed:

* **Green** - COI-3243 bugs 2, 3 and 4 (`native-choice`, `native-choice-captured`), COI-3238 item 1's box drawing gap and find control (`terminal-text`), and COI-3238 item 2 (`side-panel`).
* **Red, client half only** - COI-3243 bug 1 (`dialog-input-hold`). A composer message sent while a dialog is open goes into the dialog. The daemon half cannot be tested on a mock that would only assert itself.
* **Red** - COI-3238 item 3 (`seat-rows`), the dialog's tab bar as buttons (`native-choice`), and a short conversation growing the PTY without limit (`pty-size`, COI-3291).
* **Fixme, bed cannot judge yet** - link hover and history depth in `terminal-text` (COI-3293).

Journeys that hold today: `unlock-resume` (a locked device redials inside and after the resume window, COI-3305), `read-output`, `switch-seats`, `ask-card`, `native-choice` (one question), `start-seat`, `composer-keyboard`, `stream-under-load`, `open-url`.

## What it cannot tell you

* **Not a real phone** - WebKit emulation is not iOS. The keyboard journey shrinks the viewport by a keyboard's height, which a real iOS keyboard does not match. A Simulator run is the follow-up if Kai sees something here misses.
* **Authored screens** - only the dialog screens are captured. The working screen and the answered screen are authored.
* **`stream-under-load` is a guard, not proof** - it passes on main without the COI-3246 fix, because its burst repaints with absolute cursor moves. A proof needs a burst with relative moves around a resize.
* **Baselines are per platform** - they sit under `baselines/darwin/`. A Linux run writes its own.
* **No CI job yet** - GitHub Actions is blocked on COI-3227. Run the bed locally.

## Beside the drive

`just aterm-client-drive` drives the Demo host at 1280px and 320px with a few assertions. The bed is the wider tool: a mock daemon, five viewports, per step baselines. They do not share code.
