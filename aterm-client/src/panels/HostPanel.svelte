<script lang="ts">
  import { app, checkHost, selectedHost } from "../lib/app.svelte";
  import type { Host } from "../lib/protocol";

  const host = $derived(selectedHost());
  const silent = $derived(app.hosts.filter((candidate) => candidate.status.kind === "unreachable"));
  const running = $derived(app.sessions.filter((session) => session.state !== "failed").length);

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
        <p><strong>{quiet.label} is not answering.</strong> Start its daemon with <code>aterm daemon</code>, then retry.</p>
        <button class="button" onclick={() => retry(quiet)}>Retry {quiet.label}</button>
      </div>
    {/each}
    <p class="lede small">Hosts beyond this Mac arrive once the daemon listens past loopback.</p>
  {:else if host.status.kind === "checking"}
    <h1>Checking {host.label}</h1>
    <p class="lede" role="status">Asking the daemon at <code>{host.address}</code> whether it is up.</p>
  {:else if host.status.kind === "unreachable"}
    <h1>{host.label} is not answering</h1>
    <div class="alert" role="alert">
      <p>{host.status.reason} Start it with <code>aterm daemon</code>, or launch any seat with <code>aterm</code>, then retry.</p>
      <button class="button" onclick={() => retry(host)}>Retry</button>
    </div>
  {:else}
    <h1>{host.label}</h1>
    <p class="lede">{running} {running === 1 ? "seat" : "seats"} running. Pick a seat from the sidebar to open its terminal.</p>
  {/if}
  <p class="note" role="status">{app.notice}</p>
</section>

<style>
  .panel { max-width: 760px; padding: 40px clamp(16px, 4vw, 48px); display: flex; flex-direction: column; gap: 18px; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: clamp(30px, 6vw, 44px); line-height: 1.1; }
  .lede { margin: 0; color: var(--muted); font-size: 17px; }
  .small { font-size: 14px; }
  code { font-family: var(--font-mono); font-size: 0.92em; color: var(--text-soft); }
  .alert { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; padding: 12px 16px; border: 1px solid var(--danger); border-radius: 10px; background: var(--danger-fill); }
  .alert p { margin: 0; flex: 1 1 240px; }
  .note { margin: 0; min-height: 1.4em; color: var(--muted); font-size: 14px; }
</style>
