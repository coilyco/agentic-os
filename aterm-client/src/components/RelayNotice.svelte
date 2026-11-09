<script lang="ts">
  import { openByPerson } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";

  /** The seat whose screen shows the card for a question this seat asked. */
  let { seat }: { seat: Session } = $props();
</script>

<p class="relayed" role="status">
  <span>Its question is on {seat.identity}'s screen.</span>
  <button type="button" onclick={() => openByPerson(seat.id)}>Open {seat.identity}</button>
</p>

<style>
  .relayed { margin: 0; padding: 8px 12px; display: flex; align-items: center; justify-content: space-between; gap: 8px 12px; flex-wrap: wrap; border-top: 1px solid var(--brand); background: var(--surface); color: var(--text); font-size: 14px; }
  button { min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--brand); background: transparent; color: var(--text); font-weight: 600; }
  button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
