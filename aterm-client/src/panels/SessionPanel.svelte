<script lang="ts">
  import Creature from "../components/Creature.svelte";
  import MessagesPanel from "../components/MessagesPanel.svelte";
  import Terminal from "../components/Terminal.svelte";
  import { app, colorOf } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";
  import type { Role } from "../lib/roster";

  let { role, session }: { role: Role; session: Session } = $props();
  const messages = $derived(app.messages.filter((m) => m.from.role === role.slug || m.to.role === role.slug));
</script>

<section class="session" style:--accent={role.color}>
  <header>
    <Creature role={role.slug} color={role.color} size={40} />
    <h1>{session.identity}</h1>
    <span class="role">{role.displayName} // {session.seat}</span>
    <span class="state mono">{session.state}</span>
  </header>
  <div class="body">
    {#if app.connection}
      {#key session.id}
        <Terminal connection={app.connection} sessionId={session.id} label={session.identity} accent={role.color} {messages} {colorOf} />
      {/key}
    {/if}
    <MessagesPanel {messages} selfRole={role.slug} {colorOf} />
  </div>
</section>

<style>
  .session { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
  header { min-height: 64px; padding: 8px 24px; display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; border-bottom: 3px solid var(--accent); background: color-mix(in srgb, var(--accent) 6%, var(--ground)); }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 22px; }
  .role { font-size: 14px; color: color-mix(in srgb, var(--accent) 55%, white); }
  .state { margin-left: auto; font-size: 12px; padding: 4px 10px; border-radius: 999px; border: 1px solid var(--accent); }
  .body { flex: 1; display: grid; grid-template-columns: minmax(0, 1fr) 360px; min-height: 0; }
  @media (max-width: 1000px) {
    .body { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(320px, 1fr) auto; }
    .body :global(.panel) { border-left: none; border-top: 1px solid var(--line); max-height: 40vh; }
  }
</style>
