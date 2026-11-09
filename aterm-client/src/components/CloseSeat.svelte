<script lang="ts">
  import { app, closable, closeSeatNow } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";

  // Close for a seat that is fully idle (COI-3488). One tap asks, a second ends it.
  // `where` keeps the question to the control that was tapped: a row, or the seat's own view.
  let { session, needsKai, where, compact = false }: { session: Session; needsKai: boolean; where: "row" | "view"; compact?: boolean } = $props();
  const key = $derived(`${where}:${session.id}`);
  const offered = $derived(closable(session, needsKai));
  const asking = $derived(offered && app.closing === key);
</script>

{#if asking}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <span class="ask" role="group" aria-label={`Close ${session.identity}?`} class:compact onkeydown={(event) => event.key === "Escape" && (app.closing = null)}>
    <span class="words">Close {session.identity}?</span>
    <button type="button" class="yes" onclick={() => closeSeatNow(session)}>Close it</button>
    <button type="button" class="no" onclick={() => (app.closing = null)}>Keep</button>
  </span>
{:else if offered}
  <button type="button" class="close" class:compact aria-label={`Close ${session.identity}`} onclick={() => (app.closing = key)}>{compact ? `Close ${session.identity}` : "Close"}</button>
{/if}

<style>
  button { min-height: 44px; min-width: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); }
  button:hover, button:focus-visible { border-color: var(--danger); color: var(--danger-text); }
  .ask { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; }
  .words { font-size: 13px; color: var(--muted); }
  .yes { border-color: var(--danger); color: var(--danger-text); }
  .compact { width: 100%; }
  .ask.compact { flex-direction: column; align-items: stretch; text-align: center; }
</style>
