<script lang="ts">
  import Sidebar from "./components/Sidebar.svelte";
  import { onMount } from "svelte";
  import { app, jumpToWaiting, sessionFor, waitingSeats } from "./lib/app.svelte";
  import HostPanel from "./panels/HostPanel.svelte";
  import LaunchPanel from "./panels/LaunchPanel.svelte";
  import SessionPanel from "./panels/SessionPanel.svelte";

  const role = $derived(app.roles.find((candidate) => candidate.slug === app.selectedRole));
  const session = $derived(role ? sessionFor(role.slug) : undefined);
  const waiting = $derived(waitingSeats());
  // The title is what the Windows alt-tab switcher shows, so it names who is waiting.
  $effect(() => {
    const first = waiting[0];
    document.title = first ? `(${waiting.length}) ${first.identity} ${first.kind === "asking" ? "asking" : "done"} // aterm` : "aterm";
    const badge = navigator as Navigator & { setAppBadge?: (count: number) => Promise<void>; clearAppBadge?: () => Promise<void> };
    if (waiting.length) void badge.setAppBadge?.(waiting.length).catch(() => {});
    else void badge.clearAppBadge?.().catch(() => {});
  });

  function arrive(): void {
    if (document.visibilityState !== "visible" || !app.attachedHostId) return;
    const current = app.selectedRole ? sessionFor(app.selectedRole) : undefined;
    const alreadyThere = current && waiting.some((each) => each.sessionId === current.id);
    app.triage = true;
    if (!alreadyThere) jumpToWaiting();
  }

  onMount(() => {
    const leave = () => (app.triage = false);
    window.addEventListener("focus", arrive);
    window.addEventListener("blur", leave);
    document.addEventListener("visibilitychange", arrive);
    return () => {
      window.removeEventListener("focus", arrive);
      window.removeEventListener("blur", leave);
      document.removeEventListener("visibilitychange", arrive);
    };
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
