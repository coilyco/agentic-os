<script lang="ts">
  import type { ISearchOptions, SearchAddon } from "@xterm/addon-search";
  import { onMount } from "svelte";
  import { findColours, readToken } from "../lib/brand";
  import { findCountText } from "../lib/terminal-extras";

  let { search, initial = "", onclose }: { search: SearchAddon; initial?: string; onclose: () => void } = $props();

  // Hex only, since xterm paints these itself, so they come from the accent token. The fills stay dark.
  const options: ISearchOptions = {
    caseSensitive: false,
    decorations: findColours(readToken("--treehouse", "#66dd8a"), readToken("--ground", "#0b0c10")),
  };
  // svelte-ignore state_referenced_locally
  let query = $state(initial);
  let results = $state({ resultIndex: -1, resultCount: 0 });
  let input: HTMLInputElement;

  onMount(() => {
    const listening = search.onDidChangeResults((next) => (results = next));
    input.focus();
    input.select();
    if (query) search.findNext(query, { ...options, incremental: true });
    return () => {
      listening.dispose();
      search.clearDecorations();
    };
  });

  function change(): void {
    if (query) return void search.findNext(query, { ...options, incremental: true });
    search.clearDecorations();
    results = { resultIndex: -1, resultCount: 0 };
  }

  function step(back: boolean): void {
    if (!query) return;
    if (back) search.findPrevious(query, options);
    else search.findNext(query, options);
  }

  function keys(event: KeyboardEvent): void {
    if (event.key === "Enter") {
      event.preventDefault();
      step(event.shiftKey);
    } else if (event.key === "Escape") {
      event.preventDefault();
      onclose();
    }
  }
</script>

<div class="find" role="search">
  <input bind:this={input} bind:value={query} type="text" aria-label="Find in output" placeholder="Find in output" autocomplete="off" spellcheck="false" oninput={change} onkeydown={keys} />
  <span class="count" role="status">{findCountText(query, results)}</span>
  <button type="button" aria-label="Previous match" onclick={() => step(true)}>↑</button>
  <button type="button" aria-label="Next match" onclick={() => step(false)}>↓</button>
  <button type="button" aria-label="Close find" onclick={onclose}>✕</button>
</div>

<style>
  .find { position: absolute; z-index: 4; top: 12px; right: 28px; display: flex; align-items: center; gap: 6px; padding: 6px; border-radius: 10px; border: 1px solid var(--control-line); background: var(--surface); box-shadow: 0 6px 24px #000a; }
  input { width: 200px; min-height: 36px; padding: 0 10px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font: 14px var(--font-body); }
  input:focus-visible, button:focus-visible { outline: 2px solid var(--brand); outline-offset: 1px; }
  .count { min-width: 56px; font-size: 13px; color: var(--muted); text-align: center; white-space: nowrap; }
  button { min-width: 36px; min-height: 36px; border-radius: 8px; border: 1px solid transparent; background: transparent; color: var(--text); font-size: 16px; }
  button:hover { border-color: var(--control-line); background: #1d1729; }
  @media (pointer: coarse) {
    input, button { min-height: 44px; }
    button { min-width: 44px; }
  }
  @media (max-width: 720px) {
    .find { right: 12px; left: 12px; }
    input { flex: 1; width: auto; min-width: 0; }
  }
</style>
