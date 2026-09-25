<script lang="ts">
  import { FitAddon } from "@xterm/addon-fit";
  import { Terminal, type IDisposable } from "@xterm/xterm";
  import "@xterm/xterm/css/xterm.css";
  import { onMount } from "svelte";
  import { envelopeRows } from "../lib/messages";
  import type { HostConnection, PeerMessage } from "../lib/protocol";

  let {
    connection,
    sessionId,
    label,
    accent,
    messages,
    colorOf,
  }: {
    connection: HostConnection;
    sessionId: string;
    label: string;
    accent: string;
    messages: PeerMessage[];
    colorOf: (role: string) => string;
  } = $props();

  let host: HTMLDivElement;
  let term: Terminal | undefined;
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
      const span = lines.slice(index, index + 1 + message.body.split(/\r?\n/).length);
      const marker = term.registerMarker(first.start - (buffer.baseY + buffer.cursorY));
      if (!marker) continue;
      const color = colorOf(message.from.role);
      const decoration = term.registerDecoration({ marker, width: term.cols, height: span.reduce((sum, line) => sum + line.rows, 0), layer: "bottom" });
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
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host);
    fit.fit();
    const observer = new ResizeObserver(() => {
      fit.fit();
      remark();
    });
    observer.observe(host);
    const input = terminal.onData((data) => connection.input(sessionId, data));
    const unsubscribe = connection.subscribe((event) => {
      if (event.type === "output" && event.sessionId === sessionId) terminal.write(event.data, markEnvelopes);
    });
    return () => {
      unsubscribe();
      input.dispose();
      observer.disconnect();
      terminal.dispose();
    };
  });

  $effect(() => {
    void messages.length;
    markEnvelopes();
  });
</script>

<div class="term-host" role="region" aria-label={`Terminal for ${label}`} bind:this={host}></div>

<style>
  .term-host { flex: 1 1 auto; min-height: 240px; padding: 12px 0 12px 16px; background: var(--terminal); overflow: hidden; }
</style>
