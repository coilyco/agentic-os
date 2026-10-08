# Views and the shared browser

The side panel's Views and Browser tabs. Where they fit: [architecture.md](architecture.md).

## What a host offers

The side panel's Views and Browser tabs render from optional `views` and `browser` channels on `HostConnection`. `DaemonHost` offers `views` only when `welcome.features` lists `mcp-apps` ([contract](mcp-apps-gateway.md)), and subscribes to the `views` channel then. It maps `view`, `view_update` and `view_closed` onto the view events, and a `view_call` rides the typing guard like `input`. It offers `browser` once a welcome lists the `browser` feature ([the frames](browser.md)). A tab without its channel says the host does not carry it. The Demo host scripts only what the daemon does, so it has neither.

## One sandboxed iframe per view

The MCP Apps spec asks a web host for a sandbox proxy on a second origin. This client is served by the daemon on one origin, so each view gets one iframe with `sandbox="allow-scripts"` and no `allow-same-origin`. That gives it an opaque origin, with no reach into this page, its storage, or the daemon socket. The view's `_meta.ui.csp` becomes a policy tag pinned first in its `<head>`, so it has no network unless it declared the domain. Messages are matched by the iframe's window, because an opaque origin has no name to check. The host side of the protocol is ext-apps' `AppBridge` with no MCP client, so every view call goes to the daemon gateway through `ViewBridge`'s handlers, and the sandbox-proxy hooks it also offers stay unused.

## A view loads on demand

`ViewsPanel` imports `AppView` with a dynamic `import()` once a session has a view, so AppBridge and its peers (ext-apps, the MCP SDK, zod) sit in their own chunk, and the two empty states never fetch it. A failed chunk fetch is cached by the browser for the life of the page, so the panel's error offers a reload, not a retry. The app-shell worker precaches the chunk with every other built file.

## The browser is a picture

The shared browser is the host's own Chromium, shown by CDP screencast. The pane draws each frame contained in its box and maps pointer and keys back to page pixels as CDP `Input` events. It sends them only from the screen holding control, and taking over from another screen is a forced take. Escape leaves the page, so the keyboard is never trapped in it.

## Narrow screens

Below 1000px the side panel is a bottom sheet over the terminal (COI-2506). It starts put away, as its tab strip pinned to the foot of the window, and the page keeps that strip's height clear of the composer. Picking a tab raises the sheet to 80% of the window height, and picking the tab already showing, the chevron beside the tabs, or Escape (from the strip or the Messages and Views tabs, never from a shell or the remote page) puts it away. A question waiting on the seat puts it away too, since it sits at the foot of the terminal. At 360px and below the tab labels shrink, and a badge sits on its tab's corner with "live" as a dot, so four tabs and the chevron fit 320px. Rules: `src/lib/sheet.ts`.

The pane shows the followed tab's title and its place among the open pages, and announces a change. [browser.md](browser.md#what-the-client-shows) has the rest.
