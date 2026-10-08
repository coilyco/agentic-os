<script lang="ts">
  import AttentionSwitches from "./AttentionSwitches.svelte";
  import Creature from "./Creature.svelte";
  import { app, asksFor, glowSeats, selectHost, selectRole, selectSession } from "../lib/app.svelte";
  import type { Host, Session } from "../lib/protocol";
  import { hostRunning, orderSessions, roleFor, sessionCode, sessionLabel, splitSessions } from "../lib/sessions";
  import { tablistKeys } from "../lib/tabs";

  const live = $derived(splitSessions(orderSessions(app.sessions, app.roles)));
  const onScreen = $derived(Boolean(app.selectedSession || app.selectedRole));
  const glowing = $derived(new Set(app.alerts.visual ? glowSeats().map((seat) => seat.sessionId) : []));

  function hostDetail(host: Host): string {
    const running = hostRunning(host, app.attachedHostId, app.sessions);
    if (running !== null) return host.found ? `${running} running, found on tailnet` : `${running} running`;
    if (host.status.kind === "unreachable") return "no answer";
    return "checking";
  }

  function stateText(session: Session): string {
    if (session.state === "failed") return "launch failed";
    if (asksFor(session.id).length) return "asking you";
    if (app.unseen[session.id]) return "done, your turn";
    if (session.drafting) return "you're typing";
    if (session.pending) return `${session.pending} ${session.pending === 1 ? "message" : "messages"} waiting`;
    return session.state;
  }

  function activity(session: Session): string {
    if (session.state === "failed") return "failed";
    if (asksFor(session.id).length) return "asking";
    if (app.unseen[session.id]) return "unseen";
    return session.state;
  }

  function harnesses(seats: { key: string }[]): string {
    return seats.length ? `on ${seats.map((seat) => seat.key).join(", ")}` : "no seat on this host";
  }
</script>

