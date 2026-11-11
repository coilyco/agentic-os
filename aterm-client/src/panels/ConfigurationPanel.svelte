<script lang="ts">
  import Creature from "../components/Creature.svelte";
  import { app, closeConfiguration, reconnecting, selectedHost, showHost } from "../lib/app.svelte";
  import { rolesState } from "../lib/configuration";

  const host = $derived(selectedHost());
  const view = $derived(
    rolesState(!!app.attachedHostId, !!host && host.id === app.attachedHostId && host.status.kind === "online" && !reconnecting(), app.roles),
  );
</script>

<section class="configuration" aria-labelledby="configuration-title">
  <header>
    <button type="button" class="back" onclick={closeConfiguration}>Back</button>
    <h1 id="configuration-title">Configuration</h1>
  </header>
  <p class="lede">What each role is called on this host. You can read it here. Editing arrives once this host can store configuration.</p>

  <section class="group" aria-labelledby="roles-label">
    <h2 id="roles-label">Roles</h2>
    {#if view.kind === "no-host"}
      <div class="empty">
        <p><b>No host is connected.</b></p>
        <p class="hint">Roles come from a host. Pick one to see its roles.</p>
        <button type="button" class="action" onclick={showHost}>Pick a host</button>
      </div>
    {:else if view.kind === "no-roles"}
      <div class="empty">
        <p><b>This host sent no roles.</b></p>
        <p class="hint">A host that is still starting sends its roles a moment after it connects. Try again in a few seconds.</p>
      </div>
    {:else}
      {#if view.stale}<p class="stale" role="status">This host is not answering. These roles are from the last time it did.</p>{/if}
      <ul>
        {#each view.rows as row (row.slug)}
          <li>
            <Creature role={row.slug} color={row.color} size={40} />
            <div class="who">
              <b>{row.name}</b>
              <span>{row.seat}</span>
              {#if row.pool.length}<small>Later seats: {row.pool.join(", ")}</small>{/if}
              {#if row.clients.length}<small>Runs on: {row.clients.join(", ")}</small>{/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</section>

<style>
  .configuration { max-width: 640px; padding: 24px clamp(16px, 4vw, 48px) 40px; display: flex; flex-direction: column; gap: 22px; }
  header { display: flex; align-items: center; gap: 14px; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: clamp(26px, 6vw, 36px); line-height: 1.1; }
  h2 { margin: 0 0 6px; font: 600 12px var(--font-body); letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); }
  .lede { margin: 0; color: var(--muted); font-size: 15px; }
  .back, .action { min-height: 44px; min-width: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .back:focus-visible, .action:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .group { display: flex; flex-direction: column; gap: 8px; }
  .hint { margin: 0; color: var(--muted); font-size: 13px; }
  .stale { margin: 0; padding: 8px 12px; border-radius: 8px; border: 1px solid var(--line); background: var(--surface); color: var(--muted); font-size: 13px; }
  .empty { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .empty p { margin: 0; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  li { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .who { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .who span { font-size: 14px; }
  small { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
</style>
