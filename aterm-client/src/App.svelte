<script lang="ts">
  import Attention from "./components/Attention.svelte";
  import LinkBanner from "./components/LinkBanner.svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import { onMount } from "svelte";
  import { app, jumpToWaiting, sessionById, waitingSeats } from "./lib/app.svelte";
  import { roleFor } from "./lib/sessions";
  import HostPanel from "./panels/HostPanel.svelte";
  import LaunchPanel from "./panels/LaunchPanel.svelte";
  import SessionPanel from "./panels/SessionPanel.svelte";

  const session = $derived(sessionById(app.selectedSession));
  const role = $derived(session ? roleFor(session, app.roles) : app.roles.find((candidate) => candidate.slug === app.selectedRole));
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
    const current = sessionById(app.selectedSession);
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
  const labelledBy = $derived(
    session ? `tab-session-${session.id}` : role ? `tab-role-${role.slug}` : app.selectedHostId ? `tab-host-${app.selectedHostId}` : undefined,
  );
</script>

<Attention />
<div class="shell">
  <Sidebar />
  <main>
    <LinkBanner />
    <div id="main-panel" class="panel-root" role="tabpanel" aria-labelledby={labelledBy}>
    {#if role && session && session.state !== "failed"}
      {#key session.id}
        <SessionPanel {role} {session} />
      {/key}
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
