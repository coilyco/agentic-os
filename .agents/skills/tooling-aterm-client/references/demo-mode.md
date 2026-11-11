# Demo mode

A way into the client with no host to connect to (COI-3315). A Google Play reviewer has no daemon, and the real one gives a host login shell, so the client carries its own made-up host.

## What a person sees

* **The first screen** - on "Pick a host", a dashed amber card offers **Try the demo** above the host form. At 320px it is on screen without scrolling.
* **Inside** - four made-up seats, a question from one of them (a choice card), two messages between seats, and a side Terminal tab. The first seat opens on its terminal.
* **A seat in Focus** - the first seat has a made-up transcript read from memory, so Focus, Default and Verbose show, and it "shares" three drawn pictures that open in the viewer. Both live in [`demo-script.ts`](../../../../aterm-client/src/lib/demo-script.ts) and [`demo-frames.ts`](../../../../aterm-client/src/lib/fixtures/demo-frames.ts). The transcript keeps no page on the device.
* **Marked everywhere** - an amber `DEMO` bar sits above the whole page, above the seat menu too, so no screen can pass for a real host. Each seat prints a `DEMO` line first, the side terminal does the same, and the tab title starts `DEMO //`.
* **Leaving** - **Leave demo** returns to "Pick a host" with focus on **Try the demo**. Nothing of the demo stays.

## What is real

Nothing. The demo is in [`src/lib/demo-host.ts`](../../../../aterm-client/src/lib/demo-host.ts), a `HostConnection` that runs in the page.

* **No network** - it imports no transport. [`demo-isolation.test.ts`](../../../../aterm-client/src/lib/demo-isolation.test.ts) records every `WebSocket`, `fetch`, `XMLHttpRequest`, `EventSource` and beacon with a real saved host on the device, runs the whole journey and ten idle minutes, and expects none. A control in the same file shows the recorder does see a socket when a real host is picked.
* **No shell** - the side terminal is a table of replies in [`demo-script.ts`](../../../../aterm-client/src/lib/demo-script.ts). It reads nothing and runs nothing, and a line it does not know says so.
* **No residue** - nothing is written about hosts or terminals while it runs. Discovery waits while it is up, and every id it makes starts with `demo-` so leaving can drop it.

## Why it is not the mock host

`mock-host.ts` is the dev harness behind `just dev` and its `?terminals=` modes, and it changes with the work. The demo has to stay put for a store review, and its content has to be safe to publish. In a production build the "Demo host" row opens this demo, and `just dev` keeps the mock host.

## Writing for it

* **Made up** - no real host, person or project in the script.
* **34 columns** - a phone terminal is about 36 columns and breaks words at its edge, so prose is wrapped before it is sent. A test fails on a longer row.

## Not done

* The demo is not reached from the host list in the desktop dev build, which still opens the mock host.
* Screenshots for a store listing are COI-3313's. Words addressed to a store reader belong to the Developer Advocate.
