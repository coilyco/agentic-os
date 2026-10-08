<script lang="ts">
  import { addHost, app, checkHost, HOSTED, removeHost, selectHost, selectedHost } from "../lib/app.svelte";
  import type { Host } from "../lib/protocol";
  import { splitSessions } from "../lib/sessions";
  import InstallCard from "../components/InstallCard.svelte";

  const host = $derived(selectedHost());
  const silent = $derived(app.hosts.filter((candidate) => candidate.status.kind === "unreachable"));
  const running = $derived(splitSessions(app.sessions).running.length);

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

  async function retry(target: Host): Promise<void> {
    await checkHost(target);
    app.notice = target.status.kind === "online" ? `${target.label} answered.` : `${target.label} still did not answer.`;
  }
</script>

<section class="panel">
  {#if !host}
    <h1>Pick a host</h1>
    <p class="lede">Sessions run on the host. This window only attaches to them. Pick one from the sidebar.</p>
    {#each silent as quiet (quiet.id)}
      <div class="alert" role="alert">
        <p><strong>{quiet.label} is not answering.</strong> Its daemon may be stopped, or running with a tailnet listener that fails TLS. On that machine, run <code>aterm doctor</code>, which says which, then retry.</p>
        <button class="button" onclick={() => retry(quiet)}>Retry {quiet.label}</button>
      </div>
    {/each}
    {#if HOSTED && !hasDaemonHost}
      <p class="lede">No hosts on this device yet. Add one by its tailnet name, once, and this device remembers it.</p>
    {/if}
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
  {:else if host.status.kind === "unreachable"}
    <h1>{host.label} is not answering</h1>
    <div class="alert" role="alert">
      <p>{host.status.reason} On that machine, <code>aterm doctor</code> says which. A stopped daemon starts with <code>aterm daemon</code>, or by launching any seat with <code>aterm</code>. Then retry.</p>
      <button class="button" onclick={() => retry(host)}>Retry</button>
    </div>
    {#if host.id.startsWith("saved:")}
      <button class="button remove" onclick={() => removeHost(host.id)}>Remove {host.label} from this device</button>
    {/if}
  {:else}
    <h1>{host.label}</h1>
    <p class="lede">{running} {running === 1 ? "seat" : "seats"} running. Pick a seat from the sidebar to open its terminal.</p>
  {/if}
  <p class="note" role="status">{app.notice}</p>
  <InstallCard />
</section>

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
