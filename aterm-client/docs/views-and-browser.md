# Views and the shared browser

The side panel's Views and Browser tabs. Where they fit: [architecture.md](architecture.md).

## They wait on the daemon

The side panel's Views and Browser tabs render from optional `views` and `browser` channels on `HostConnection`. No host has them yet, because the daemon frames are proposed on `teable:coilyco/agentic-os#8220` and not yet contract, so both tabs say the host does not carry them. The Demo host scripts only what the daemon does, so it has neither.

## One sandboxed iframe per view

The MCP Apps spec asks a web host for a sandbox proxy on a second origin. This client is served by the daemon on one origin, so each view gets one iframe with `sandbox="allow-scripts"` and no `allow-same-origin`. That gives it an opaque origin, with no reach into this page, its storage, or the daemon socket. The view's `_meta.ui.csp` becomes a policy tag pinned first in its `<head>`, so it has no network unless it declared the domain. Messages are matched by the iframe's window, because an opaque origin has no name to check.

## The browser is a picture

The shared browser is the host's own Chromium, shown by CDP screencast. The pane draws each frame contained in its box and maps pointer and keys back to page pixels as CDP `Input` events. It sends them only from the screen holding control, and taking over from another screen is a forced take. Escape leaves the page, so the keyboard is never trapped in it.

## Narrow screens

Below 1000px the side panel sits under the terminal with a height cap, which a live page lifts so it stays readable.
