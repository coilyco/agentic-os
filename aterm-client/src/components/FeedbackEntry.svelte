<script lang="ts">
  import { feedbackAvailability, openFeedback } from "../lib/feedback.svelte";

  // Where the menu closes itself on a phone, so the sheet opens over the seat and not over the menu.
  // `compact` is the phone bar's icon. It exists only while Send can work, since the menu entry says why not.
  let { onopen, compact = false }: { onopen?: () => void; compact?: boolean } = $props();
  const availability = $derived(feedbackAvailability());

  function open(): void {
    if (!availability.ok) return;
    onopen?.();
    openFeedback();
  }
</script>

{#if compact}
  {#if availability.ok}
    <button type="button" class="icon" aria-label="Send feedback" title="Send feedback" onclick={open}>
      <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true" focusable="false"><path d="M4 5.5h16v10H9.5L5.5 19v-3.5H4z" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" /></svg>
    </button>
  {/if}
{:else}
<!-- aria-disabled keeps the entry on the tab order, so the reason is read instead of the entry vanishing. -->
<button type="button" class="entry" aria-disabled={!availability.ok} aria-describedby={availability.ok ? undefined : "feedback-why"} onclick={open}>
  <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false"><path d="M4 5.5h16v10H9.5L5.5 19v-3.5H4z" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" /></svg>
  <span class="text">
    <span class="label">Send feedback</span>
    {#if !availability.ok}<span id="feedback-why" class="why">{availability.reason}</span>{/if}
  </span>
</button>
{/if}

<style>
  .icon { flex: none; width: 44px; height: 44px; display: inline-flex; align-items: center; justify-content: center; padding: 0; border: 1px solid var(--control-line); border-radius: 8px; background: transparent; color: var(--text-soft); }
  .icon:hover { color: var(--text); border-color: var(--brand); }
  .entry { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 44px; padding: 8px 12px; border: 1px solid var(--control-line); border-radius: 8px; background: transparent; color: var(--text-soft); text-align: left; }
  .entry:hover:not([aria-disabled="true"]) { color: var(--text); border-color: var(--brand); }
  .entry[aria-disabled="true"] { cursor: not-allowed; }
  .text { display: flex; flex-direction: column; min-width: 0; }
  .label { font-size: 15px; }
  .why { font-size: 12px; line-height: 1.3; color: var(--muted); }
</style>
