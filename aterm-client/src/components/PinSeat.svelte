<script lang="ts">
  import { pinnedNow, togglePinned } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";

  // Pin a seat to the top of the list (COI-3346). A toggle, so its state is announced.
  let { session }: { session: Session } = $props();
  const pinned = $derived(pinnedNow(session));
</script>

<button type="button" class="pin" aria-pressed={pinned} aria-label={`Pin ${session.identity}`} title={pinned ? "Pinned. Tap to unpin." : "Pin to the top"} onclick={() => togglePinned(session)}>
  <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
    <path d="M6 1.8h4l-.6 4.2 2.4 2.4v1H8.8V14L8 15l-.8-1V9.4H4.2v-1L6.6 6z" fill={pinned ? "currentColor" : "none"} stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" />
  </svg>
</button>

<style>
  .pin { flex: none; display: inline-flex; align-items: center; justify-content: center; width: 44px; min-height: 44px; border-radius: 8px; border: 1px solid transparent; background: transparent; color: var(--muted); }
  .pin:hover, .pin:focus-visible { border-color: var(--control-line); color: var(--text); }
  .pin[aria-pressed="true"] { color: var(--accent, var(--brand)); }
</style>
