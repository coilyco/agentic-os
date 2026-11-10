<script lang="ts">
  import { FitAddon } from "@xterm/addon-fit";
  import { SearchAddon } from "@xterm/addon-search";
  import { WebLinksAddon } from "@xterm/addon-web-links";
  import { Terminal, type IDisposable } from "@xterm/xterm";
  import "@xterm/xterm/css/xterm.css";
  import { onMount } from "svelte";
  import FindBar from "./FindBar.svelte";
  import { AT_END, jumpLabel, linesAbove, scrolled, wrote, type Follow } from "../lib/follow";
  import { envelopeRows } from "../lib/messages";
  import { harnessBox } from "../lib/harness";
  import { resolveColour } from "../lib/brand";
  import { openLink } from "../lib/link-notice.svelte";
  import { boxToReport, nextTall } from "../lib/pty-size";
  import { detectChoice } from "../lib/choices";
  import { linkTarget, opensLink, wantsFind } from "../lib/terminal-extras";
  import { coastRows, swipeGoesTo, takeRows } from "../lib/touch-scroll";
  import { loadWrap, storeWrap } from "../lib/wrap";
  import { lineChars, urlAtChain } from "../lib/wrap-links";
  import { DEFAULT_TEXT_SIZE, loadTextSize, stepTextSize, storeTextSize, TEXT_SIZES } from "../lib/text-size";
  import { isBlank, screenRows, tabBarReading, type TabBarCells } from "../lib/screen";
  import { afterQueued, safely, watchDelay } from "../lib/pane-watch";
  import type { HostConnection, PeerMessage } from "../lib/protocol";

  let {
    connection,
    sessionId,
    label,
    accent,
    messages,
    colorOf,
    onscreen,
    locked = false,
    cropHarness = false,
    covered = false,
  }: {
    connection: HostConnection;
    sessionId: string;
    label: string;
    accent: string;
    messages: PeerMessage[];
    colorOf: (role: string) => string;
    /** The visible rows after each write, for readers such as the choice detector. */
    onscreen?: (rows: string[], tabBar: TabBarCells | null) => void;
    /** The daemon refuses this connection's typing, so keys go nowhere. */
    locked?: boolean;
    /** The composer is the one input at every width, so the harness's own input box is scrolled out of view. */
    cropHarness?: boolean;
    /** A card sits over the foot of the terminal, so the way back to the latest output waits. */
    covered?: boolean;
  } = $props();

  let host: HTMLDivElement;
  let term: Terminal | undefined;
  // The box scrolls when the terminal is bigger than it: a PTY sized to a larger client, or a cropped harness box.
  let panning = $state(false);
  // Whether the view sits above the newest output, and how many lines came in while it did.
  let latest = $state<Follow>(AT_END);
  let trackView = () => {};
  // Said in the pane while it has drawn nothing and the host is being asked again. Empty otherwise.
  let waiting = $state("");
  const HIDE_SETTLE_MS = 350;
  // Rows must touch, or box-drawing characters break into dashes.
  const LINE_HEIGHT = 1;
  const SCROLLBACK = 10000;
  // The device's text size. A different size is a different box of columns, which layout() reports.
  let textSize = $state(loadTextSize());
  let sizing = $state(false);
  // Measured in pixels, not columns: a smaller size adds columns and must not take its own button away.
  let narrow = $state(false);
  const NARROW_PX = 360;
  // Long lines fold to the screen's width in this client alone. The daemon is told nothing, so no other view of the seat moves (COI-3544).
  let wrapping = $state(false);
  // Offered where dragging sideways is the tedious part: a touch screen or a narrow window, and only while the
  // PTY is wider than the box or the switch is on. A desktop pane scrolls its long lines.
  let handheld = $state(false);
  let overflows = $state(false);
  const canWrap = $derived((handheld && overflows) || wrapping);
  const showSize = $derived(narrow || sizing || textSize !== DEFAULT_TEXT_SIZE);
  let refit = () => {};
  let finding = $state(false);
  let findSeed = $state("");
  let search = $state<SearchAddon>();
  let decorations: IDisposable[] = [];
  const marked = new Set<number>();

  // A wrapped row continues the logical line above it, so envelopes and bodies
  // are matched and measured in logical lines, not screen rows.
  function logicalLines(terminal: Terminal): { start: number; rows: number; text: string }[] {
    const buffer = terminal.buffer.active;
    const lines: { start: number; rows: number; text: string }[] = [];
    for (let row = 0; row < buffer.length; row++) {
      const line = buffer.getLine(row);
      const text = line?.translateToString(true) ?? "";
      const last = lines.at(-1);
      if (line?.isWrapped && last) {
        last.rows += 1;
        last.text += text;
      } else {
        lines.push({ start: row, rows: 1, text });
      }
    }
    return lines;
  }

  // Where each envelope sits, for the folded view, which paints its stripes on the rows themselves.
  let stripes: { start: number; rows: number; color: string }[] = [];
  let painted = false;
  function paintStripes(): void {
    if (!term || !host || (!wrapping && !painted)) return;
    const first = term.buffer.active.viewportY;
    host.querySelectorAll<HTMLElement>(".xterm-rows > div").forEach((row, index) => {
      const at = wrapping ? stripes.find((stripe) => first + index >= stripe.start && first + index < stripe.start + stripe.rows) : undefined;
      if (at) {
        row.dataset.stripe = "";
        row.style.setProperty("--stripe", at.color);
        row.style.background = `color-mix(in srgb, ${at.color} 14%, transparent)`;
      } else if ("stripe" in row.dataset) {
        delete row.dataset.stripe;
        row.style.removeProperty("--stripe");
        row.style.background = "";
      }
    });
    painted = wrapping;
  }

  // The address under a tap on a folded row. xterm maps a pointer by cell size, which a fold breaks.
  function urlUnder(x: number, y: number): string | null {
    const range = document.caretRangeFromPoint?.(x, y);
    if (!term || !range) return null;
    const node = range.startContainer;
    const row = (node instanceof Element ? node : node.parentElement)?.closest<HTMLElement>(".xterm-rows > div");
    if (!row?.parentElement) return null;
    const upto = document.createRange();
    upto.selectNodeContents(row);
    upto.setEnd(node, range.startOffset);
    const buffer = term.buffer.active;
    const at = buffer.viewportY + [...row.parentElement.children].indexOf(row);
    let start = at;
    while (start > 0 && buffer.getLine(start)?.isWrapped) start--;
    let end = at;
    while (buffer.getLine(end + 1)?.isWrapped) end++;
    const chain: string[] = [];
    for (let line = start; line <= end; line++) {
      const text = buffer.getLine(line);
      chain.push(text ? lineChars(text) : "");
    }
    const offset = upto.toString().length;
    // A caret at a character's far half lands one past it.
    return urlAtChain(chain, at - start, offset) ?? urlAtChain(chain, at - start, offset - 1);
  }

  // Stamp every envelope the harness has echoed so far with its sender's colour.
  function markEnvelopes(): void {
    if (!term || messages.length === 0) {
      stripes = [];
      paintStripes();
      return;
    }
    const buffer = term.buffer.active;
    const lines = logicalLines(term);
    const found = envelopeRows(lines.map((line) => line.text), messages);
    stripes = [...found].flatMap(([index, message]) => {
      const first = lines[index];
      return first ? [{ start: first.start, rows: first.rows, color: colorOf(message.from.role) }] : [];
    });
    paintStripes();
    for (const [index, message] of found) {
      const first = lines[index];
      if (!first || marked.has(first.start)) continue;
      const marker = term.registerMarker(first.start - (buffer.baseY + buffer.cursorY));
      if (!marker) continue;
      const color = colorOf(message.from.role);
      const decoration = term.registerDecoration({ marker, width: term.cols, height: first.rows, layer: "bottom" });
      if (!decoration) continue;
      decoration.onRender((element) => {
        // Pulled into the container's left padding so the stripe never covers column 0.
        element.style.marginLeft = "-10px";
        element.style.paddingRight = "10px";
        element.style.borderLeft = `4px solid ${color}`;
        element.style.background = `color-mix(in srgb, ${color} 14%, transparent)`;
      });
      decorations.push(decoration);
      marked.add(first.start);
    }
  }

  function reportScreen(): void {
    if (!term || !onscreen) return;
    onscreen(screenRows(term.buffer.active, term.rows), tabBarReading(term.buffer.active, term.rows));
  }

  let watchHarness = () => {};
  let watchTall = () => {};
  // The input and the newest text sit at the bottom left, so that corner stays in view
  // unless the person has panned away. Cropped rows lie below it.
  let hiddenPx = () => 0;
  // From the drawn screen, not scrollHeight, which a phone's xterm inflates past the rows it shows.
  const contentPx = () => {
    const style = getComputedStyle(host);
    const screen = host.querySelector<HTMLElement>(".xterm-screen")?.clientHeight ?? 0;
    return parseFloat(style.paddingTop) + screen + parseFloat(style.paddingBottom);
  };
  const lastTop = () => Math.max(0, contentPx() - host.clientHeight - hiddenPx());
  const atBottom = () => lastTop() - host.scrollTop < 2;
  // Sideways is left alone: a phone resizes with its address bar and keyboard, and a
  // reset here would send a person who panned to a long line back to column 0.
  function anchor(follow: boolean): void {
    if (follow) host.scrollTop = lastTop();
    edges();
  }

  // Whether text lies past either side, for the cue a phone's hidden scroll bar cannot give.
  let past = $state({ left: false, right: false });
  // Where the visible part sits along the full width, in percent, for the thin position bar.
  let thumb = $state({ x: 0, w: 100 });
  let measuring = 0;
  // A frame later, so reading the layout never lands between a resize and its anchor.
  function edges(): void {
    if (measuring) return;
    measuring = requestAnimationFrame(() => {
      measuring = 0;
      if (!host) return;
      overflows = host.scrollWidth > host.clientWidth + 2;
      const left = host.scrollLeft > 2;
      const right = host.scrollLeft + host.clientWidth < host.scrollWidth - 2;
      if (left !== past.left || right !== past.right) past = { left, right };
      const full = Math.max(host.scrollWidth, 1);
      const x = Math.round((host.scrollLeft / full) * 100);
      const w = Math.round((host.clientWidth / full) * 100);
      if (x !== thumb.x || w !== thumb.w) thumb = { x, w };
    });
  }

  // The scan reads the whole buffer, so a stream of writes asks for it a few times a second.
  let marking: ReturnType<typeof setTimeout> | undefined;
  function markSoon(): void {
    if (marking) return;
    marking = setTimeout(() => {
      marking = undefined;
      safely("marking envelopes", markEnvelopes);
    }, 200);
  }

  // A step that throws here would stop xterm taking any more writes, so each runs on its own.
  function afterWrite(): void {
    markSoon();
    safely("reading the screen", reportScreen);
    safely("watching the harness", () => watchHarness());
    safely("watching for a clipped dialog", () => watchTall());
    safely("noting the screen drew", () => noteDrawn());
    safely("noting text past the edge", edges);
  }

  let noteDrawn = () => {};
  let afterSize = (_follow: boolean) => {};

  function remark(): void {
    decorations.forEach((decoration) => decoration.dispose());
    decorations = [];
    marked.clear();
    markEnvelopes();
  }

  onMount(() => {
    const phone = matchMedia("(max-width: 720px)");
    wrapping = loadWrap(sessionId);
    const handheldQuery = matchMedia("(max-width: 720px), (pointer: coarse)");
    handheld = handheldQuery.matches;
    const onHandheld = () => (handheld = handheldQuery.matches);
    handheldQuery.addEventListener("change", onHandheld);
    const terminal = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      fontSize: textSize,
      lineHeight: LINE_HEIGHT,
      scrollback: SCROLLBACK,
      allowProposedApi: true,
      theme: { background: resolveColour("var(--terminal)"), foreground: resolveColour("var(--text-soft)"), cursor: resolveColour(accent), selectionBackground: `${resolveColour(accent)}55` },
    });
    term = terminal;
    // The seat's own window answers terminal queries. A reply from here would be
    // duplicate input, and replayed history would re-ask old ones. See the architecture reference.
    const swallow = () => true;
    terminal.parser.registerCsiHandler({ final: "n" }, swallow);
    terminal.parser.registerCsiHandler({ final: "c" }, swallow);
    terminal.parser.registerCsiHandler({ prefix: ">", final: "c" }, swallow);
    for (const color of [10, 11, 12]) terminal.parser.registerOscHandler(color, (data) => data.includes("?"));
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    const searching = new SearchAddon();
    terminal.loadAddon(searching);
    search = searching;
    const touch = matchMedia("(pointer: coarse)");
    const verb = /Mac|iPhone|iPad/.test(navigator.platform) ? "Cmd-click" : "Ctrl-click";
    terminal.loadAddon(
      new WebLinksAddon(
        (click, uri) => {
          // A folded row opens its link from the tap handler, which reads the text under the finger.
          if (wrapping) return;
          const href = linkTarget(uri);
          if (href && opensLink(click, touch.matches)) void openLink(href);
        },
        { hover: () => (host.title = touch.matches ? "" : `${verb} to open`), leave: () => (host.title = "") },
      ),
    );
    terminal.open(host);
    // Above the end when xterm's own view or the pan box is scrolled away from the newest row.
    const above = () => linesAbove(terminal.buffer.active) + (panning && !atBottom() ? 1 : 0);
    // One layout read per frame: a replay scrolls xterm thousands of times (COI-3422).
    let tracking = 0;
    trackView = () => {
      if (tracking) return;
      tracking = requestAnimationFrame(() => {
        tracking = 0;
        latest = scrolled(latest, above());
      });
    };
    terminal.onScroll(trackView);
    terminal.onRender(paintStripes);
    const onTap = (event: MouseEvent) => {
      if (!wrapping || event.defaultPrevented) return;
      const href = linkTarget(urlUnder(event.clientX, event.clientY) ?? "");
      if (href && opensLink(event, touch.matches)) void openLink(href);
    };
    host.addEventListener("click", onTap);
    host.addEventListener("scroll", trackView);
    host.addEventListener("scroll", edges);
    // Folded rows change the screen's height as text arrives, so a view at the newest line follows it down.
    let pinned = true;
    const onPin = () => (pinned = atBottom());
    host.addEventListener("scroll", onPin);
    const grown = new ResizeObserver(() => {
      if (wrapping && pinned) host.scrollTop = lastTop();
    });
    const drawn = host.querySelector(".xterm-screen");
    if (drawn) grown.observe(drawn);
    // A finger's vertical drag scrolls the history: xterm reads wheels and keys, not touch (COI-3253).
    const rowPx = () => (host.querySelector<HTMLElement>(".xterm-screen")?.clientHeight ?? 0) / Math.max(terminal.rows, 1);
    let fingerY = 0;
    let carry = 0;
    let speed = 0;
    let fingerAt = 0;
    let fingerX = 0;
    let coasting = 0;
    // A full-screen app has no history here, so it gets a wheel as a mouse would send it.
    // A wrapped screen can be taller than the box, so a drag moves the box first and the history after it.
    const scrollBox = (rows: number) => {
      const px = rowPx();
      if (!px) return rows;
      const before = host.scrollTop;
      host.scrollTop = Math.min(lastTop(), Math.max(0, before + rows * px));
      return rows - Math.round((host.scrollTop - before) / px);
    };
    const move = (taken: number) => {
      const rows = wrapping ? scrollBox(taken) : taken;
      if (!rows) return;
      const active = terminal.buffer.active;
      const where = swipeGoesTo(active.type, active.baseY, terminal.modes.mouseTrackingMode);
      if (where === "history") terminal.scrollLines(rows);
      if (where !== "app") return;
      const screen = host.querySelector(".xterm-screen") ?? host;
      for (let tick = 0; tick < Math.min(Math.abs(rows), 12); tick++) {
        screen.dispatchEvent(new WheelEvent("wheel", { deltaY: Math.sign(rows), deltaMode: 1, clientX: fingerX, clientY: fingerY, bubbles: true, cancelable: true }));
      }
    };
    const stopCoast = () => {
      cancelAnimationFrame(coasting);
      coasting = 0;
    };
    const onTouchStart = (event: TouchEvent) => {
      stopCoast();
      if (event.touches.length !== 1) return;
      fingerY = event.touches[0]!.clientY;
      fingerX = event.touches[0]!.clientX;
      fingerAt = event.timeStamp;
      carry = 0;
      speed = 0;
    };
    const onTouchMove = (event: TouchEvent) => {
      if (event.touches.length !== 1) return;
      const y = event.touches[0]!.clientY;
      const drag = fingerY - y;
      const span = Math.max(event.timeStamp - fingerAt, 1);
      speed = speed * 0.6 + (drag / span) * 0.4;
      fingerY = y;
      fingerAt = event.timeStamp;
      const taken = takeRows(carry, drag, rowPx());
      carry = taken.carry;
      fingerX = event.touches[0]!.clientX;
      if (taken.rows) move(taken.rows);
    };
    const onTouchEnd = () => {
      let left = coastRows(speed, rowPx());
      const step = () => {
        const now = Math.sign(left) * Math.max(1, Math.round(Math.abs(left) / 8));
        if (!left) return;
        move(now);
        left -= now;
        coasting = requestAnimationFrame(step);
      };
      if (left) coasting = requestAnimationFrame(step);
    };
    host.addEventListener("touchstart", onTouchStart, { passive: true });
    host.addEventListener("touchmove", onTouchMove, { passive: true });
    host.addEventListener("touchend", onTouchEnd, { passive: true });
    host.addEventListener("touchcancel", stopCoast, { passive: true });
    // A harness that reports the mouse would take every wheel tick and leave the history still.
    // On the normal screen the wheel reads the history. A full-screen app keeps its mouse.
    let wheelCarry = 0;
    terminal.attachCustomWheelEventHandler((event) => {
      const active = terminal.buffer.active;
      if (terminal.modes.mouseTrackingMode === "none" || active.type !== "normal" || active.baseY === 0) return true;
      const row = rowPx();
      const taken = takeRows(wheelCarry, event.deltaMode === 1 ? event.deltaY * row : event.deltaY, row);
      wheelCarry = taken.carry;
      if (taken.rows) terminal.scrollLines(taken.rows);
      return false;
    });
    const cropping = () => cropHarness;
    let hidden = 0;
    // Blank rows under a floating box hide with it, but only its height is asked of the PTY.
    let boxRows = 0;
    // Rows asked beyond the box while a dialog is taller than the screen (COI-3447).
    let tall = 0;
    let lastDialog = 0;
    let tallTimer: ReturnType<typeof setTimeout> | undefined;
    const extra = () => (cropping() ? hidden : 0);
    const asking = () => (cropping() ? boxRows : 0);
    const cell = () => (host.querySelector<HTMLElement>(".xterm-screen")?.clientHeight ?? 0) / Math.max(terminal.rows, 1);
    // The window ends at the last shown row, not at the padding below it, which would show a sliver of the box.
    // A wrapped row is taller than a cell, so the cropped rows are measured.
    const tailPx = (rows: number) =>
      wrapping
        ? [...host.querySelectorAll<HTMLElement>(".xterm-rows > div")].slice(-rows).reduce((sum, row) => sum + row.getBoundingClientRect().height, 0)
        : rows * cell();
    hiddenPx = () => (extra() > 0 ? tailPx(extra()) + parseFloat(getComputedStyle(host).paddingBottom) : 0);
    // This client's capacity is what its box can hold, which is not what xterm currently is.
    const own = () => ({ rows: terminal.rows, cols: terminal.cols });
    let reported: { rows: number; cols: number } | undefined;
    // Rows the box cannot show are asked for anyway, so the harness draws its input below the fold.
    function layout(): void {
      const more = extra();
      const ask = asking();
      if (connection.ptySize === true) {
        panning = true;
        const box = boxToReport(fit.proposeDimensions(), true, own());
        const asked = box && { rows: box.rows + ask + tall, cols: box.cols };
        if (asked && (asked.rows !== reported?.rows || asked.cols !== reported?.cols)) {
          reported = asked;
          connection.resize(sessionId, asked.rows, asked.cols);
        }
      } else {
        panning = more > 0;
        fit.fit();
        if (more > 0) terminal.resize(terminal.cols, terminal.rows + ask);
        connection.resize(sessionId, terminal.rows, terminal.cols);
      }
      remark();
      // Cut at a row, whatever the box height, so no sliver of the hidden box shows.
      requestAnimationFrame(() => {
        if (terminal.element) terminal.element.style.clipPath = more > 0 ? `inset(0 0 ${tailPx(more)}px 0)` : "";
        if (more > 0) anchor(true);
      });
    }
    panning = connection.ptySize === true;
    if (!panning) fit.fit();
    // xterm measured a cell with the fallback face before this font arrived. Setting the
    // option again measures the real one, so the grid is the same on every load.
    let alive = true;
    const remeasure = () => {
      if (!alive) return;
      const family = terminal.options.fontFamily;
      terminal.options.fontFamily = `${family}, monospace`;
      terminal.options.fontFamily = family;
      layout();
      anchor(true);
    };
    void document.fonts?.load(`${terminal.options.fontSize}px "JetBrains Mono"`).then(remeasure, () => {});
    // xterm draws a new height a frame later, so the anchor runs again once it has.
    const reanchor = (follow: boolean) => requestAnimationFrame(() => anchor(follow));
    afterSize = reanchor;
    refit = () => requestAnimationFrame(() => {
      layout();
      anchor(true);
      reanchor(true);
    });
    const observer = new ResizeObserver(() => {
      narrow = host.clientWidth < NARROW_PX;
      const follow = atBottom();
      layout();
      anchor(follow);
      reanchor(follow);
    });
    observer.observe(host);
    // A pause lets a redraw finish, so a half-drawn box does not count as a box that left.
    let settle: ReturnType<typeof setTimeout> | undefined;
    watchHarness = () => {
      if (!cropping()) return;
      clearTimeout(settle);
      settle = setTimeout(() => {
        const rows = screenRows(terminal.buffer.active, terminal.rows);
        const found = harnessBox(rows);
        const next = found ? rows.length - found.top : 0;
        if (next === hidden && (found?.height ?? 0) === boxRows) return;
        hidden = next;
        boxRows = found?.height ?? 0;
        layout();
        anchor(true);
      }, HIDE_SETTLE_MS);
    };
    // Throttled, since a redraw is many writes. It re-arms while rows are held, to let them go.
    const TALL_SETTLE_MS = 350;
    watchTall = () => {
      if (connection.ptySize !== true || tallTimer) return;
      tallTimer = setTimeout(() => {
        tallTimer = undefined;
        const choice = detectChoice(screenRows(terminal.buffer.active, terminal.rows));
        const now = performance.now();
        if (choice) lastDialog = now;
        const next = nextTall(tall, {
          dialog: choice !== null,
          clipped: (choice?.hidden ?? 0) > 0,
          honored: terminal.rows >= (reported?.rows ?? 0),
          idleMs: now - lastDialog,
        });
        if (next !== tall) {
          tall = next;
          layout();
          anchor(true);
        }
        if (tall > 0) watchTall();
      }, TALL_SETTLE_MS);
    };
    // A soft keyboard would only be a second input, so a tap on the terminal does not raise one.
    const shapeForPhone = () => {
      terminal.textarea?.setAttribute("inputmode", cropping() ? "none" : "text");
      if (!cropping()) hidden = boxRows = 0;
      layout();
      anchor(true);
    };
    phone.addEventListener("change", shapeForPhone);
    terminal.textarea?.setAttribute("inputmode", cropping() ? "none" : "text");
    // A terminal that types nothing has no use for Tab, so it leaves instead of trapping focus.
    terminal.attachCustomKeyEventHandler((event) => {
      if (event.type === "keydown" && wantsFind(event)) {
        event.preventDefault();
        openFind();
        return false;
      }
      return !(locked && event.key === "Tab");
    });
    // The replay is the only thing an idle seat sends, so a pane that is still empty asks again.
    let asked = 0;
    let watching: ReturnType<typeof setTimeout> | undefined;
    const empty = () => isBlank(screenRows(terminal.buffer.active, terminal.rows));
    noteDrawn = () => {
      if (!waiting && asked === 0) return;
      if (empty()) return;
      waiting = "";
      asked = 0;
      clearTimeout(watching);
    };
    function watch(): void {
      clearTimeout(watching);
      const wait = watchDelay(asked);
      if (wait === null) {
        waiting = "This seat's screen has not arrived. Select the seat again to ask once more.";
        return;
      }
      watching = setTimeout(() => {
        if (!empty()) return noteDrawn();
        asked += 1;
        waiting = "Waiting for this seat's screen. Asking the host again.";
        // The replay is the whole history, so the half-drawn screen goes before it.
        afterQueued(terminal, "clearing for the replay", () => {
          terminal.reset();
          remark();
          trackView();
        });
        connection.replay(sessionId);
        watch();
      }, wait);
    }
    // A hidden window pauses painting, and a pane left blank then is not told to try again.
    const wake = () => {
      if (document.visibilityState !== "visible") return;
      safely("repainting", () => terminal.refresh(0, terminal.rows - 1));
      if (empty()) {
        asked = 0;
        watch();
      }
    };
    document.addEventListener("visibilitychange", wake);
    window.addEventListener("focus", wake);
    const input = terminal.onData((data) => {
      if (!locked) connection.input(sessionId, data);
    });
    const unsubscribe = connection.subscribe((event) => {
      if (event.type === "output" && event.sessionId === sessionId) {
        // New lines are counted from xterm's own rows, which cost no layout read.
        const counting = latest.away;
        const before = linesAbove(terminal.buffer.active);
        terminal.write(event.data, () => {
          afterWrite();
          if (counting && latest.away) latest = wrote(latest, before, linesAbove(terminal.buffer.active));
          trackView();
        });
      }
      // The host attaches this seat again with replay right after, so the old screen would print twice.
      else if (event.type === "size" && event.sessionId === sessionId) {
        // Behind the bytes already queued, which the harness drew for the size before this one.
        afterQueued(terminal, "sizing to the host", () => {
          const follow = atBottom();
          terminal.resize(event.cols, event.rows);
          remark();
          anchor(follow);
          afterSize(follow);
        });
      } else if (event.type === "reconnected") {
        afterQueued(terminal, "clearing after reconnect", () => {
          terminal.reset();
          remark();
          trackView();
        });
        asked = 0;
        watch();
      }
    });
    // Subscribed first, so the replay the attach triggers is not missed.
    const box = fit.proposeDimensions() ?? own();
    connection.attach(sessionId, panning ? box.rows : terminal.rows, panning ? box.cols : terminal.cols);
    if (panning) reported = box;
    watch();
    return () => {
      alive = false;
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
      clearTimeout(watching);
      phone.removeEventListener("change", shapeForPhone);
      handheldQuery.removeEventListener("change", onHandheld);
      clearTimeout(settle);
      clearTimeout(tallTimer);
      unsubscribe();
      connection.detach(sessionId);
      input.dispose();
      host.removeEventListener("scroll", trackView);
      cancelAnimationFrame(tracking);
      clearTimeout(marking);
      host.removeEventListener("scroll", edges);
      host.removeEventListener("scroll", onPin);
      host.removeEventListener("click", onTap);
      grown.disconnect();
      host.removeEventListener("touchstart", onTouchStart);
      host.removeEventListener("touchmove", onTouchMove);
      host.removeEventListener("touchend", onTouchEnd);
      host.removeEventListener("touchcancel", stopCoast);
      stopCoast();
      observer.disconnect();
      terminal.dispose();
    };
  });

  $effect(() => {
    const off = locked;
    if (!term?.textarea) return;
    term.options.disableStdin = off;
    term.textarea.readOnly = off;
    term.textarea.setAttribute("aria-label", off ? "Terminal input, read only" : "Terminal input");
  });

  $effect(() => {
    void messages.length;
    markEnvelopes();
  });

  /** Opens find over the terminal, seeded with a one-line selection. The composer asks for it too. */
  export function openFind(): void {
    const picked = term?.getSelection() ?? "";
    findSeed = picked.includes("\n") ? "" : picked;
    finding = true;
  }

  function setTextSize(next: number): void {
    if (next === textSize) return;
    textSize = next;
    storeTextSize(next);
    if (term) term.options.fontSize = next;
    refit();
  }

  function setWrap(next: boolean): void {
    wrapping = next;
    storeWrap(sessionId, next);
    // The sideways pan has no use while lines fold, and a pan left behind would hide column 0.
    if (next && host) host.scrollLeft = 0;
    paintStripes();
    refit();
  }

  function closeFind(): void {
    finding = false;
    term?.focus();
  }

  function jump(): void {
    term?.scrollToBottom();
    anchor(true);
    trackView();
    term?.focus();
  }
  const jumpWords = $derived(jumpLabel(latest.unseen));
