# The streamed browser

The [aterm host daemon](../../../../docs/aterm-daemon.md) can run one headless Chromium per session and stream it to every client as CDP screencast frames. The client's Browser pane draws them and sends a person's pointer and keys back. This page is the frame contract and what the daemon does behind it. The first slice shipped under COI-2493 and the design came from COI-1973.

## What the daemon does

**The first `browser_watch` on a session starts that session's Chromium.** The daemon runs `--headless=new` with `--remote-debugging-pipe`, so CDP rides a pipe pair and no debugging port exists for another local user to reach. The profile is a throwaway directory under `<socket dir>/browser/`, and the browser sees only `HOME`, `PATH`, `TMPDIR`, locale and display variables, never the daemon's credentials. As root it adds `--no-sandbox`.

**`welcome.features` lists `browser` only when a browser binary exists.** `ATERM_BROWSER` names one, else the daemon looks for Google Chrome or Chromium at the macOS paths and on `PATH`. A host with none leaves the client's pane on its empty state.

**It ends with its session, its process, or the daemon.** The session's process group gets SIGTERM, then SIGKILL after 3 seconds. A browser that exits, or whose page closes, becomes `closed` with a reason, and the next `browser_watch` starts a new one. A daemon killed outright leaves its Chromium running (COI-2519).

**The daemon follows the first page target and nothing else.** `browser_state` carries that page's url and title, kept current from `Target.targetInfoChanged`.

## Frames

Request frames carry an `id`, and a refusal comes back as `error` with that `id`, a `code`, and a stable `reason`.

* `browser_watch` `{session, width?, height?}` - the client attaches. `width` and `height` are the pane in CSS pixels, passed to `Page.startScreencast` as `maxWidth` and `maxHeight`. The page's own viewport is never resized, since that would change what the agent sees. Answered by a `browser_state` echoing `id`. A repeat `browser_watch` restarts a closed browser, and restarts the screencast only when the size changed.
* `browser_unwatch` `{session}` - the client leaves. A holder that leaves hands control back.
* `browser_control` `{session, take, force?}` - `take: true` makes the driver `person` and this client the holder. It is Kai's input, so it takes the typing guard: a browser started by a session may watch but not take (`session_descendant`), and a tailnet device needs its passkey assertion first (`passkey_required`). A take while another client holds control is refused as `browser_held` unless it carries `force: true`. `take: false` hands back, and only the holder may.
* `browser_input` `{session, kind, params}` - `kind` is `mouse` or `wheel` (CDP `Input.dispatchMouseEvent`), `key` (`Input.dispatchKeyEvent`) or `text` (`Input.insertText`). `params` is the CDP object in page CSS pixels, under 64 KiB.
* `browser_navigate` `{session, url}` - `Page.navigate`. Only `http`, `https` and `about:blank` pass, so a `file:` address cannot put the host's disk in a frame every watcher receives.
* `browser_state` `{session, state, driver, holder?, url, title, reason?, client}` - `state` is `none`, `live` or `closed`. `driver` is `agent` or `person`. `reason` says why a browser is `none` or `closed`, and a live one carries the profile note. Pushed to every watcher on any change.
* `browser_frame` `{session, data, metadata, seq}` - `data` is a base64 JPEG. `metadata` is CDP's `ScreencastFrameMetadata` in snake_case: `offset_top`, `page_scale_factor`, `device_width`, `device_height`, `scroll_offset_x`, `scroll_offset_y`, `timestamp`. `seq` counts frames, so a gap is frames dropped for a slow client.

`browser_input` and `browser_navigate` are refused unless the driver is `person` and the sender is the holder. A client that never sent `browser_watch` for the session gets `not_watching` on any of the driving frames, and the others are `not_driver`, `browser_held` and `no_browser`.

**`browser_state.client` is the one field beyond the frames accepted on COI-1973.** It is the name the daemon gave the receiving connection, so a client tells that it holds control when `holder` equals `client`. Only the daemon knows a connection's name.

## Frames reach a slow client late and never in a queue

**The daemon acks each CDP frame itself.** A client's speed never throttles the page. Each watcher keeps only its newest `browser_state` and newest `browser_frame`, so a client that cannot keep up sees `seq` jump.

**A screen that starts watching gets the last frame at once.** Chromium casts only what repaints, so a still page would otherwise show a late screen nothing. If a started screencast sends nothing for 600 milliseconds, the daemon sends one screenshot as frame `seq` 1 in its place.

## What is not built

* Pointing the session's Playwright MCP at this Chromium, so the agent drives the browser a person watches (COI-2516).
* Refusing the agent's commands while a person holds control (COI-2520).
* The role's Playwright `userDataDir` and the same-role profile-lock fallback. Every browser runs on a temporary profile today (COI-2517).
* Cleanup after a killed daemon, and tabs beyond the first page (COI-2519).
