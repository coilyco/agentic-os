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
  import { boxToReport } from "../lib/pty-size";
  import { linkTarget, opensLink, wantsFind } from "../lib/terminal-extras";
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

  // Stamp every envelope the harness has echoed so far with its sender's colour.
  function markEnvelopes(): void {
    if (!term) return;
    const buffer = term.buffer.active;
    const lines = logicalLines(term);
    for (const [index, message] of envelopeRows(lines.map((line) => line.text), messages)) {
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
  let measuring = 0;
  // A frame later, so reading the layout never lands between a resize and its anchor.
  function edges(): void {
    if (measuring) return;
    measuring = requestAnimationFrame(() => {
      measuring = 0;
      if (!host) return;
      const left = host.scrollLeft > 2;
      const right = host.scrollLeft + host.clientWidth < host.scrollWidth - 2;
      if (left !== past.left || right !== past.right) past = { left, right };
    });
  }

  // A step that throws here would stop xterm taking any more writes, so each runs on its own.
  function afterWrite(): void {
    safely("marking envelopes", markEnvelopes);
    safely("reading the screen", reportScreen);
    safely("watching the harness", () => watchHarness());
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
    const terminal = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      fontSize: 14,
      lineHeight: LINE_HEIGHT,
      scrollback: SCROLLBACK,
      allowProposedApi: true,
      theme: { background: "#101216", foreground: "#d4d8e0", cursor: accent, selectionBackground: `${accent}55` },
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
          const href = linkTarget(uri);
          if (href && opensLink(click, touch.matches)) window.open(href, "_blank", "noopener,noreferrer");
        },
        { hover: () => (host.title = touch.matches ? "" : `${verb} to open`), leave: () => (host.title = "") },
      ),
    );
    terminal.open(host);
    // Above the end when xterm's own view or the pan box is scrolled away from the newest row.
    const above = () => linesAbove(terminal.buffer.active) + (panning && !atBottom() ? 1 : 0);
    trackView = () => (latest = scrolled(latest, above()));
    terminal.onScroll(trackView);
    host.addEventListener("scroll", trackView);
    host.addEventListener("scroll", edges);
    const cropping = () => cropHarness;
    let hidden = 0;
    // Blank rows under a floating box hide with it, but only its height is asked of the PTY.
    let boxRows = 0;
    const extra = () => (cropping() ? hidden : 0);
    const asking = () => (cropping() ? boxRows : 0);
    const cell = () => (host.querySelector<HTMLElement>(".xterm-screen")?.clientHeight ?? 0) / Math.max(terminal.rows, 1);
    // The window ends at the last shown row, not at the padding below it, which would show a sliver of the box.
    hiddenPx = () => (extra() > 0 ? extra() * cell() + parseFloat(getComputedStyle(host).paddingBottom) : 0);
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
        const asked = box && { rows: box.rows + ask, cols: box.cols };
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
        if (terminal.element) terminal.element.style.clipPath = more > 0 ? `inset(0 0 ${more * cell()}px 0)` : "";
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
    const observer = new ResizeObserver(() => {
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
        const before = above();
        terminal.write(event.data, () => {
          afterWrite();
          latest = wrote(latest, before, above());
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
      clearTimeout(settle);
      unsubscribe();
      connection.detach(sessionId);
      input.dispose();
      host.removeEventListener("scroll", trackView);
      host.removeEventListener("scroll", edges);
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
  <div class="term-host" class:panning role="region" aria-label={`Terminal for ${label}`} bind:this={host}>
    <p class="waiting" role="status">{waiting}</p>
  </div>
  <span class="edge left" class:on={past.left} aria-hidden="true">‹</span>
  <span class="edge right" class:on={past.right} aria-hidden="true">›</span>
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
  .term-host.panning { overflow: auto; }
  .term-host.panning :global(.xterm) { width: max-content; }
  .jump { position: absolute; z-index: 3; right: 28px; bottom: 16px; min-height: 44px; display: inline-flex; align-items: center; gap: 8px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--brand); background: var(--surface); color: var(--text); font: 600 14px var(--font-body); box-shadow: 0 6px 24px #000a; }
  .count { min-width: 22px; height: 22px; padding: 0 6px; box-sizing: border-box; border-radius: 11px; background: var(--brand); color: var(--brand-ink); font: 700 12px/22px var(--font-body); text-align: center; }
  .edge { display: none; position: absolute; z-index: 2; top: 0; bottom: 0; width: 28px; align-items: center; color: var(--text); font: 600 22px var(--font-body); pointer-events: none; opacity: 0; transition: opacity 0.15s; }
  .edge.left { left: 0; justify-content: flex-start; padding-left: 4px; background: linear-gradient(to right, #101216dd, transparent); }
  .edge.right { right: 0; justify-content: flex-end; padding-right: 4px; background: linear-gradient(to left, #101216dd, transparent); }
  .edge.on { opacity: 1; }
  .jump:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  @media (max-width: 720px) {
    .jump { right: 12px; bottom: 12px; }
    .edge { display: flex; }
  }
</style>
