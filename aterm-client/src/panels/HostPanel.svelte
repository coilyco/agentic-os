<script lang="ts">
  import { app, selectedHost } from "../lib/app.svelte";

  const host = $derived(selectedHost());
  const silent = $derived(app.hosts.filter((candidate) => candidate.status.kind === "unreachable"));
  const running = $derived(app.sessions.filter((session) => session.state !== "failed").length);

  function retry(label: string): void {
    // The mock has no network to retry against, so the host stays silent.
    app.notice = `${label} still did not answer.`;
  }

  function addHost(event: SubmitEvent): void {
    event.preventDefault();
    app.notice = "Adding a host by address arrives with the host daemon.";
  }
</script>

<section class="panel">
  {#if !host}
    <h1>Pick a host</h1>
    <p class="lede">Sessions run on the host. This window only attaches to them. Pick one from the sidebar.</p>
    {#each silent as quiet (quiet.id)}
      <div class="alert" role="alert">
        <p><strong>{quiet.label} stopped answering.</strong> Its sessions keep running if the host is up. You just can't see them from here.</p>
        <button class="button" onclick={() => retry(quiet.label)}>Retry {quiet.label}</button>
      </div>
    {/each}
    <form class="add" onsubmit={addHost}>
      <label>Another host <input name="address" type="text" placeholder="host or address" autocomplete="off" /></label>
      <button class="button" type="submit">Add host</button>
    </form>
  {:else if host.status.kind === "unreachable"}
    <h1>{host.label} is not answering</h1>
    <div class="alert" role="alert">
      <p>Its sessions keep running if the host is up. You just can't see them from here.</p>
      <button class="button" onclick={() => retry(host.label)}>Retry</button>
    </div>
  {:else if host.status.kind === "auth-required"}
    <h1>{host.label} needs you to sign in</h1>
    <p class="lede">Signing in to a host is not designed yet, so this host stays locked for now.</p>
  {:else}
    <h1>{host.label}</h1>
    <p class="lede">{running} {running === 1 ? "seat" : "seats"} running. Pick a seat from the sidebar to open its terminal or launch it.</p>
  {/if}
  <p class="note" role="status">{app.notice}</p>
</section>

<style>
  .panel { max-width: 760px; padding: 40px clamp(16px, 4vw, 48px); display: flex; flex-direction: column; gap: 18px; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: clamp(30px, 6vw, 44px); line-height: 1.1; }
  .lede { margin: 0; color: var(--muted); font-size: 17px; }
  .alert { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; padding: 12px 16px; border: 1px solid var(--danger); border-radius: 10px; background: var(--danger-fill); }
  .alert p { margin: 0; flex: 1 1 240px; }
  .add { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
  .add label { flex: 1 1 220px; display: flex; flex-direction: column; gap: 6px; color: var(--muted); font-size: 14px; }
  .add input { min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font-family: var(--font-mono); }
  .note { margin: 0; min-height: 1.4em; color: var(--muted); font-size: 14px; }
</style>
