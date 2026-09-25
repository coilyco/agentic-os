<script lang="ts">
  import Sidebar from "./components/Sidebar.svelte";
  import { app, sessionFor } from "./lib/app.svelte";
  import HostPanel from "./panels/HostPanel.svelte";
  import LaunchPanel from "./panels/LaunchPanel.svelte";
  import SessionPanel from "./panels/SessionPanel.svelte";

  const role = $derived(app.roles.find((candidate) => candidate.slug === app.selectedRole));
  const session = $derived(role ? sessionFor(role.slug) : undefined);
  const waiting = $derived(Object.keys(app.unseen).length);
  $effect(() => {
    document.title = waiting ? `(${waiting}) aterm` : "aterm";
  });
  const labelledBy = $derived(role ? `tab-seat-${role.slug}` : app.selectedHostId ? `tab-host-${app.selectedHostId}` : undefined);
</script>

<div class="shell">
  <Sidebar />
  <main>
    <div id="main-panel" class="panel-root" role="tabpanel" aria-labelledby={labelledBy}>
    {#if role && session && session.state !== "failed"}
      <SessionPanel {role} {session} />
    {:else if role}
      <LaunchPanel {role} />
    {:else}
      <HostPanel />
    {/if}
    </div>
  </main>
</div>

<style>
  .shell { height: 100%; display: flex; }
  main { flex: 1; min-width: 0; display: flex; flex-direction: column; overflow: auto; }
  .panel-root { flex: 1; min-height: 0; display: flex; flex-direction: column; }
  @media (max-width: 720px) {
    .shell { flex-direction: column; }
  }
</style>
