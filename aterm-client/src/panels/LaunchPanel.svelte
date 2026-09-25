<script lang="ts">
  import Creature from "../components/Creature.svelte";
  import { app, sessionFor } from "../lib/app.svelte";
  import type { Role } from "../lib/roster";

  let { role }: { role: Role } = $props();
  const session = $derived(sessionFor(role.slug));
</script>

<section class="panel" style:--accent={role.color}>
  <div class="title">
    <Creature role={role.slug} color={role.color} size={96} />
    <div>
      <h1>{role.identity}</h1>
      <p class="role">{role.displayName}</p>
    </div>
  </div>
  <p class="purpose">{role.purpose}</p>
  {#if session?.state === "failed"}
    <p class="failure" role="alert"><strong>The last launch failed.</strong> {session.failure}</p>
  {/if}
  {#if app.connection?.canLaunch}
    <div class="seats" role="group" aria-label="Launch on a harness">
      {#each role.seats as seat, index (seat.key)}
        <button class="button" class:primary={index === 0 && session?.state !== "failed"} onclick={() => app.connection?.launch(role.slug, seat.key)}>
          Launch on {seat.key}
        </button>
      {/each}
    </div>
  {:else}
    <p class="howto">Not running. Launching from this window is not wired to the daemon yet, so start it on the host:</p>
    <pre class="mono">aterm {role.slug}</pre>
  {/if}
</section>

<style>
  .panel { max-width: 760px; padding: 40px clamp(16px, 4vw, 48px); display: flex; flex-direction: column; gap: 18px; }
  .title { display: flex; gap: 14px; align-items: center; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: clamp(30px, 6vw, 40px); }
  .role { margin: 0; color: color-mix(in srgb, var(--accent) 55%, white); }
  .purpose { margin: 0; color: var(--text-soft); font-size: 17px; }
  .failure { margin: 0; padding: 12px 16px; border: 1px solid var(--danger); border-radius: 10px; background: var(--danger-fill); color: var(--danger-text); }
  .seats { display: flex; gap: 10px; flex-wrap: wrap; }
  .howto { margin: 0; color: var(--muted); }
  pre { margin: 0; padding: 12px 16px; border-radius: 8px; background: var(--terminal); border: 1px solid var(--line); color: var(--text); overflow-x: auto; }
</style>
