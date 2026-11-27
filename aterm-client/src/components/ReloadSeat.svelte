<script lang="ts">
  import { app, reloadable, reloadSeatNow } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";

  // Reload connectors for an idle seat (COI-3787). One tap asks, a second restarts it on its own conversation.
  let { session }: { session: Session } = $props();
  let asking = $state(false);
  const offered = $derived(reloadable(session));
  const last = $derived(app.reloads[session.id]);
</script>

{#if asking && offered}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <span class="ask" role="group" aria-label={`Reload ${session.identity}?`} onkeydown={(event) => event.key === "Escape" && (asking = false)}>
    <span class="words">Restart {session.identity} on this conversation, so it reads its connectors again?</span>
    <button type="button" class="yes" onclick={() => ((asking = false), reloadSeatNow(session))}>Reload</button>
    <button type="button" onclick={() => (asking = false)}>Keep</button>
  </span>
{:else if offered}
  <button type="button" aria-label={`Reload connectors for ${session.identity}`} onclick={() => (asking = true)}>Reload connectors</button>
{/if}
{#if last}<p class="note" class:refused={last.state === "refused"} role="status">{last.text}</p>{/if}

<style>
  button { min-height: 44px; min-width: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); }
  button:hover, button:focus-visible { border-color: var(--brand); }
  .ask { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; }
  .words { font-size: 13px; color: var(--muted); }
  .yes { border-color: var(--brand); }
  .note { flex-basis: 100%; margin: 0; font-size: 13px; color: var(--muted); }
  .note.refused { color: var(--warn-text); }
</style>
