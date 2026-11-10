<script lang="ts">
  import { app, setRoleShown } from "../lib/app.svelte";

  // Seats by role (COI-3346). On means shown. A role stays listed while hidden so it can come back.
  const rows = $derived(
    app.roles
      .map((role) => ({ role, seats: app.sessions.filter((seat) => seat.role === role.slug && seat.state !== "failed").length }))
      .filter(({ role, seats }) => seats > 0 || app.hiddenRoles.includes(role.slug)),
  );
</script>

{#if rows.length}
  <section class="by-role" aria-labelledby="by-role-label">
    <h2 id="by-role-label">Seats by role</h2>
    <div class="switches">
      {#each rows as { role, seats } (role.slug)}
        <label class="switch">
          <input type="checkbox" role="switch" checked={!app.hiddenRoles.includes(role.slug)} onchange={(event) => setRoleShown(role.slug, event.currentTarget.checked)} />
          <span class="track" aria-hidden="true"></span>
          <span class="name">{role.displayName} <span class="count">{seats} {seats === 1 ? "seat" : "seats"}</span></span>
        </label>
      {/each}
    </div>
    <p class="hint">A role switched off leaves the list. A seat that needs you, one you pinned, or the one open stays.</p>
  </section>
{/if}

<style>
  .by-role { padding-top: 12px; border-top: 1px solid var(--line); }
  h2 { margin: 6px 10px 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  .switches { display: flex; flex-direction: column; }
  .switch { position: relative; min-height: 44px; display: flex; align-items: center; gap: 12px; padding: 0 10px; border-radius: 8px; cursor: pointer; }
  .switch:hover { background: #171a21; }
  input { position: absolute; opacity: 0; inset: 0; width: 100%; height: 100%; margin: 0; cursor: pointer; }
  .track { position: relative; flex: none; width: 36px; height: 20px; border-radius: 10px; border: 2px solid var(--control-line); background: transparent; }
  .track::after { content: ""; position: absolute; top: 2px; left: 2px; width: 12px; height: 12px; border-radius: 6px; background: var(--muted); transition: transform 120ms; }
  input:checked + .track { background: var(--brand); border-color: var(--brand); }
  input:checked + .track::after { transform: translateX(16px); background: var(--brand-ink); }
  input:focus-visible + .track { outline: 2px solid var(--brand); outline-offset: 3px; }
  .name { font-size: 14px; color: var(--text-soft); }
  .count { color: var(--muted); font-size: 12px; }
  .hint { margin: 0 10px 4px; font-size: 12px; color: var(--muted); }
</style>