</script>

<div class="term-frame">
  <div class="term-host" class:panning class:wrapping role="region" aria-label={`Terminal for ${label}`} bind:this={host}>
    <p class="waiting" role="status">{waiting}</p>
  </div>
  <span class="edge left" class:on={past.left} aria-hidden="true">‹</span>
  <span class="edge right" class:on={past.right} aria-hidden="true">›</span>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  {#if showSize || canWrap}
  <div class="size" onkeydown={(event) => event.key === "Escape" && (sizing = false)}>
    {#if canWrap}
      <button type="button" class="wrap" aria-pressed={wrapping} aria-label="Wrap long lines" onclick={() => setWrap(!wrapping)}>Wrap</button>
    {/if}
    {#if sizing}
      <div class="size-row" role="group" aria-label="Text size">
        <button type="button" aria-label="Smaller text" disabled={textSize === TEXT_SIZES[0]} onclick={() => setTextSize(stepTextSize(textSize, -1))}>A−</button>
        <output aria-live="polite">{textSize}px</output>
        <button type="button" aria-label="Larger text" disabled={textSize === TEXT_SIZES[TEXT_SIZES.length - 1]} onclick={() => setTextSize(stepTextSize(textSize, 1))}>A+</button>
      </div>
    {/if}
    {#if showSize}
      <button type="button" class="size-open" aria-label="Text size" aria-expanded={sizing} onclick={() => (sizing = !sizing)}>Aa</button>
    {/if}
  </div>
  {/if}
  <span class="pan" class:on={past.left || past.right} aria-hidden="true"><i style:left={`${thumb.x}%`} style:width={`${thumb.w}%`}></i></span>
  {#if finding && search}
    <FindBar {search} initial={findSeed} onclose={closeFind} />
  {/if}
  {#if latest.away && !covered}
    <button type="button" class="jump" aria-label={jumpWords.spoken} onclick={jump}>
      <span aria-hidden="true">↓</span>
      <span aria-hidden="true">Jump to latest</span>
      {#if jumpWords.count}<span class="count" aria-hidden="true">{jumpWords.count}</span>{/if}
    </button>
  {/if}
</div>

<style>
  .term-frame { position: relative; flex: 1 1 auto; min-height: 240px; min-width: 0; display: flex; flex-direction: column; }
  .term-host { position: relative; flex: 1 1 auto; min-height: 0; padding: 12px 0 12px 16px; background: var(--terminal); overflow: hidden; }
  .waiting { position: absolute; z-index: 1; top: 12px; left: 16px; right: 16px; margin: 0; color: var(--muted); font-size: 13px; pointer-events: none; }
  /* The client scrolls a vertical drag itself, so the browser keeps only the sideways pan and pinch. */
  .term-host { touch-action: pan-x pinch-zoom; }
  .term-host.panning { overflow: auto; }
  .term-host.panning :global(.xterm) { width: max-content; }
  /* Rows fold at the box's width. Inline styles carry xterm's fixed sizes, so these override them. */
  .term-host.wrapping { overflow-x: hidden; }
  .term-host.wrapping :global(.xterm) { width: 100%; }
  .term-host.wrapping :global(.xterm-screen) { width: 100% !important; height: auto !important; }
  .term-host.wrapping :global(.xterm-rows) { position: static !important; width: 100%; white-space: pre-wrap !important; overflow-wrap: anywhere; }
  .term-host.wrapping :global(.xterm-rows > div) { width: auto !important; height: auto !important; overflow: visible !important; }
  /* A blank row keeps one line's height. */
  .term-host.wrapping :global(.xterm-rows > div::after) { content: "\200b"; }
  .term-host.wrapping :global(.xterm-rows span) { display: inline !important; height: auto !important; }
  /* xterm parks the input at the cursor's cell, which lies off the right edge on a folded screen and would scroll the box to it. The box's foot is where a pinned view already looks. */
  .term-host.wrapping :global(.xterm-helpers) { position: static; }
  .term-host.wrapping :global(.xterm-helper-textarea) { left: 0 !important; top: auto !important; bottom: 0; }
  /* Stripes are placed by row number, which a folded row breaks, so they are painted on the rows instead. */
  .term-host.wrapping :global(.xterm-decoration-container) { display: none; }
  .term-host.wrapping :global(.xterm-rows > div) { position: relative; }
  .term-host.wrapping :global(.xterm-rows > div[data-stripe]::before) { content: ""; position: absolute; left: -12px; top: 0; bottom: 0; width: 4px; background: var(--stripe); }
  .size .wrap[aria-pressed="true"] { border-color: var(--brand); background: color-mix(in srgb, var(--brand) 22%, var(--surface)); }
  .jump { position: absolute; z-index: 3; right: 28px; bottom: 16px; min-height: 44px; display: inline-flex; align-items: center; gap: 8px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--brand); background: var(--surface); color: var(--text); font: 600 14px var(--font-body); box-shadow: 0 6px 24px var(--shade); }
  .count { min-width: 22px; height: 22px; padding: 0 6px; box-sizing: border-box; border-radius: 11px; background: var(--brand); color: var(--brand-ink); font: 700 12px/22px var(--font-body); text-align: center; }
  .edge { display: flex; position: absolute; z-index: 2; top: 0; bottom: 0; width: 28px; align-items: center; color: var(--text); font: 600 22px var(--font-body); pointer-events: none; opacity: 0; transition: opacity 0.15s; }
  .edge.left { left: 0; justify-content: flex-start; padding-left: 4px; background: linear-gradient(to right, var(--terminal-veil), transparent); }
  .edge.right { right: 0; justify-content: flex-end; padding-right: 4px; background: linear-gradient(to left, var(--terminal-veil), transparent); }
  .edge.on { opacity: 1; }
  .size { position: absolute; z-index: 3; top: 12px; right: 28px; display: flex; align-items: center; gap: 6px; }
  .size-row { display: flex; align-items: center; gap: 4px; padding: 4px; border-radius: 12px; border: 1px solid var(--control-line); background: var(--surface); box-shadow: 0 6px 24px var(--shade); }
  .size button { min-width: 44px; min-height: 44px; padding: 0 10px; border-radius: 10px; border: 1px solid var(--control-line); background: var(--surface); color: var(--text); font: 600 15px var(--font-body); }
  .size button:disabled { opacity: 0.4; }
  .size output { min-width: 44px; text-align: center; color: var(--muted); font: 600 13px var(--font-body); }
  .pan { position: absolute; z-index: 2; left: 16px; right: 16px; bottom: 3px; height: 4px; border-radius: 2px; background: var(--track); pointer-events: none; opacity: 0; transition: opacity 0.15s; }
  .pan.on { opacity: 1; }
  .pan i { position: absolute; top: 0; bottom: 0; min-width: 12px; border-radius: 2px; background: var(--brand); }
  .size button:focus-visible, .jump:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  @media (max-width: 720px) {
    .jump { right: 12px; bottom: 12px; }
    .size { right: 12px; }
  }
</style>
