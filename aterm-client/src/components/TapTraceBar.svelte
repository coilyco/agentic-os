<script lang="ts">
  import { onMount } from "svelte";
  import { PLACE } from "../lib/build";
  import { format, headerFor, onChange, records } from "../lib/tap-trace";

  let count = $state(0);
  let copied = $state(false);
  let shown = $state("");

  onMount(() => {
    count = records().length;
    return onChange(() => (count = records().length));
  });

  // One tap copies the record. The real daemon takes no feedback record yet (COI-3345), so the clipboard is the way out.
  async function copy(): Promise<void> {
    const body = format(records(), headerFor(window, __ATERM_BUILD__, PLACE));
    try {
      await navigator.clipboard.writeText(body);
      copied = true;
      setTimeout(() => (copied = false), 2500);
    } catch {
      shown = body;
    }
  }
</script>

<!-- Events from this bar are left out of the record, so using it does not change what it holds. -->
<aside class="bar" data-taptrace aria-label="Tap trace">
  <button type="button" disabled={count === 0} aria-label={copied ? "Trace copied" : `Copy trace, ${count} events`} onclick={() => void copy()}>{copied ? "Copied" : `Copy ${count}`}</button>
  <p class="visually-hidden" role="status">{copied ? "Copied. Paste it into a message to a seat." : ""}</p>
  {#if shown}
    <div class="fallback">
      <p>Could not copy. Select the text and copy it.</p>
      <textarea class="mono" readonly rows="8" value={shown} aria-label="Trace text"></textarea>
      <button type="button" onclick={() => (shown = "")}>Close</button>
    </div>
  {/if}
</aside>

<style>
  .bar { position: fixed; z-index: 2000; top: max(2px, env(safe-area-inset-top)); right: 2px; display: flex; flex-direction: column; align-items: flex-end; gap: 4px; max-width: calc(100vw - 4px); }
  button { min-height: 44px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--brand); background: var(--raised); color: var(--text); font: 600 13px var(--font-body); }
  button:not(:disabled) { background: var(--brand); color: var(--brand-ink); }
  button:disabled { opacity: 0.5; }
  .fallback { width: min(92vw, 420px); display: flex; flex-direction: column; gap: 6px; padding: 8px; border-radius: 10px; border: 1px solid var(--brand); background: var(--raised); color: var(--text); font-size: 12px; }
  .fallback p { margin: 0; }
  textarea { width: 100%; box-sizing: border-box; background: var(--terminal); color: var(--text); border: 1px solid var(--control-line); border-radius: 8px; font-size: 11px; }
</style>
