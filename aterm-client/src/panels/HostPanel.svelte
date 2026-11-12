<script lang="ts">
  import { addHost, app, checkHost, overviewShown, placeNow, reconnecting, refreshMachines, removeHost, saveFoundHost, selectHost, selectedHost } from "../lib/app.svelte";
  import AccountPanel from "../components/AccountPanel.svelte";
  import DemoEntry from "../components/DemoEntry.svelte";
  import Unlock from "../components/Unlock.svelte";
  import { CAN_PASSKEY, EXTERNAL } from "../lib/build";
  import type { Host } from "../lib/protocol";
  import { typingNotice } from "../lib/typing";
  import { splitSessions } from "../lib/sessions";
  import Home from "../components/Home.svelte";
  import Overview from "../components/Overview.svelte";
  import InstallCard from "../components/InstallCard.svelte";
  import ConnectionCheck from "../components/ConnectionCheck.svelte";

  const host = $derived(selectedHost());
  // A machine the broker lists as offline is not a fault, and has its own words when picked.
  const silent = $derived(app.hosts.filter((candidate) => candidate.status.kind === "unreachable" && !candidate.brokered));
  const running = $derived(splitSessions(app.sessions).running.length);
  // A healthy attached host with no seat open rests on the seat overview, or on the logo alone when asked. A fault or a lock shows the full page.
  const home = $derived(overviewShown());

  let draft = $state("");
  let draftError = $state("");
  const hasDaemonHost = $derived(app.hosts.some((candidate) => candidate.kind === "daemon"));

  function add(event: SubmitEvent): void {
    event.preventDefault();
    try {
      const added = addHost(draft);
      draft = "";
      draftError = "";
      app.notice = `Added ${added.label}. Checking it now.`;
      selectHost(added);
    } catch (error) {
      draftError = error instanceof Error ? error.message : String(error);
    }
  }

  function keep(target: Host): void {
    saveFoundHost(target);
    app.notice = `${target.label} is saved to this device.`;
  }

  async function retry(target: Host): Promise<void> {
    await checkHost(target);
    app.notice = target.status.kind === "online" ? `${target.label} answered.` : `${target.label} still did not answer.`;
  }
</script>

