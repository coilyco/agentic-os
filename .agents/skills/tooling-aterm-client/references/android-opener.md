# Opening an artifact or link in the device's own browser

The Android and Mac apps are Tauri windows, and a Tauri window does nothing for `window.open` or `target=_blank` (COI-3520). In wry 0.57 the macOS window delegate answers a new-window request with nothing when the app registered no handler, and this app registers none. On Android the source does not say whether the page is navigated away or the call is dropped. A run on the emulator settles it, and it is not done.

**Every way the client opens an address goes through `openExternal` in `open-external.ts`.** In the app it calls the official `tauri-plugin-opener` (`plugin:opener|open_url`), which sends Android an `ACTION_VIEW` in a new task and macOS `open`, so the address lands in the default browser, where Kai is signed in to claude.ai. In a browser tab it opens a tab as before. Only `http` and `https` addresses go out, and the capability allows no other scheme and no file reveal.

**What changed on screen.** In the apps, the card a seat's `open_url` raises for a claude.ai artifact reads "Open in browser" and opens it with no second step. It no longer offers "Show here", since claude.ai refuses framing. A plain page a seat wants to drive keeps "Show here" and the seat's streamed browser (COI-3258). A tab in a browser is unchanged. A terminal link, an MCP view's link, and the connection check's link open the same way.

**A refusal is said in words, in a card at the top beside the opened-page cards** (`link-notice.svelte.ts`). It names the address and the reason the system gave, and an artifact's own card stays with the address to copy. A success clears it.

**The plugin is pinned `=2.7.2`.** Audited 2026-11-09: the crate is published from `tauri-apps/plugins-workspace` by `tauri-bot`, has about 16 million downloads, 2.7.2 came out 2026-10-14, its `rust-version` is 1.90 like this repo's, and it needs `tauri ^2.12`. It adds about 40 locked crates, nearly all `zbus` and `async-*` for Linux, which no build here compiles. On Android it adds `androidx.browser` through Gradle, which is not hash-verified like the rest ([android.md](android.md)).

## Not yet checked on a device

* A tap on an artifact card opening the browser on Android and in the Mac app, and a terminal link doing the same.
* The Android behaviour before this change, to know whether the app was navigated away.
