# A seat's page opens in the client

When a seat asks to open a page, the [host daemon](../../../../docs/aterm-daemon.md) hands the address to a client that can show it, labeled with the seat. With no such client the host's own browser opens, so nothing is lost (COI-3258). This page is the daemon half. The client half draws the page and is frontend-eng's.

**`aterm open <url>` is the verb, and it is what a seat's `BROWSER`, `open` and `xdg-open` run.** One http or https address goes to the daemon. Anything else, a file, a folder, `open -a Safari`, two addresses, runs the host's opener with the arguments untouched. It is stamped by the seat's `ATERM_SESSION_TOKEN`, as `send` is, so outside a seat it just opens the native browser.

**Every seat launch is pointed at it by the daemon, not by the launcher.** At start the daemon writes `open` and `xdg-open` into `shims/` beside its socket (0700 directory, 0700 files, each `exec`s this binary's `aterm open`) and rewrites them at each start, so an upgraded binary's path replaces the old one. A seat's environment gets that directory first on `PATH` and `BROWSER=<shims>/open`, overriding any `BROWSER` the launch carried. A plain terminal is Kai's own shell and gets neither. A daemon that cannot write the shims logs it and leaves seats on their own browser.

**The frames.**
* `subscribe` with `channel: "open_url"` is how a client says it can show an address. It answers `subscribed`. A client that does not subscribe is never counted, and a `sessions` subscriber is not one. The welcome feature is `open-url`.
* `open_url` `{token, url}` is the seat's request. It answers `opened` `{session, delivered}`, `delivered` being how many subscribers it was offered to. Refusals carry `reason`: `bad_url` (not an http or https address with a host), `too_large` (over 8 KiB), `too_many` (over 20 in a minute from one seat). A token that names no live seat is refused as `send` refuses it.
* The push to each subscriber is `open_url` `{session, url}`. `session` is the seat's name and is the label. The token never leaves the daemon.

**The fallback is the verb's.** `delivered` of 0, no daemon, a daemon without `open-url`, or a call outside a seat all run the native opener: `/usr/bin/open` by absolute path on macOS, the first `xdg-open` outside `shims/` elsewhere. That process gets `PATH` without `shims/`, a `BROWSER` that names a shim removed, and `ATERM_OPEN_NATIVE=1`, so the native opener cannot reach the shim again. `too_many` is the exception: it fails instead, since opening the native browser would be the flood the limit stops.

**The address is never logged.** An OAuth login URL carries its state. The daemon logs the seat and how many clients took it.

## What each harness did

Observed on 2026-11-04 against a private daemon on this build, with a fake client subscribed. Each seat ran `open`, and claude also `xdg-open` and `python3 -m webbrowser`.

* **claude** - `BROWSER`, `open` and `xdg-open` resolved to the shims. All three reached the client, labeled with the seat, from the Bash tool.
* **goose** - `open` reached the client.
* **codex** - **not covered.** Its shell tool drops `BROWSER` and the shim `PATH`, so `open` ran `/usr/bin/open` and the native browser opened (COI-3275).
* **opencode** - not run. Its model route was unknown on the day, so no shell command ran.

## What it does not cover

* **Which tab or pane the client uses is the client's call.** The frame carries the seat and the address and says nothing about showing it. Whether a page lands in the seat's own browser pane or a separate tab is for Kai to decide (COI-3258).
* **A harness that opens a page without `PATH` or `BROWSER`** is not reached. An absolute `/usr/bin/open`, or a native library calling the system directly, opens the native browser as before.
