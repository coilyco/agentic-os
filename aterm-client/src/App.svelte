<script lang="ts">
  import Attention from "./components/Attention.svelte";
  import DemoBanner from "./components/DemoBanner.svelte";
  import FeedbackSheet from "./components/FeedbackSheet.svelte";
  import LinkBanner from "./components/LinkBanner.svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import SeatToast from "./components/SeatToast.svelte";
  import ShareSheet from "./components/ShareSheet.svelte";
  import OpenedPageCard from "./components/OpenedPageCard.svelte";
  import PageReader from "./components/PageReader.svelte";
  import { app, inDemo, overviewShown, sessionById, waitingSeats } from "./lib/app.svelte";
  import { roleFor } from "./lib/sessions";
  import HostPanel from "./panels/HostPanel.svelte";
  import LaunchPanel from "./panels/LaunchPanel.svelte";
  import ConfigurationPanel from "./panels/ConfigurationPanel.svelte";
  import SettingsPanel from "./panels/SettingsPanel.svelte";
  import SessionPanel from "./panels/SessionPanel.svelte";

  const session = $derived(sessionById(app.selectedSession));
  const role = $derived(session ? roleFor(session, app.roles) : app.roles.find((candidate) => candidate.slug === app.selectedRole));
  const waiting = $derived(waitingSeats());
  // The title is what the Windows alt-tab switcher shows, so it names who is waiting.
  $effect(() => {
    const first = waiting[0];
    const title = first ? `(${waiting.length}) ${first.identity} ${first.kind === "asking" ? "asking" : "left a page"} // Treehouse` : "Treehouse";
    document.title = inDemo() ? `DEMO // ${title}` : title;
    const badge = navigator as Navigator & { setAppBadge?: (count: number) => Promise<void>; clearAppBadge?: () => Promise<void> };
    if (waiting.length) void badge.setAppBadge?.(waiting.length).catch(() => {});
    else void badge.clearAppBadge?.().catch(() => {});
  });

  const labelledBy = $derived(
    session ? `tab-session-${session.id}` : role ? `tab-role-${role.slug}` : app.selectedHostId ? `tab-host-${app.selectedHostId}` : undefined,
  );
</script>

<Attention />
<SeatToast />
<OpenedPageCard />
<PageReader />
<FeedbackSheet />
<ShareSheet />
<!-- The page behind the reader is inert, so Tab out of the sandboxed frame has nowhere to go but back into the reader. -->
<!-- Above the shell, not inside it, so the seat menu that drops over the page cannot cover it. -->
<div class="frame" class:demo={inDemo()}>
<DemoBanner />
<div class="shell" class:overview={overviewShown()} inert={app.pageOpen !== null}>
  <Sidebar />
  <main>
    <LinkBanner />
    <div id="main-panel" class="panel-root" class:clear-bottom={app.settingsOpen || app.configurationOpen || !(role && session && session.state !== "failed")} role="tabpanel" aria-labelledby={labelledBy}>
    {#if app.settingsOpen}
      <SettingsPanel />
    {:else if app.configurationOpen}
      <ConfigurationPanel />
    {:else if role && session && session.state !== "failed"}
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
</div>

<style>
  /* viewport-fit=cover draws under the status bar, the cutout and the Android nav buttons, so the shell keeps clear of them (COI-2817). */
  .frame { height: 100%; display: flex; flex-direction: column; }
  .shell { flex: 1; min-height: 0; display: flex; padding: env(safe-area-inset-top) env(safe-area-inset-right) 0 env(safe-area-inset-left); }
  /* The banner takes the status-bar inset itself while the demo is up. */
  .frame.demo .shell { padding-top: 0; }
  main { flex: 1; min-width: 0; display: flex; flex-direction: column; overflow: auto; }
  .panel-root { flex: 1; min-height: 0; display: flex; flex-direction: column; }
  /* A seat's panel clears the bottom itself, through the tab strip it reserves. The list and launch screens have no strip. */
  .panel-root.clear-bottom { padding-bottom: env(safe-area-inset-bottom); }
  @media (max-width: 720px) {
    .shell { flex-direction: column; }
    /* The seat overview is where the app rests, so a phone shows it first and the hosts, roles and switches under it (COI-3518). */
    .shell.overview { overflow-y: auto; }
    .shell.overview main { order: -1; flex: none; overflow: visible; }
    /* A phone on its side keeps the row, with the sidebar as a rail. */
    .shell:has(:global(.sidebar[data-mode="reading"])) { flex-direction: row; }
  }
</style>
