<script lang="ts">
  import { onMount } from "svelte";
  import type { TranscriptChannel } from "../lib/protocol";
  import type { TranscriptMatch } from "../lib/transcript";
  import { searchStatusText, stepMatch } from "../lib/transcript-search";

  // The host searches the seat's whole transcript, and this bar steps between what it found (COI-3797).
  // `initial` opens on a phrase and one match, as a tap in the every-seat search does.
  let { channel, sessionId, initial, onshow, onclose }: { channel: TranscriptChannel; sessionId: string; initial?: { query: string; id: string }; onshow: (match: TranscriptMatch | null, query: string) => void; onclose: () => void } = $props();

  const WAIT_MS = 300;
  let query = $state("");
  let matches = $state<TranscriptMatch[]>([]);
  let total = $state(0);
  let partial = $state(false);
  let at = $state(-1);
  let phase = $state<"idle" | "searching" | "done" | "failed">("idle");
  let input: HTMLInputElement;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let asked = 0;
  const status = $derived(searchStatusText({ query, phase, at, count: matches.length, total, partial }));

  onMount(() => {
    input.focus();
    if (initial) {
      query = initial.query;
      phase = "searching";
      void run(initial.id);
    }
    return () => clearTimeout(timer);
  });

  function changed(): void {
    clearTimeout(timer);
    if (!query.trim()) {
      asked++;
      matches = [];
      at = -1;
      phase = "idle";
      onshow(null, "");
      return;
    }
    phase = "searching";
    timer = setTimeout(() => void run(), WAIT_MS);
  }

  async function run(start?: string): Promise<void> {
    if (!channel.search) return;
    const mine = ++asked;
    const phrase = query.trim();
    try {
      const reply = await channel.search(sessionId, phrase);
      if (mine !== asked) return;
      matches = reply.matches;
      total = reply.total;
      partial = reply.partial;
      phase = "done";
      // The newest match first, since the view sits at the newest line.
      const wanted = start ? matches.findIndex((match) => match.id === start) : -1;
      at = wanted >= 0 ? wanted : matches.length - 1;
      onshow(matches[at] ?? null, phrase);
    } catch {
      if (mine === asked) phase = "failed";
    }
  }

  function step(older: boolean): void {
    if (phase !== "done" || !matches.length) return;
    at = stepMatch(at, matches.length, older);
    onshow(matches[at] ?? null, query.trim());
  }

  function keys(event: KeyboardEvent): void {
    if (event.key === "Enter") {
      event.preventDefault();
      clearTimeout(timer);
      // Enter on a fresh phrase searches now. After that it steps back through older matches.
      if (phase === "searching") void run();
      else step(!event.shiftKey);
    } else if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      onclose();
    }
  }
</script>

<div class="search" role="search">
  <input bind:this={input} bind:value={query} type="search" aria-label="Search this seat's transcript" placeholder="Search transcript" autocomplete="off" spellcheck="false" enterkeyhint="search" oninput={changed} onkeydown={keys} />
  <span class="count" role="status" aria-live="polite">{status}</span>
  <button type="button" aria-label="Older match" title="Older match (Enter)" disabled={!matches.length} onclick={() => step(true)}>↑</button>
  <button type="button" aria-label="Newer match" title="Newer match (Shift+Enter)" disabled={!matches.length} onclick={() => step(false)}>↓</button>
  <button type="button" aria-label="Close search" onclick={onclose}>✕</button>
</div>

<style>
  .search { flex: none; display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-bottom: 1px solid var(--line); background: var(--surface); }
  input { flex: 1 1 auto; min-width: 0; min-height: 36px; padding: 0 10px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font: 14px var(--font-body); }
  input:focus-visible, button:focus-visible { outline: 2px solid var(--brand); outline-offset: 1px; }
  .count { flex: none; min-width: 64px; font-size: 13px; color: var(--muted); text-align: center; white-space: nowrap; }
  button { flex: none; min-width: 36px; min-height: 36px; border-radius: 8px; border: 1px solid transparent; background: transparent; color: var(--text); font-size: 16px; }
  button:hover:not(:disabled) { border-color: var(--control-line); background: var(--selected); }
  button:disabled { opacity: 0.4; }
  @media (pointer: coarse) {
    input, button { min-height: 44px; }
    button { min-width: 44px; }
  }
  @media (max-width: 360px) {
    .count { min-width: 0; font-size: 12px; }
    button { min-width: 40px; }
  }
</style>
