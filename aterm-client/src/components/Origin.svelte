<script lang="ts">
  import { originWords } from "../lib/idle-close";
  import type { Session } from "../lib/protocol";

  // Where a seat started (COI-3504): a terminal window on the host, or none. Unknown shows nothing.
  let { origin }: { origin: Session["origin"] } = $props();
  const words = $derived(originWords(origin));
</script>

{#if origin}
  <svg class="origin" data-origin={origin} role="img" aria-label={words} viewBox="0 0 16 16" width="14" height="14">
    <title>{words}</title>
    {#if origin === "window"}
      <rect x="1.5" y="2.5" width="13" height="11" rx="2" fill="none" stroke="currentColor" stroke-width="1.5" />
      <path d="M4.5 6.5l2 1.5-2 1.5M8 10h3" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
    {:else}
      <rect x="1.5" y="2.5" width="13" height="11" rx="2" fill="none" stroke="currentColor" stroke-width="1.5" stroke-dasharray="2.2 2.2" />
    {/if}
  </svg>
{/if}

<style>
  .origin { flex: none; color: var(--muted); vertical-align: -2px; }
</style>
