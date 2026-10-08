<script lang="ts">
  import { app, viewsFor } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";

  let { session }: { session: Session } = $props();
  const views = $derived(viewsFor(session.id));

  // AppView holds AppBridge and its peers (ext-apps, the MCP SDK, zod), which outweigh the rest of
  // the client, so it loads when a view first exists. The two empty states never pay for it.
  const appView = $derived(views.length > 0 ? import("./AppView.svelte") : undefined);
</script>

<div class="views">
  {#if !app.connection?.views}
    <p class="empty">This host doesn't forward MCP App views yet. Once its daemon does, a tool that returns a view opens it here, next to the terminal.</p>
  {:else if !appView}
    <p class="empty">No tool has returned a view in this session yet. When one does, it opens here, and you can use it the way {session.identity} does.</p>
  {:else}
    {#await appView}
      <p class="empty" role="status">Opening the view.</p>
    {:then { default: AppView }}
      {#each views as view (view.id)}
        <AppView {view} />
      {/each}
    {:catch}
      <!-- A browser keeps a failed dynamic import for the life of the page, so only a reload can retry it. -->
      <p class="empty" role="alert">The view's code didn't load. <button type="button" class="retry" onclick={() => location.reload()}>Reload the page</button></p>
    {/await}
  {/if}
</div>

<style>
  .views { display: flex; flex-direction: column; gap: 12px; }
  .empty { margin: 0; color: var(--muted); font-size: 13px; }
  .retry { min-height: 32px; padding: 0 10px; border-radius: 6px; border: 1px solid var(--control-line); background: transparent; color: var(--text-soft); font-size: 12px; }
</style>
