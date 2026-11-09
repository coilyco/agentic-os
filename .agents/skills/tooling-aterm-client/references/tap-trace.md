# The tap trace

An on-device record of what one tap or one submit did, for a phone bug the test bed cannot reproduce (COI-3334, COI-3243 bug 3). Code: `aterm-client/src/lib/tap-trace.ts` and `src/components/TapTraceBar.svelte`.

## Use it

* Open the client with `?taptrace` on the address. With no flag nothing runs, nothing is stored and no bar shows.
* A small **Copy N** button sits at the top right, N being the events held. Tap it once and the record is on the clipboard. Paste it into a message to a seat. If the clipboard refuses, a text box shows the record to copy by hand.
* Reload the page to start a record over.

## What it holds

* **Events** - pointer, touch, mouse, click and focus events in the order they fired, each with a time in ms since the first, the target, the point, and which renderer was showing: `daemon-card`, `screen-card` or `terminal`.
* **Marks** - fixed words the app says about itself: `card-shown`, `card-gone`, `screen menu` or `no-menu` when the screen read changes, `answer` with the renderer, and `viewport` with the visual size, so a keyboard opening shows.
* **Header** - build, place, user agent, viewport, pixel ratio, whether hover exists.

## What it never holds

A target is told by tag, id, class words, its place in a list and `:disabled`, never by its text or value. No seat name, question, option label, daemon address or token goes in, and the record never reaches the console or Sentry, which takes crashes only. The test bed asserts this on a real card walk.

## Read it

A drop to the terminal shows as `card-gone` with no `answer` before it, or `screen no-menu` while the card was up. A tap that needed two shows as a `pointerdown` and `touchend` with no `click` after it, a `focusout` between, or a `viewport` change inside the tap.

The feedback channel (COI-3345) is not on the real daemon yet, so the clipboard is the way out. When it is, the button can send a record instead.
