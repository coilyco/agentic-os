# The streamed browser

The [aterm host daemon](../../../../docs/aterm-daemon.md) can run one headless Chromium per session and stream it to every client as CDP screencast frames. The client's Browser pane draws them and sends a person's pointer and keys back. This page is the frame contract and what the daemon does behind it. The first slice shipped under COI-2493 and the design came from COI-1973.

## What the daemon does

**The first `browser_watch` on a session starts that session's Chromium.** The daemon runs `--headless=new` with `--remote-debugging-pipe`, so CDP rides a pipe pair and no debugging port exists for another local user to reach. The profile is the role's Playwright profile when free, else a throwaway directory under `<socket dir>/browser/` (see below), and the browser sees only `HOME`, `PATH`, `TMPDIR`, locale and display variables, never the daemon's credentials. As root it adds `--no-sandbox`.

**`welcome.features` lists `browser` only when a browser binary exists.** `ATERM_BROWSER` names one, else the daemon looks for Google Chrome or Chromium at the macOS paths and on `PATH`. A host with none leaves the client's pane on its empty state.

**The browser starts on its role's Playwright profile, so logins made there are in the stream.** The daemon reads `browser.userDataDir` from `local_coilyco_playwright_<role>.json` in `<projects root>/coilyco/agentic-os-kai/config/`, or from the directory `ATERM_PLAYWRIGHT_CONFIG_DIR` names. The files are Kai's per-host input, read at each start and never embedded. The profile is never deleted. A role with no file, an `isolated` config, or a relative `userDataDir` gets a throwaway profile, and `browser_state.reason` says which.

**One process at a time may use a profile, so a second session of the role gets a throwaway one.** The daemon tracks which session holds each role profile, and the reason on the second names the first. It also reads Chromium's `SingletonLock` in the profile, so a live process outside aterm, the role's own Playwright MCP included, sends the session to a throwaway profile and the reason names its pid. While a streamed browser holds the profile, that role's Playwright MCP is expected to answer "browser is already in use" until the streamed browser ends (not yet observed against this daemon).

**It ends with its session, its process, or the daemon.** The daemon asks Chromium to close over CDP, which writes cookies out (SIGTERM skips that, and Chromium otherwise flushes about 30 seconds after a login). The session's process group gets SIGTERM after 2 seconds, then SIGKILL after 3 more. A browser that exits, or whose page closes, becomes `closed` with a reason, and the next `browser_watch` starts a new one. A daemon killed outright cannot stop it, so each launch writes a record under `<socket dir>/browser/pids/`, and the next daemon ends any browser it still finds there and deletes the throwaway profiles. On macOS Chrome exits about a second after its CDP pipe closes, so what that leaves is the profile directory. The record is for a build that does not exit on its own.

**A session's Playwright MCP can drive this same browser.** A launch with `ATERM_BROWSER_PROXY=1` in its environment gets `PLAYWRIGHT_MCP_CDP_ENDPOINT=ws://127.0.0.1:7419/cdp/<session token>`, which `@playwright/mcp` 0.0.78 reads in place of `--cdp-endpoint`, so the projected MCP config stays static. It is opt-in because the shared browser runs on a temporary profile, so a role's Playwright logins are not in it (COI-2517). The first CDP connection starts the browser, with no watcher needed, and a person's `browser_watch` later sees the page the agent opened.
* **The peer is vouched for twice.** The token names the session, and the socket's owning pid must run under that session's own process. A pid or process table the daemon cannot read refuses, the opposite default of the typing guard, since this admits a peer instead of limiting one. A request with an `Origin` or from off loopback is refused.
* **The agent gets its own CDP sessions and nothing else.** The daemon relays the agent's commands under ids of its own and passes back only events from sessions the agent created or auto-attached. The session the daemon streams and types into answers `Session with given id not found.` so the agent cannot address it.
* **The browser belongs to the person watching it.** `Browser.close` is acknowledged and dropped, and so is the agent's `Target.setDiscoverTargets`, which would switch off the daemon's own tracking. When the agent disconnects, the daemon closes the tabs and contexts it opened, detaches its sessions and turns auto-attach off. A newer connection replaces an older one for the same session.
* **While a person holds control the agent's commands are refused, and its events keep coming.** The proxy answers a command with `a person has control of this browser (aterm); wait for it to be handed back`, so the agent's page model follows the person and handing back needs no replay. What still passes is the bookkeeping a Playwright connection needs to attach and initialise a page (`browser_proxy_gate.go`, fixed by running `@playwright/mcp` 0.0.78 against a real Chromium), so an agent that connects mid-control is ready the moment control returns. An evaluation is refused as a thrown exception, because Playwright rewrites a protocol error from one into "Execution context was destroyed". A click by snapshot ref still reads "Ref eN not found" in Playwright's own words, so the agent should take a new snapshot, which carries the message.
* **One run against the real thing:** `ATERM_REAL_PLAYWRIGHT_MCP=<path to @playwright/mcp cli.js> go test -run RealPlaywright` in `aterm/`, with Chrome or Chromium installed. It skips without both.

