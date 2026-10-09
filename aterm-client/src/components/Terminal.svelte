<script lang="ts">
  import { FitAddon } from "@xterm/addon-fit";
  import { Terminal, type IDisposable } from "@xterm/xterm";
  import "@xterm/xterm/css/xterm.css";
  import { onMount } from "svelte";
  import { envelopeRows } from "../lib/messages";
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
  } = $props();

  let host: HTMLDivElement;
  let term: Terminal | undefined;
  // The daemon sizes the PTY to the largest client, so this box pans a terminal that may be bigger.
  let panning = $state(false);
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

  function afterWrite(): void {
    markEnvelopes();
    reportScreen();
  }

  // The input and the newest text sit at the bottom left, so that corner stays in view
  // unless the person has panned away.
  function anchor(wasAtBottom: boolean): void {
    if (wasAtBottom) host.scrollTop = host.scrollHeight;
    host.scrollLeft = 0;
  }

  function remark(): void {
    decorations.forEach((decoration) => decoration.dispose());
    decorations = [];
    marked.clear();
    markEnvelopes();
  }

  onMount(() => {
    const terminal = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      fontSize: 14,
      lineHeight: 1.3,
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
    panning = connection.ptySize === true;
    // This client's capacity is what its box can hold, which is not what xterm currently is.
    const own = () => ({ rows: terminal.rows, cols: terminal.cols });
    let reported: { rows: number; cols: number } | undefined;
    const capacity = () => fit.proposeDimensions() ?? own();
    if (!panning) fit.fit();
    const observer = new ResizeObserver(() => {
      if (connection.ptySize === true) {
        panning = true;
        const box = boxToReport(fit.proposeDimensions(), true, own());
        if (box && (box.rows !== reported?.rows || box.cols !== reported?.cols)) {
          reported = box;
          connection.resize(sessionId, box.rows, box.cols);
        }
      } else {
        fit.fit();
        connection.resize(sessionId, terminal.rows, terminal.cols);
      }
      remark();
    });
    observer.observe(host);
    // A terminal that types nothing has no use for Tab, so it leaves instead of trapping focus.
    terminal.attachCustomKeyEventHandler((event) => !(locked && event.key === "Tab"));
    const input = terminal.onData((data) => {
      if (!locked) connection.input(sessionId, data);
    });
    const unsubscribe = connection.subscribe((event) => {
      if (event.type === "output" && event.sessionId === sessionId) terminal.write(event.data, afterWrite);
      // The host attaches this seat again with replay right after, so the old screen would print twice.
      else if (event.type === "size" && event.sessionId === sessionId) {
        const atBottom = host.scrollHeight - host.clientHeight - host.scrollTop < 2;
        terminal.resize(event.cols, event.rows);
        remark();
        anchor(atBottom);
      } else if (event.type === "reconnected") {
        terminal.reset();
        remark();
      }
    });
    // Subscribed first, so the replay the attach triggers is not missed.
    const box = capacity();
    connection.attach(sessionId, panning ? box.rows : terminal.rows, panning ? box.cols : terminal.cols);
    if (panning) reported = box;
    return () => {
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
