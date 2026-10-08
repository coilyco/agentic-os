<script lang="ts">
  import { currentInstallState, install, pwa } from "../lib/pwa.svelte";

  const state = $derived(currentInstallState());
  const shown = $derived(state.kind !== "installed" && (state.kind !== "menu" || pwa.waited));
</script>

{#if shown}
  <section class="install" aria-labelledby="install-title">
    <h2 id="install-title">Install aterm as an app</h2>
    {#if state.kind === "ready"}
      <p>It opens in its own window with its own icon, so it has an alt-tab entry and a Dock or app-list icon. Installing grants nothing new. A host still has to answer before any seat shows.</p>
      <button class="button primary" type="button" onclick={install}>Install aterm</button>
    {:else if state.kind === "menu"}
      <p>Your browser installs it from its own menu. A window opened that way works the same as this one.</p>
      <ul>
        <li><strong>Chrome</strong> - the install icon in the address bar, or Install app in the menu on Android.</li>
        <li><strong>Safari on a Mac</strong> - File, then Add to Dock.</li>
        <li><strong>Safari on iPhone or iPad</strong> - Share, then Add to Home Screen.</li>
      </ul>
    {:else}
      <p>This address cannot be installed. A browser installs only from https, or from localhost on this machine. Open the host by its tailnet name over https.</p>
    {/if}
    <p class="outcome" role="status">
      {pwa.outcome === "accepted" ? "Installing. Its window opens on its own." : pwa.outcome === "dismissed" ? "Not installed. The browser's menu still offers it." : ""}
    </p>
  </section>
{/if}

<style>
  .install { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  h2 { margin: 0; font-family: var(--font-display); font-size: 17px; font-weight: 600; }
  p { margin: 0; color: var(--text-soft); font-size: 15px; }
  ul { margin: 0; padding-left: 18px; color: var(--text-soft); font-size: 14px; display: flex; flex-direction: column; gap: 4px; }
  .outcome { min-height: 0; color: var(--muted); font-size: 14px; }
  .outcome:empty { display: none; }
</style>
