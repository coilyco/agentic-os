<script lang="ts">
  import { FitAddon } from "@xterm/addon-fit";
  import { Terminal, type IDisposable } from "@xterm/xterm";
  import "@xterm/xterm/css/xterm.css";
  import { onMount } from "svelte";
  import { envelopeRows } from "../lib/messages";
  import { harnessInputRows } from "../lib/harness";
  import { boxToReport } from "../lib/pty-size";
  import { screenRows } from "../lib/screen";
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
  }: {
    connection: HostConnection;
    sessionId: string;
    label: string;
    accent: string;
    messages: PeerMessage[];
    colorOf: (role: string) => string;
    /** The visible rows after each write, for readers such as the choice detector. */
    onscreen?: (rows: string[]) => void;
    /** The daemon refuses this connection's typing, so keys go nowhere. */
    locked?: boolean;
    /** On a phone the composer is the one input, so the harness's own input box is scrolled out of view. */
    cropHarness?: boolean;
  } = $props();

  let host: HTMLDivElement;
  let term: Terminal | undefined;
  // The box scrolls when the terminal is bigger than it: a PTY sized to a larger client, or a cropped harness box.
  let panning = $state(false);
  const HIDE_SETTLE_MS = 350;
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
    onscreen(screenRows(term.buffer.active, term.rows));
  }

  let watchHarness = () => {};
  // The input and the newest text sit at the bottom left, so that corner stays in view
  // unless the person has panned away. Cropped rows lie below it.
  let hiddenPx = () => 0;
  const atBottom = () => host.scrollHeight - host.clientHeight - host.scrollTop - hiddenPx() < 2;
  function anchor(follow: boolean): void {
    if (follow) host.scrollTop = Math.max(0, host.scrollHeight - host.clientHeight - hiddenPx());
    host.scrollLeft = 0;
  }

  function afterWrite(): void {
    markEnvelopes();
    reportScreen();
    watchHarness();
  }

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
      lineHeight: phone.matches ? 1.15 : 1.3,
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
    terminal.open(host);
    const cropping = () => cropHarness && phone.matches;
    let hidden = 0;
    const extra = () => (cropping() ? hidden : 0);
    const cell = () => (host.querySelector<HTMLElement>(".xterm-screen")?.clientHeight ?? 0) / Math.max(terminal.rows, 1);
    // The window ends at the last shown row, not at the padding below it, which would show a sliver of the box.
    hiddenPx = () => (extra() > 0 ? extra() * cell() + parseFloat(getComputedStyle(host).paddingBottom) : 0);
    // This client's capacity is what its box can hold, which is not what xterm currently is.
    const own = () => ({ rows: terminal.rows, cols: terminal.cols });
    let reported: { rows: number; cols: number } | undefined;
    // Rows the box cannot show are asked for anyway, so the harness draws its input below the fold.
    function layout(): void {
      const more = extra();
      if (connection.ptySize === true) {
        panning = true;
        const box = boxToReport(fit.proposeDimensions(), true, own());
        const asked = box && { rows: box.rows + more, cols: box.cols };
        if (asked && (asked.rows !== reported?.rows || asked.cols !== reported?.cols)) {
          reported = asked;
          connection.resize(sessionId, asked.rows, asked.cols);
        }
      } else {
        panning = more > 0;
        fit.fit();
        if (more > 0) terminal.resize(terminal.cols, terminal.rows + more);
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
    const observer = new ResizeObserver(() => {
      const follow = atBottom();
      layout();
      anchor(follow);
    });
    observer.observe(host);
    // A pause lets a redraw finish, so a half-drawn box does not count as a box that left.
    let settle: ReturnType<typeof setTimeout> | undefined;
    watchHarness = () => {
      if (!cropping()) return;
      clearTimeout(settle);
      settle = setTimeout(() => {
        const next = harnessInputRows(screenRows(terminal.buffer.active, terminal.rows));
        if (next === hidden) return;
        hidden = next;
        layout();
        anchor(true);
      }, HIDE_SETTLE_MS);
    };
    // A soft keyboard would only be a second input, so a tap on the terminal does not raise one.
    const shapeForPhone = () => {
      terminal.textarea?.setAttribute("inputmode", cropping() ? "none" : "text");
      terminal.options.lineHeight = phone.matches ? 1.15 : 1.3;
      if (!cropping()) hidden = 0;
      layout();
      anchor(true);
    };
    phone.addEventListener("change", shapeForPhone);
    terminal.textarea?.setAttribute("inputmode", cropping() ? "none" : "text");
    // A terminal that types nothing has no use for Tab, so it leaves instead of trapping focus.
    terminal.attachCustomKeyEventHandler((event) => !(locked && event.key === "Tab"));
    const input = terminal.onData((data) => {
      if (!locked) connection.input(sessionId, data);
    });
    const unsubscribe = connection.subscribe((event) => {
      if (event.type === "output" && event.sessionId === sessionId) terminal.write(event.data, afterWrite);
      // The host attaches this seat again with replay right after, so the old screen would print twice.
      else if (event.type === "size" && event.sessionId === sessionId) {
        const follow = atBottom();
        terminal.resize(event.cols, event.rows);
        remark();
        anchor(follow);
      } else if (event.type === "reconnected") {
        terminal.reset();
        remark();
      }
    });
    // Subscribed first, so the replay the attach triggers is not missed.
    const box = fit.proposeDimensions() ?? own();
    connection.attach(sessionId, panning ? box.rows : terminal.rows, panning ? box.cols : terminal.cols);
    if (panning) reported = box;
    return () => {
      phone.removeEventListener("change", shapeForPhone);
      clearTimeout(settle);
      unsubscribe();
      connection.detach(sessionId);
      input.dispose();
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
</script>

<div class="term-host" class:panning role="region" aria-label={`Terminal for ${label}`} bind:this={host}></div>

<style>
  .term-host { flex: 1 1 auto; min-height: 240px; padding: 12px 0 12px 16px; background: var(--terminal); overflow: hidden; }
  .term-host.panning { overflow: auto; }
  .term-host.panning :global(.xterm) { width: max-content; }
</style>
