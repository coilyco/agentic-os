<script lang="ts">
  import { app, reconnecting, retryNow } from "../lib/app.svelte";
  import { secondsUntil } from "../lib/backoff";

  const link = $derived(app.link);
  const label = $derived(app.hosts.find((host) => host.id === app.attachedHostId)?.label ?? "The host");
  const shown = $derived(link.state !== "live" || app.newerBuild);
  // Five lost dials is about half a minute, long enough that launchd is not simply reviving it.
  const STILL_SILENT = 5;

  let now = $state(Date.now());
  $effect(() => {
    if (!reconnecting()) return;
    now = Date.now();
    const tick = setInterval(() => (now = Date.now()), 500);
    return () => clearInterval(tick);
  });
  const wait = $derived(link.state === "reconnecting" && link.retryAt !== null ? secondsUntil(link.retryAt, now) : null);
</script>

<!-- The status is on the page before it has words, since a status inserted with its text is not announced. -->
<div class="link" class:shown data-state={link.state}>
  <p class="line" role="status">
    {#if link.state === "reconnecting"}
      <strong>{label} is reconnecting.</strong>
      <span class="extra">It stopped answering, and your seats stay as they were until it is back.{#if link.attempt >= STILL_SILENT} Still no answer. On that machine, <code>aterm doctor</code> says why.{/if}</span>
    {:else if link.state === "restored"}
      <strong>{label} is back.</strong> <span class="extra">Your seats are live again.</span>
    {/if}
    {#if app.newerBuild}
      A newer aterm is ready. <span class="extra">Reload to use it.</span>
    {/if}
  </p>
  {#if link.state === "reconnecting"}
    <span class="count mono" aria-hidden="true">{wait === null ? "Trying now" : `Next try in ${wait} s`}</span>
    <button class="button" type="button" onclick={retryNow} disabled={wait === null}>Retry now</button>
  {/if}
  {#if app.newerBuild}
    <button class="button primary" type="button" onclick={() => location.reload()}>Reload</button>
  {/if}
</div>

<style>
  .link { flex: none; display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; }
  .link.shown { padding: 10px clamp(16px, 4vw, 48px); border-bottom: 1px solid var(--line); background: var(--surface); }
  .link[data-state="reconnecting"] { border-bottom-color: var(--warn); background: var(--warn-fill); }
  .line { margin: 0; flex: 1 1 260px; font-size: 15px; }
  .link[data-state="reconnecting"] strong { color: var(--warn-text); }
  code { font-family: var(--font-mono); font-size: 0.92em; color: var(--text-soft); }
  .count { font-size: 13px; color: var(--muted); }
  .button { min-height: 44px; }
  .button:disabled { opacity: 0.5; cursor: default; }
  /* A phone has no height to spare above the sheet, so the bar is one row. The words stay announced. */
  @media (max-width: 720px) {
    .link.shown { padding: 8px 16px; }
    .line { flex-basis: 150px; }
    .extra { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .count { display: none; }
  }
</style>