**The daemon follows the newest open page.** A page opened by `window.open` or a new tab becomes the one streamed, the screencast moves to it, and `browser_state` carries its url and title, kept current from `Target.targetInfoChanged`. When the followed page closes the stream returns to the newest page left, and the browser becomes `closed` only when none is left. A tab the agent brings to the front without opening a new one is not followed, since no CDP event announces it that the daemon listens for.

## Frames

Request frames carry an `id`, and a refusal comes back as `error` with that `id`, a `code`, and a stable `reason`.

* `browser_watch` `{session, width?, height?}` - the client attaches. `width` and `height` are the pane in CSS pixels, passed to `Page.startScreencast` as `maxWidth` and `maxHeight`. The page's own viewport is never resized, since that would change what the agent sees. Answered by a `browser_state` echoing `id`. A repeat `browser_watch` restarts a closed browser, and restarts the screencast only when the size changed.
* `browser_unwatch` `{session}` - the client leaves. A holder that leaves hands control back.
* `browser_control` `{session, take, force?}` - `take: true` makes the driver `person` and this client the holder. It is Kai's input, so it takes the typing guard: a browser started by a session may watch but not take (`session_descendant`), and a tailnet device needs its passkey assertion first (`passkey_required`). A take while another client holds control is refused as `browser_held` unless it carries `force: true`. `take: false` hands back, and only the holder may.
* `browser_input` `{session, kind, params}` - `kind` is `mouse` or `wheel` (CDP `Input.dispatchMouseEvent`), `key` (`Input.dispatchKeyEvent`) or `text` (`Input.insertText`). `params` is the CDP object in page CSS pixels, under 64 KiB.
* `browser_navigate` `{session, url}` - `Page.navigate`. Only `http`, `https` and `about:blank` pass, so a `file:` address cannot put the host's disk in a frame every watcher receives.
* `browser_state` `{session, state, driver, holder?, url, title, tab?, tabs?, reason?, client}` - `state` is `none`, `live` or `closed`. `driver` is `agent` or `person`. `tab` and `tabs` are the followed page's place among the open pages, oldest first from 1, and how many are open, sent while `live`. `reason` says why a browser is `none` or `closed`, and a live one carries the profile note. Pushed to every watcher on any change.
* `browser_frame` `{session, data, metadata, seq}` - `data` is a base64 JPEG. `metadata` is CDP's `ScreencastFrameMetadata` in snake_case: `offset_top`, `page_scale_factor`, `device_width`, `device_height`, `scroll_offset_x`, `scroll_offset_y`, `timestamp`. `seq` counts frames, so a gap is frames dropped for a slow client.

`browser_input` and `browser_navigate` are refused unless the driver is `person` and the sender is the holder. A client that never sent `browser_watch` for the session gets `not_watching` on any of the driving frames, and the others are `not_driver`, `browser_held` and `no_browser`.

**`browser_state.client` is the one field beyond the frames accepted on COI-1973.** It is the name the daemon gave the receiving connection, so a client tells that it holds control when `holder` equals `client`. Only the daemon knows a connection's name.

## Frames reach a slow client late and never in a queue

**The daemon acks each CDP frame itself.** A client's speed never throttles the page. Each watcher keeps only its newest `browser_state` and newest `browser_frame`, so a client that cannot keep up sees `seq` jump.

**A screen that starts watching gets the last frame at once.** Chromium casts only what repaints, so a still page would otherwise show a late screen nothing. If a started screencast sends nothing for 600 milliseconds, the daemon sends one screenshot as frame `seq` 1 in its place.

## What the client shows

The Browser pane names the followed page above the address: its title, and `Tab 2 of 3` beside it while more than one page is open. A daemon that sends no `tab` and `tabs` shows the title alone. When the numbers change, the pane says `Now showing tab 3 of 3` to a screen reader, since the daemon moves the stream without a click. The pane draws only what `browser_state` carries.

## What is not built

* A list of the open tabs with their titles and urls. `browser_state` carries a place and a count, so a list needs a daemon frame for it (COI-2558 files the request).
* Following a tab the agent brings to the front without opening one. No CDP event announces it that the daemon listens for.
