<script lang="ts">
  import Creature from "./Creature.svelte";
  import { app, asksFor, selectHost, selectRole, sessionFor } from "../lib/app.svelte";
  import type { Host } from "../lib/protocol";
  import { tablistKeys } from "../lib/tabs";

  const seats = $derived(
    [...app.roles].sort((a, b) => Number(Boolean(sessionFor(b.slug))) - Number(Boolean(sessionFor(a.slug)))),
  );

  function hostDetail(host: Host): string {
    if (host.status.kind === "online") return `${host.status.sessionCount} running`;
    if (host.status.kind === "unreachable") return "no answer";
    return "checking";
  }

  function seatDetail(slug: string): string {
    const session = sessionFor(slug);
    if (!session) return "not running";
    if (session.state === "failed") return "launch failed";
    if (asksFor(session.id).length) return `${session.seat} // asking you`;
    if (app.unseen[session.id]) return `${session.seat} // done, your turn`;
    if (session.drafting) return `${session.seat} // you're typing`;
    if (session.pending) return `${session.seat} // ${session.pending} ${session.pending === 1 ? "message" : "messages"} waiting`;
    return `${session.seat} // ${session.state}`;
  }

  function activity(slug: string): string {
    const session = sessionFor(slug);
    if (!session) return "none";
    if (session.state === "failed") return "failed";
    if (asksFor(session.id).length) return "asking";
    if (app.unseen[session.id]) return "unseen";
    return session.state;
  }
</script>

<nav class="sidebar" aria-label="Hosts and seats">
  <div class="mark">ATERM</div>

  <h2 id="hosts-label">Hosts</h2>
  <div role="tablist" aria-labelledby="hosts-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
    {#each app.hosts as host (host.id)}
      {@const selected = app.selectedHostId === host.id && !app.selectedRole}
      <button
        role="tab"
        id={`tab-host-${host.id}`}
        aria-selected={selected}
        aria-controls="main-panel"
        tabindex={selected || (!app.selectedHostId && host === app.hosts[0]) ? 0 : -1}
        class="tab"
        data-kind={host.status.kind}
        onclick={() => selectHost(host)}
      >
        <span class="dot" aria-hidden="true"></span>
        <span class="text"><span class="name">{host.label}</span><span class="detail">{hostDetail(host)}</span></span>
      </button>
    {/each}
  </div>

  {#if app.attachedHostId}
    <h2 id="seats-label">Seats</h2>
    {#if seats.length === 0}
      <p class="waiting">Waiting for the roster.</p>
    {:else}
      <div role="tablist" aria-labelledby="seats-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
        {#each seats as role (role.slug)}
          {@const session = sessionFor(role.slug)}
          {@const selected = app.selectedRole === role.slug}
          <button
            role="tab"
            id={`tab-seat-${role.slug}`}
            aria-selected={selected}
            aria-controls="main-panel"
            tabindex={selected || (!app.selectedRole && role === seats[0]) ? 0 : -1}
            class="tab seat"
            data-state={session?.state ?? "none"}
            data-activity={activity(role.slug)}
            style:--accent={role.color}
            onclick={() => selectRole(role.slug)}
          >
            <span class="avatar"><Creature role={role.slug} color={role.color} size={36} /></span>
            <span class="text"><span class="name">{role.identity}</span><span class="detail">{role.displayName} // {seatDetail(role.slug)}</span></span>
          </button>
        {/each}
      </div>
    {/if}
  {/if}
</nav>

<style>
  .sidebar { width: 280px; flex: none; border-right: 1px solid var(--line); padding: 16px 12px; display: flex; flex-direction: column; gap: 6px; overflow-y: auto; }
  .mark { font-family: var(--font-display); font-weight: 700; font-size: 20px; letter-spacing: 0.08em; color: var(--brand); padding: 4px 10px 12px; }
  h2 { margin: 14px 10px 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  [role="tablist"] { display: flex; flex-direction: column; gap: 2px; }
  .tab { width: 100%; min-height: 48px; display: flex; align-items: center; gap: 12px; padding: 8px 10px; border: none; border-radius: 8px; background: transparent; color: var(--text); text-align: left; }
  .tab:hover { background: #171a21; }
  .tab[aria-selected="true"] { background: #1d1729; box-shadow: inset 0 0 0 1px #3a3350; }
  .seat[aria-selected="true"] { background: color-mix(in srgb, var(--accent) 14%, var(--ground)); box-shadow: inset 0 0 0 1px var(--accent); }
  .dot { width: 10px; height: 10px; border-radius: 5px; background: var(--ok); flex: none; }
  [data-kind="unreachable"] .dot { background: none; border: 2px solid var(--danger); }
  [data-kind="checking"] .dot { background: none; border: 2px solid var(--muted); }
  .seat[data-state="none"] :global(.creature) { opacity: 0.5; filter: saturate(0.4); }
  .avatar { position: relative; display: inline-flex; flex: none; border-radius: 26%; }
  [data-activity="working"] .avatar::before {
    content: ""; position: absolute; inset: -3px; border-radius: 30%;
    background: conic-gradient(from var(--spin, 0deg), var(--accent), transparent 40%, transparent 60%, var(--accent));
    animation: spin 1.4s linear infinite; z-index: 0;
  }
  [data-activity="working"] .avatar :global(.creature) { position: relative; z-index: 1; }
  [data-activity="unseen"] .avatar::after {
    content: ""; position: absolute; top: -4px; right: -4px; width: 12px; height: 12px;
    border-radius: 6px; background: var(--brand); box-shadow: 0 0 0 2px var(--ground);
  }
  [data-activity="unseen"] .name, [data-activity="asking"] .name { color: var(--brand); }
  [data-activity="asking"] .avatar::after {
    content: "?"; position: absolute; top: -6px; right: -6px; width: 18px; height: 18px;
    border-radius: 9px; background: var(--brand); color: var(--brand-ink); box-shadow: 0 0 0 2px var(--ground);
    font: 700 12px/18px var(--font-body); text-align: center;
  }
  @property --spin { syntax: "<angle>"; inherits: false; initial-value: 0deg; }
  @keyframes spin { to { --spin: 360deg; } }
  @media (prefers-reduced-motion: reduce) {
    [data-activity="working"] .avatar::before { animation: none; background: var(--accent); opacity: 0.6; }
  }
  .seat[data-state="failed"] :global(.creature) { box-shadow: inset 0 0 0 2px var(--danger); }
  .text { display: flex; flex-direction: column; min-width: 0; }
  .name { font-weight: 600; font-size: 15px; }
  .detail { font-size: 12px; color: var(--muted); }
  [data-kind="unreachable"] .detail, [data-state="failed"] .detail { color: #ff9aa5; }
  .waiting { margin: 0 10px; color: var(--muted); font-size: 14px; }

  @media (max-width: 720px) {
    .sidebar { width: auto; border-right: none; border-bottom: 1px solid var(--line); padding: 8px 0; gap: 4px; overflow: visible; }
    .mark, h2, .waiting { display: none; }
    [role="tablist"] { flex-direction: row; overflow-x: auto; padding: 0 16px; scrollbar-width: none; }
    .tab { width: auto; flex: none; }
    .detail { display: none; }
  }
</style>
