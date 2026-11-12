<script lang="ts">
  import { app, unlockAgain } from "../lib/app.svelte";

  // A brokered connection starts locked and unlocks itself with the account. This is for the one that did not take.
  const show = $derived(app.brokered && !app.typing.allowed);
  const failed = $derived(!app.typing.allowed && (app.typing.reason === "unlock_refused" || app.typing.reason === "unlock_failed"));
</script>

{#if show && failed}
  <div class="unlock">
    <button class="button primary" type="button" onclick={() => unlockAgain()}>Unlock again</button>
    <p class="hint">This asks your account for a fresh unlock. After a few refusals it reconnects to start over.</p>
  </div>
{/if}

<style>
  .unlock { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; margin-top: 12px; }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
</style>