<!-- One tab stop per group: the selected tab, else the group's first. -->
{#snippet seatTab(session: Session, group: Session[])}
  {@const role = roleFor(session, app.roles)}
  {@const selected = app.selectedSession === session.id}
  {@const code = sessionCode(session)}
  {@const stop = group.some((each) => each.id === app.selectedSession) ? selected : session === group[0]}
  <button
    role="tab"
    id={`tab-session-${session.id}`}
    aria-selected={selected}
    aria-controls="main-panel"
    aria-label={sessionLabel(session, role, stateText(session))}
    tabindex={stop ? 0 : -1}
    class="tab seat"
    data-state={session.state}
    data-activity={activity(session)}
    data-degraded={session.degraded.length > 0}
    data-glow={glowing.has(session.id)}
    style:--accent={role.color}
    onclick={() => selectSession(session.id)}
  >
    <span class="avatar">
      <Creature role={role.slug} color={role.color} size={36} />
      {#if session.degraded.length}<span class="warn-badge" aria-hidden="true">!</span>{/if}
    </span>
    <span class="text">
      <span class="name-row">
        <span class="name">{session.identity}</span>
        <span class="harness mono">{session.seat}</span>
        {#if code}<span class="code-inline mono">{code}</span>{/if}
      </span>
      <span class="detail">{code ? `${role.displayName} // ${code}` : role.displayName}</span>
      <span class="detail state-text">{stateText(session)}</span>
      {#if session.degraded.length}
        <span class="degraded">started without {session.degraded.join(", ")}</span>
      {/if}
    </span>
  </button>
{/snippet}

<nav class="sidebar" aria-label="Hosts and sessions">
  <div class="mark">ATERM</div>

  <h2 id="hosts-label">Hosts</h2>
  <div role="tablist" aria-labelledby="hosts-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
    {#each app.hosts as host (host.id)}
      {@const selected = app.selectedHostId === host.id && !onScreen}
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
    <h2 id="sessions-label">Running <span class="count">{live.running.length}</span></h2>
    {#if live.running.length === 0}
      <p class="waiting">Nothing running on this host. Start a seat below.</p>
    {:else}
      <div role="tablist" aria-labelledby="sessions-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
        {#each live.running as session (session.id)}{@render seatTab(session, live.running)}{/each}
      </div>
    {/if}

    {#if live.failed.length}
      <h2 id="failed-label">Launch failed <span class="count">{live.failed.length}</span></h2>
      <div role="tablist" aria-labelledby="failed-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
        {#each live.failed as session (session.id)}{@render seatTab(session, live.failed)}{/each}
      </div>
    {/if}

    <h2 id="start-label">Start a seat</h2>
    {#if app.roles.length === 0}
      <p class="waiting">Waiting for the roster.</p>
    {:else}
      <div role="tablist" aria-labelledby="start-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
        {#each app.roles as role (role.slug)}
          {@const selected = app.selectedRole === role.slug}
          <button
            role="tab"
            id={`tab-role-${role.slug}`}
            aria-selected={selected}
            aria-controls="main-panel"
            tabindex={selected || (!app.selectedRole && role === app.roles[0]) ? 0 : -1}
            class="tab start"
            style:--accent={role.color}
            onclick={() => selectRole(role.slug)}
          >
            <span class="avatar"><Creature role={role.slug} color={role.color} size={24} /></span>
            <span class="text"><span class="name">{role.displayName}</span><span class="detail">{harnesses(role.seats)}</span></span>
          </button>
        {/each}
      </div>
    {/if}
  {/if}

  <AttentionSwitches />
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
  .seat[data-glow="true"] { background: linear-gradient(90deg, color-mix(in srgb, var(--accent) 45%, var(--ground)), color-mix(in srgb, var(--accent) 8%, var(--ground)) 85%); box-shadow: inset 4px 0 0 var(--accent); }
  .dot { width: 10px; height: 10px; border-radius: 5px; background: var(--ok); flex: none; }
  [data-kind="unreachable"] .dot { background: none; border: 2px solid var(--danger); }
  [data-kind="checking"] .dot { background: none; border: 2px solid var(--muted); }
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
  .name-row { display: flex; align-items: baseline; gap: 6px; min-width: 0; }
  .harness { flex: none; font-size: 11px; padding: 1px 6px; border-radius: 4px; border: 1px solid color-mix(in srgb, var(--accent) 60%, var(--line)); color: var(--text-soft); }
  .code-inline { display: none; font-size: 11px; color: var(--muted); }
  .state-text { color: var(--text-soft); }
  [data-activity="unseen"] .state-text, [data-activity="asking"] .state-text { color: var(--brand); }
  .count { font-family: var(--font-body); letter-spacing: 0; color: var(--text-soft); }
  .degraded { font-size: 12px; color: var(--warn-text); }
  .degraded::before { content: "! "; font-weight: 700; color: var(--warn); }
  .warn-badge {
    position: absolute; bottom: -4px; right: -4px; z-index: 2; width: 16px; height: 16px;
    border-radius: 8px; background: var(--warn); color: var(--ground); box-shadow: 0 0 0 2px var(--ground);
    font: 700 11px/16px var(--font-body); text-align: center;
  }
  .start { min-height: 44px; padding: 6px 10px; }
  .start .name { font-weight: 400; font-size: 14px; color: var(--text-soft); }
  .start :global(.creature) { opacity: 0.7; }
  .start[aria-selected="true"] { background: color-mix(in srgb, var(--accent) 14%, var(--ground)); box-shadow: inset 0 0 0 1px var(--accent); }
  .detail { font-size: 12px; color: var(--muted); }
  [data-kind="unreachable"] .detail, [data-state="failed"] .detail { color: #ff9aa5; }
  .detail, .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .waiting { margin: 0 10px; color: var(--muted); font-size: 14px; }

  @media (max-width: 720px) {
    .sidebar { width: auto; border-right: none; border-bottom: 1px solid var(--line); padding: 8px 0; gap: 4px; overflow: visible; }
    .mark, h2, .waiting { display: none; }
    [role="tablist"] { flex-direction: row; overflow-x: auto; padding: 0 16px; scrollbar-width: none; }
    .tab { width: auto; flex: none; }
    .detail, .degraded { display: none; }
    .code-inline { display: inline; }
  }
</style>
