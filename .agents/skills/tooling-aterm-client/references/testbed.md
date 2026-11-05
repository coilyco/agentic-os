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

* **Mock daemon** - `mock-daemon.ts` serves `aterm-client/dist` at `/` and speaks `aterm.daemon.v1` on a websocket at the same origin, as the real daemon does, so the client needs no configuration. It scripts only frames the daemon has: `welcome`, `sessions`, `roster`, `attach` replay, `size`, `output`, `ask`, `asked`, `launch`, `subscribe` to `open_url`, and a seat's browser (`browser_watch`, `browser_control`, `browser_navigate`, `browser_state`, no frames). A frame it does not model answers with an `error`. It records every `input`, `answer` and `launch` for assertions. It can also fail on purpose: `silenceClients` leaves a socket open and mute, `dials('refuse'|'hang'|'ok')` and `refuseNext(n)` shape the next dials, and `dropClients` closes from its side.
* **Scenes** - `scenes.ts` draws a seat's screen at whatever size the client asks for, as a harness redraws on resize. Claude Code's AskUserQuestion dialog is emulated from real captures, so a click on a card walks the dialog.
* **Captures** - `fixtures/captures/` holds real Claude Code 2.1.318 screens, replayed verbatim at their own width on desktop. Add one by dropping a plain-text screen in that folder.
* **Roster** - the mock serves `src/lib/fixtures/roster.json`, so one fixture has one owner.
* **Another build** - `ATERM_TESTBED_DIST=<dir>` points the bed at any built client, to try a fix before it lands. Add `ATERM_TESTBED_COMPARE=0` to judge its behavior and ignore how it now looks, since a fix changes pixels. Run the update verb afterward to take the new baselines.

## Projects

* `desktop-1440` and `desktop-1920` - Chromium. The side panel starts open, which is itself COI-3238 item 2.
* `iphone` - iPhone 15 emulation on WebKit, Safari's engine.
* `android` - Pixel 7 emulation on Chromium.
* `phone-320` - the narrowest phone, Chromium.
* `iphone-landscape` (844x390, WebKit) and `android-landscape` (740x360, Chromium) - run only `landscape.spec.ts`, which counts the rows of seat output a person can read.

## Quirks go green

A journey tagged `@quirk` is a reported quirk. It fails now and goes green when the fix lands, so `just aterm-client-testbed` ends red while any quirk stands. Run it before pushing a fix. A fix also changes screenshots, so run the update verb, read the diff, and commit the new baselines with the fix.

State on main after PRs 193, 199, 203 and the find control, as the bed shows it, not as assumed:

* **Green** - COI-3243 bugs 2, 3 and 4 (`native-choice`, `native-choice-captured`), COI-3238 item 1's box drawing gap and find control (`terminal-text`), COI-3238 item 2 (`side-panel`), and COI-3243 bug 1's client half (`dialog-input-hold`, COI-3322): Send waits while a dialog is open and the message stays as a draft. The daemon half cannot be tested on a mock that would only assert itself.
* **Red** - the dialog's tab bar as buttons (`native-choice`).
* **Fixme, bed cannot judge yet** - link hover and history depth in `terminal-text` (COI-3293).

Journeys that hold today: `reconnect` (COI-3352: a restart ridden out, a silent socket, a page returning after hours, a hung dial, a host unreachable at page load, a cold start whose first or attach dial fails, and an hour of tries that ends in a rest, each reconnecting with no tap, on a faked clock), `unlock-resume` (a locked device redials inside and after the resume window, COI-3305), `read-output`, `switch-seats`, `ask-card`, `native-choice` (one question), `start-seat`, `seat-rows`, `seat-sheet` (phones), `composer-keyboard` (compared again, COI-3321), `pty-size` (COI-3291: a short conversation does not grow the PTY), `stream-under-load`, `open-url`, `wide-line` (COI-3317: a PTY held wider than a phone scrolls to its last character, swipes, keeps its place through a resize, and shows an edge cue. The mock's `holdWide` stands in for a wider client), `message-expand` (COI-3362: a long and a short message open and close in the Messages tab, by keyboard too, and the waiting question stays).

## What it cannot tell you

* **Not a real phone** - WebKit emulation is not iOS. The keyboard journey shrinks the viewport by a keyboard's height, which a real iOS keyboard does not match. A Simulator run is the follow-up if Kai sees something here misses.
* **Authored screens** - only the dialog screens are captured. The working screen and the answered screen are authored.
* **`stream-under-load` is a guard, not proof** - it passes on main without the COI-3246 fix, because its burst repaints with absolute cursor moves. A proof needs a burst with relative moves around a resize.
* **Baselines are per platform** - they sit under `baselines/darwin/` for a Mac and `baselines/linux/` for CI. A font rasterises differently on each.

## In CI

The `testbed` job in `.github/workflows/aterm-client.yml` runs on a GitHub-hosted Ubuntu runner whenever `aterm-client/**` changes. It gates on `--grep-invert @quirk`, so reported quirks do not fail a pull request, and it runs the `@quirk` set as a report that cannot fail the job. A missing baseline fails in CI. The report, results and diffs upload as the `testbed` artifact.

To write or refresh the Linux baselines, run the workflow by hand with `update` on, download the `testbed` artifact, and commit `baselines/linux/`. Do the same after a change that moves pixels.

## Beside the drive

`just aterm-client-drive` drives the Demo host at 1280px and 320px with a few assertions. The bed is the wider tool: a mock daemon, five viewports, per step baselines. They do not share code.

## How many seats it holds

`just aterm-client-scale 25,100,200` starts the mock daemon with that many seats, one in ten working, and prints per roster frame cost, burst long tasks, output streaming CPU, and seat switch times from a real Chromium. `SCALE_LIVE=<daemon page>` runs the real client against a real daemon, read only, and never opens a seat since attaching resizes live terminals. It is a measurement and not the gate.

The gate holds two lines. `journeys/seat-scale.spec.ts` asserts 100 seats show no long task over 100 ms across seat switches and typing. `src/lib/app-scale.test.ts` asserts a pass over every row of a 3000 seat roster is cheap, which a per row rebuild of the seat set broke (COI-3363). Timing a whole page ratio was tried for the second and was too noisy to gate on.

Read cost ratios and not milliseconds when the machine is busy: every figure inflates under load. A Mac running dozens of seats is busy.