{#if home}
  {#if app.logoHome}<Home />{:else}<Overview />{/if}
{:else}
<section class="panel">
  {#if !host}
    <h1>Pick a host</h1>
    <p class="lede">Sessions run on the host. This window only attaches to them. Pick one from the host list.</p>
    <AccountPanel />
    {#each silent as quiet (quiet.id)}
      <div class="alert" role="alert">
        {#if quiet.answered}
          <p><strong>{quiet.label} stopped answering.</strong> It answered earlier on this page, so it is probably restarting. This page asks again on its own, and Retry asks now.</p>
        {:else}
          <p><strong>{quiet.label} is not answering.</strong> Pick it to run the connection check, which shows how far the connection got.</p>
        {/if}
        <button class="button" onclick={() => retry(quiet)}>Retry {quiet.label}</button>
      </div>
    {/each}
    {#if EXTERNAL && !hasDaemonHost}
      <p class="lede">No hosts on this device yet. Add one by its tailnet name, once, and this device remembers it.</p>
    {/if}
    <DemoEntry />
    <form class="add" onsubmit={add}>
      <label for="add-host">Add a host by its tailnet name</label>
      <div class="row">
        <input id="add-host" bind:value={draft} placeholder="machine.your-tailnet.ts.net" autocomplete="off" spellcheck="false" aria-invalid={draftError ? "true" : undefined} aria-describedby="add-host-hint" />
        <button class="button primary" type="submit" disabled={!draft.trim()}>Add host</button>
      </div>
      <p id="add-host-hint" class="hint" class:error={draftError}>
        {draftError || "The name ends in .ts.net. Running tailscale status on that machine shows it. Only your own devices get in."}
      </p>
    </form>
  {:else if host.status.kind === "checking"}
    <h1>Checking {host.label}</h1>
    <p class="lede" role="status">Asking the daemon at <code>{host.address}</code> whether it is up.</p>
  {:else if host.brokered && host.status.kind === "unreachable"}
    <h1>{host.label} is offline</h1>
    <div class="alert" role="alert">
      <p>{host.status.reason} It stays in your list, and it opens once it connects. This list checks again every half minute.</p>
      <button class="button" onclick={() => refreshMachines()}>Check again</button>
    </div>
  {:else if host.status.kind === "unreachable"}
    <h1>{host.label} {host.answered ? "stopped answering" : "is not answering"}</h1>
    <div class="alert" role="alert">
      {#if host.answered}
        <p>{host.status.reason} It answered earlier on this page, so it is probably restarting. This page asks again on its own, and Retry asks now.</p>
      {:else if host.status.layer && host.status.layer !== "network"}
        <p>{host.status.reason}</p>
      {:else}
        <p>Nothing answered. The connection check below shows how far the connection got, and one button sends the report to prod-director. This page keeps asking for an hour, and asks again when you come back to it.</p>
      {/if}
      <button class="button" onclick={() => retry(host)}>Retry</button>
    </div>
    {#if host.id.startsWith("saved:")}
      <button class="button remove" onclick={() => removeHost(host.id)}>Remove {host.label} from this device</button>
    {:else if host.found}
      <button class="button remove" onclick={() => keep(host)}>Save {host.label} to this device</button>
    {/if}
  {:else}
    <h1>{host.label}</h1>
    {#if reconnecting() && host.id === app.attachedHostId}
      {#if running === 0}
        <p class="lede">{app.link.state === "resting" ? "Not answering." : "Reconnecting."} Nothing was running when it stopped answering.</p>
      {:else}
        <p class="lede">{app.link.state === "resting" ? "Not answering." : "Reconnecting."} {running} {running === 1 ? "seat was" : "seats were"} running when it stopped answering, and they show as they were.</p>
      {/if}
    {:else}
      <p class="lede">{running} {running === 1 ? "seat" : "seats"} running. Pick a seat from the list to open its terminal.</p>
    {/if}
    {#if host.found}
      <p class="lede">A daemon on your tailnet listed this host. It stays only while a daemon lists it, unless you save it.</p>
      <button class="button remove" onclick={() => keep(host)}>Save {host.label} to this device</button>
    {/if}
  {/if}
  {#if host && host.kind === "daemon" && host.status.kind !== "checking"}
    <!-- Keyed so a host that goes silent again checks again, and one that is only being looked at waits to be asked. -->
    {#key `${host.id} ${host.status.kind} ${reconnecting()}`}
      <ConnectionCheck {host} auto={host.status.kind === "unreachable" || (reconnecting() && host.id === app.attachedHostId)} />
    {/key}
  {/if}
  {#if host?.id === app.attachedHostId}
    <!-- The passkey section is here whatever the lock says, so enrollment never depends on a lock that failed to show. -->
    {#if !app.typing.allowed}
      <p class="lede" role="status"><strong>Read only.</strong> {typingNotice(app.typing, placeNow())}</p>
    {:else if (CAN_PASSKEY && app.passkeyAvailable) || app.deviceKeyAvailable || app.brokered}
      <p class="lede" role="status">Typing is on for this device.</p>
    {/if}
    <Unlock always />
  {/if}
  <p class="note" role="status">{app.notice}</p>
  <InstallCard />
</section>
{/if}

<style>
  .panel { max-width: 760px; padding: 40px clamp(16px, 4vw, 48px); display: flex; flex-direction: column; gap: 18px; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: clamp(30px, 6vw, 44px); line-height: 1.1; }
  .lede { margin: 0; color: var(--muted); font-size: 17px; }
  code { font-family: var(--font-mono); font-size: 0.92em; color: var(--text-soft); }
  .alert { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; padding: 12px 16px; border: 1px solid var(--danger); border-radius: 10px; background: var(--danger-fill); }
  .alert p { margin: 0; flex: 1 1 240px; }
  .note { margin: 0; min-height: 1.4em; color: var(--muted); font-size: 14px; }
  .add { display: flex; flex-direction: column; gap: 8px; }
  .add label { font-weight: 600; }
  .row { display: flex; gap: 10px; flex-wrap: wrap; }
  .row input { flex: 1 1 260px; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font-family: var(--font-mono); }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
  .hint.error { color: var(--danger-text); }
  .remove { align-self: flex-start; }
</style>
