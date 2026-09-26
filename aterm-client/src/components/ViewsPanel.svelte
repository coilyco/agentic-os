<script lang="ts">
  import AppView from "./AppView.svelte";
  import { app, viewsFor } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";

  let { session }: { session: Session } = $props();
  const views = $derived(viewsFor(session.id));
</script>

<div class="views">
  {#if !app.connection?.views}
    <p class="empty">This host doesn't forward MCP App views yet. Once its daemon does, a tool that returns a view opens it here, next to the terminal.</p>
  {:else if views.length === 0}
    <p class="empty">No tool has returned a view in this session yet. When one does, it opens here, and you can use it the way {session.identity} does.</p>
  {:else}
    {#each views as view (view.id)}
      <AppView {view} />
    {/each}
  {/if}
</div>

<style>
  .views { display: flex; flex-direction: column; gap: 12px; }
  .empty { margin: 0; color: var(--muted); font-size: 13px; }
</style>
