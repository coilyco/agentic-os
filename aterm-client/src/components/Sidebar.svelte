<script lang="ts">
  import { onMount, tick } from "svelte";
  import AttentionSwitches from "./AttentionSwitches.svelte";
  import Creature from "./Creature.svelte";
  import FeedbackEntry from "./FeedbackEntry.svelte";
  import SeatPreview from "./SeatPreview.svelte";
  import TapTraceSwitch from "./TapTraceSwitch.svelte";
  import { app, askLineFor, asksShownOn, colorOf, glowSeats, layout, openByPerson, reconnecting, selectHost, selectRole, sessionById, waitingSeats } from "../lib/app.svelte";
  import { previewFor } from "../lib/preview";
  import type { Host, Session } from "../lib/protocol";
  import { stateWithAge } from "../lib/age";
  import { activityFlag, activityWords } from "../lib/seat-activity";
  import { hostRunning, orderSessions, roleFor, sessionCode, sessionLabel, splitSessions } from "../lib/sessions";
  import { tablistKeys } from "../lib/tabs";

  const live = $derived(splitSessions(orderSessions(app.sessions, app.roles)));
  const onScreen = $derived(Boolean(app.selectedSession || app.selectedRole));
  const glowing = $derived(new Set(app.alerts.visual ? glowSeats().map((seat) => seat.sessionId) : []));
  // On a phone the one seat on screen is named here and nowhere else, and the rest waits behind it.
  const current = $derived(sessionById(app.selectedSession));
  const currentRole = $derived(current ? roleFor(current, app.roles) : app.roles.find((role) => role.slug === app.selectedRole));
  const waitingElsewhere = $derived(waitingSeats().filter((seat) => seat.sessionId !== app.selectedSession).length);
  // A phone on its side reads one seat edge to edge. The list and bar fold into a rail, and the lists open beside it.
  const reading = $derived(layout.reading && onScreen);
  // The host tab that holds the tab stop, even while a seat is on screen.
  const hereHost = $derived(app.selectedHostId ?? app.hosts[0]?.id);

  // Tracks the stylesheet's own breakpoint, so the bar and menu exist exactly when the stylesheet draws them.
  let narrow = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px)").matches);
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const read = () => (narrow = query.matches);
    read();
    query.addEventListener("change", read);
    return () => query.removeEventListener("change", read);
  });

  // Over a seat, a phone's menu is a sheet: seats, then roles to start, then alerts and hosts folded.
  const sheet = $derived((narrow || reading) && onScreen);
  const alertsSaid = $derived(`Sound ${app.alerts.sound ? "on" : "off"}, highlight ${app.alerts.visual ? "on" : "off"}`);

  // On a phone the seat gets the screen, and hosts, roles and alerts sit behind this button.
  // With no seat on screen those lists are the page, so there is nothing to open.
  let menuOpen = $state(false);
  let menuButton = $state<HTMLButtonElement>();
  $effect(() => {
    if (!onScreen) menuOpen = false;
  });

  async function toggleMenu(): Promise<void> {
    menuOpen = !menuOpen;
    if (!menuOpen) return;
    await tick();
    document.querySelector<HTMLElement>('#sidebar-menu [role="tab"][tabindex="0"]')?.focus();
  }

  // Focus that sat on a menu row would fall to the page when the row goes, so it returns to the button.
  function closeMenu(): void {
    if (!menuOpen) return;
    const inside = document.getElementById("sidebar-menu")?.contains(document.activeElement);
    menuOpen = false;
    if (inside) void tick().then(() => menuButton?.focus());
  }

  function escape(event: KeyboardEvent): void {
    if (event.key !== "Escape" || !menuOpen || event.defaultPrevented) return;
    event.preventDefault();
    const inside = document.getElementById("sidebar-menu")?.contains(document.activeElement);
    menuOpen = false;
    if (inside || document.activeElement === menuButton) void tick().then(() => menuButton?.focus());
  }

  // A seat opened from off the row, as from a toast, scrolls into the row.
  $effect(() => {
    const id = app.selectedSession;
    if (!narrow || !id) return;
    void tick().then(() => document.getElementById(`tab-session-${id}`)?.scrollIntoView({ block: "nearest", inline: "nearest" }));
  });

  function openSession(id: string): void {
    openByPerson(id);
    closeMenu();
  }

  // Hovering or focusing a glowing seat says what it wants without opening it. A phone has no hover, so it gets the toast.
  let hover = $state<{ id: string; top: number; left: number } | null>(null);
  const canHover = () => typeof matchMedia === "function" && matchMedia("(hover: hover)").matches && !narrow;
  function showPreview(id: string, row: HTMLElement): void {
    if (!glowing.has(id) || !canHover()) return;
    const box = row.getBoundingClientRect();
    hover = { id, top: Math.max(8, box.top), left: box.right + 8 };
  }
  const hidePreview = () => (hover = null);
  // A finished seat is not counted as waiting, but its row still previews its last reply.
  const previewing = $derived.by(() => {
    if (!hover) return undefined;
    const id = hover.id;
    const needs = waitingSeats().find((seat) => seat.sessionId === id);
    const seat = sessionById(id);
    return needs ?? (seat && app.unseen[id] ? { sessionId: id, role: seat.role, identity: seat.identity, kind: "done" as const } : undefined);
  });
  // A relayed question names the worker who asked, not the director whose pane shows it.
  const previewAsk = $derived(previewing ? asksShownOn(previewing.sessionId)[0] : undefined);
  const preview = $derived(
    previewing
      ? previewFor({ ...previewing, identity: (previewAsk && sessionById(previewAsk.session)?.identity) || previewing.identity }, previewAsk, sessionById(previewing.sessionId)?.lastReply)
      : null,
  );

  function openRole(slug: string): void {
    selectRole(slug);
    closeMenu();
  }

  // The attached host's state is its link, not the probe that found it.
  const kindOf = (host: Host): string => (reconnecting() && host.id === app.attachedHostId ? "reconnecting" : host.status.kind);

  function hostDetail(host: Host): string {
    if (kindOf(host) === "reconnecting") return app.link.state === "resting" ? "not answering" : "reconnecting";
    const running = hostRunning(host, app.attachedHostId, app.sessions);
    if (running !== null) return host.found ? `${running} running, found on tailnet` : `${running} running`;
    if (host.status.kind === "unreachable") return host.status.layer && host.status.layer !== "network" ? "refused" : "no answer";
    return "checking";
  }

  // Quiet times count up on their own, so the rows are redrawn every ten seconds.
  let now = $state(Date.now());
  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 10_000);
    return () => clearInterval(timer);
  });

  function stateText(session: Session, asking: string | null): string {
    if (reconnecting()) return `${session.state}, last known`;
    if (session.state === "failed") return "launch failed";
    if (asking) return asking;
    if (app.unseen[session.id]) return stateWithAge("done, your turn", session.quietSince, now);
    if (session.drafting) return "you're typing";
    if (session.pending) return `${session.pending} ${session.pending === 1 ? "message" : "messages"} waiting`;
    // A seat still starting has written nothing yet, so there is no quiet time to count.
    return stateWithAge(session.state, session.starting ? undefined : session.quietSince, now);
  }

  function activity(session: Session, asking: string | null): string {
    if (session.state === "failed") return "failed";
    if (asking) return "asking";
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
  {@const asking = askLineFor(session.id)}
  {@const stop = group.some((each) => each.id === app.selectedSession) ? selected : session === group[0]}
  <button
    role="tab"
    id={`tab-session-${session.id}`}
    aria-selected={selected}
    aria-controls="main-panel"
    aria-label={sessionLabel(session, role, [stateText(session, asking), activityWords(session.activity, now)].filter(Boolean).join(", "))}
    tabindex={stop ? 0 : -1}
    class="tab seat"
    data-state={session.state}
    data-activity={activity(session, asking)}
    data-degraded={session.degraded.length > 0}
    data-glow={glowing.has(session.id)}
    aria-describedby={hover?.id === session.id && preview ? "seat-preview" : undefined}
    style:--accent={role.color}
    onclick={() => openSession(session.id)}
    onpointerenter={(event) => showPreview(session.id, event.currentTarget)}
    onpointerleave={hidePreview}
    onfocus={(event) => showPreview(session.id, event.currentTarget)}
    onblur={hidePreview}
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
      <span class="detail state-text">{stateText(session, asking)}</span>
      {#if session.activity}
        {@const flag = activityFlag(session.activity, now)}
        <span class="activity" data-stale={flag !== ""}>
          <span class="line">{session.activity.text}</span>
          {#if flag}<span class="age">{flag}</span>{/if}
        </span>
      {/if}
      {#if session.degraded.length}
        <span class="degraded">started without {session.degraded.join(", ")}</span>
      {/if}
    </span>
  </button>
{/snippet}

{#snippet hostsGroup()}
  <h2 id="hosts-label">Hosts</h2>
  <div role="tablist" aria-labelledby="hosts-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
    {#each app.hosts as host (host.id)}
      {@const selected = app.selectedHostId === host.id && !onScreen}
      <button
        role="tab"
        id={`tab-host-${host.id}`}
        aria-selected={selected}
        aria-controls="main-panel"
        tabindex={host.id === hereHost ? 0 : -1}
        class="tab"
        data-kind={kindOf(host)}
        onclick={() => selectHost(host)}
      >
        <span class="dot" aria-hidden="true"></span>
        <span class="text"><span class="name">{host.label}</span><span class="detail">{hostDetail(host)}</span></span>
      </button>
    {/each}
  </div>
{/snippet}

{#snippet runningGroup()}
  <h2 id="sessions-label">Running <span class="count">{live.running.length}</span></h2>
  {#if live.running.length === 0}
    <p class="waiting">{narrow ? "Nothing running yet." : "Nothing running on this host. Start a seat below."}</p>
  {:else}
    <div role="tablist" aria-labelledby="sessions-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
      {#each live.running as session (session.id)}{@render seatTab(session, live.running)}{/each}
    </div>
  {/if}
{/snippet}

{#snippet failedGroup()}
  {#if live.failed.length}
    <h2 id="failed-label">Launch failed <span class="count">{live.failed.length}</span></h2>
    <div role="tablist" aria-labelledby="failed-label" aria-orientation="vertical" tabindex="-1" onkeydown={tablistKeys}>
      {#each live.failed as session (session.id)}{@render seatTab(session, live.failed)}{/each}
    </div>
  {/if}
{/snippet}

{#snippet startGroup()}
  <h2 id="start-label">Start a seat</h2>
  {#if app.roles.length === 0}
    <p class="waiting">Waiting for the roster.</p>
  {:else}
    <div role="tablist" class:chips={sheet} aria-labelledby="start-label" aria-orientation={sheet ? "horizontal" : "vertical"} tabindex="-1" onkeydown={tablistKeys}>
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
          onclick={() => openRole(role.slug)}
        >
          <span class="avatar"><Creature role={role.slug} color={role.color} size={24} /></span>
          <span class="text"><span class="name">{role.displayName}</span><span class="detail">{harnesses(role.seats)}</span></span>
        </button>
      {/each}
    </div>
  {/if}
{/snippet}

<!-- Escape is heard here only because the keys it answers come from controls inside the menu. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<nav class="sidebar" aria-label="Hosts and sessions" data-link={app.link.state} data-mode={reading ? "reading" : narrow ? "narrow" : "wide"} onkeydown={escape}>
  {#if narrow || reading}
    {#if onScreen || app.attachedHostId}
      <div class="bar" class:on-seat={onScreen}>
        {#if onScreen}
          <button
            type="button"
            class="switcher"
            bind:this={menuButton}
            style:--accent={currentRole?.color}
            aria-expanded={menuOpen}
            aria-controls="sidebar-menu"
            aria-label={`${current?.identity ?? `Start ${currentRole?.displayName ?? "a seat"}`}. Seats, starting a seat, hosts and alerts`}
            onclick={toggleMenu}
          >
            {#if currentRole}<Creature role={currentRole.slug} color={currentRole.color} size={28} />{/if}
            <span class="who">{current?.identity ?? `Start ${currentRole?.displayName ?? "a seat"}`}</span>
            {#if current}<span class="state mono" data-state={current.state}>{current.state}</span>{/if}
            {#if waitingElsewhere}<span class="elsewhere" aria-hidden="true">{waitingElsewhere}<span class="word">&nbsp;waiting</span></span>{/if}
            <svg class="chevron" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M3 6l5 5 5-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg>
          </button>
          <FeedbackEntry compact />
        {:else if app.attachedHostId}
          <div class="seats">{@render runningGroup()}{@render failedGroup()}</div>
        {/if}
      </div>
    {/if}
    {#if onScreen && menuOpen}
      <!-- A click on the dimmed page puts the menu away. Escape does the same from the keyboard. -->
      <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
      <div class="scrim" onclick={closeMenu}></div>
    {/if}
    {#if !onScreen || menuOpen}
      <div id="sidebar-menu" class="menu" data-flow={onScreen ? "sheet" : "page"}>
        <!-- Over a seat the menu is the seat switcher too, since the bar names only the one on screen. -->
        {#if onScreen}
          {#if app.attachedHostId}{@render runningGroup()}{@render failedGroup()}{@render startGroup()}{/if}
          <details class="more">
            <summary>
              <span>Alerts and hosts</span>
              <span class="said">{alertsSaid}</span>
              <svg class="chev" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M3 6l5 5 5-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg>
            </summary>
            <AttentionSwitches />
            {@render hostsGroup()}
          </details>
        {:else}
          {@render hostsGroup()}
          {#if app.attachedHostId}{@render startGroup()}{/if}
          <AttentionSwitches compact />
        {/if}
        <div class="feedback"><FeedbackEntry onopen={closeMenu} /></div>
        <TapTraceSwitch />
      </div>
    {/if}
  {:else}
    <div class="mark">TREEHOUSE</div>
    {@render hostsGroup()}
    {#if app.attachedHostId}
      {@render runningGroup()}
      {@render failedGroup()}
      {@render startGroup()}
    {/if}
    <AttentionSwitches />
    <div class="feedback"><FeedbackEntry /></div>
    <TapTraceSwitch />
  {/if}
</nav>
{#if hover && preview}
  <SeatPreview {preview} top={hover.top} left={hover.left} color={colorOf(sessionById(hover.id)?.role ?? "")} />
{/if}


<style>
  .sidebar { width: 280px; flex: none; border-right: 1px solid var(--line); padding: 16px 12px; display: flex; flex-direction: column; gap: 6px; overflow-y: auto; }
  .feedback { margin-top: 14px; padding: 0 2px; }
  .mark { font-family: var(--font-display); font-weight: 700; font-size: 20px; letter-spacing: 0.08em; color: var(--brand); padding: 4px 10px 12px; }
  h2 { margin: 14px 10px 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  [role="tablist"] { display: flex; flex-direction: column; gap: 2px; }
  .tab { width: 100%; min-height: 48px; display: flex; align-items: center; gap: 12px; padding: 8px 10px; border: none; border-radius: 8px; background: transparent; color: var(--text); text-align: left; }
  .tab:hover { background: #171a21; }
  .tab[aria-selected="true"] { background: #1d1729; box-shadow: inset 0 0 0 1px #3a3350; }
  .seat[aria-selected="true"] { background: color-mix(in srgb, var(--accent) 14%, var(--ground)); box-shadow: inset 0 0 0 1px var(--accent); }
  /* A waiting seat is a faint tint and a thin rail. The "?" or dot on the avatar and the brand-coloured name do the telling. */
  .seat[data-glow="true"] { background: color-mix(in srgb, var(--accent) 9%, var(--ground)); box-shadow: inset 2px 0 0 var(--accent); }
  .dot { width: 10px; height: 10px; border-radius: 5px; background: var(--ok); flex: none; }
  [data-kind="unreachable"] .dot { background: none; border: 2px solid var(--danger); }
  [data-kind="checking"] .dot { background: none; border: 2px solid var(--muted); }
  [data-kind="reconnecting"] .dot { background: none; border: 2px solid var(--warn); }
  [data-kind="reconnecting"] .detail { color: var(--warn-text); }
  /* Colour and the glow drain, the words stay at full contrast. The seats are the last known, not live. */
  [data-link="reconnecting"] .seat .avatar { filter: grayscale(1); opacity: 0.6; }
  [data-link="reconnecting"] .seat[data-glow="true"] { background: transparent; box-shadow: none; }
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
  /* One line, cut with an ellipsis. Past a few minutes the text mutes and the age stays whole. */
  .activity { display: flex; align-items: baseline; gap: 6px; min-width: 0; font-size: 12px; color: var(--text-soft); }
  .activity .line { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .activity .age { flex: none; color: var(--muted); }
  .activity[data-stale="true"] .line { color: var(--muted); }
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

  /* A phone: one row of seats and a Menu button, so the terminal gets the screen. Hosts, roles and alerts open over it. */
  @media (max-width: 720px) {
    .sidebar { width: auto; flex: none; position: relative; z-index: 7; padding: 0; gap: 0; overflow: visible; border-right: none; border-bottom: 1px solid var(--line); }
    .bar { display: flex; align-items: center; gap: 8px; min-height: 56px; padding: 4px 8px 4px 12px; background: var(--ground); }
    .bar.on-seat { min-height: 48px; padding: 2px 8px; }
    .switcher { flex: 1; min-width: 0; min-height: 44px; padding: 0 10px; display: flex; align-items: center; gap: 10px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font-weight: 600; text-align: left; }
    .switcher[aria-expanded="true"] { border-color: var(--accent, var(--brand)); background: #1d1729; }
    .switcher .who { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-display); font-size: 16px; }
    .switcher .state { flex: none; font-size: 11px; padding: 1px 8px; border-radius: 999px; border: 1px solid var(--accent, var(--line)); color: var(--text-soft); font-weight: 400; }
    .elsewhere { flex: none; font-size: 12px; padding: 1px 8px; border-radius: 999px; background: var(--brand); color: var(--brand-ink); }
    .chevron { flex: none; color: var(--muted); }
    .seats { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; overflow-x: auto; scrollbar-width: none; }
    .seats h2 { display: none; }
    .seats [role="tablist"] { flex-direction: row; gap: 8px; }
    .seats .tab { width: auto; flex: none; }
    .seats .waiting { margin: 0 4px; }
    .seats .detail, .seats .degraded { display: none; }
    /* A chip grows to carry the line, and no further, so the next chip still shows at the edge. */
    .seats .activity { max-width: 190px; }
    /* Name over harness and code, so a chip is narrow enough to leave the next one showing at its edge. */
    .seats .name-row { display: grid; grid-template-columns: auto auto 1fr; column-gap: 6px; align-items: baseline; }
    .seats .name { grid-column: 1 / -1; font-size: 14px; line-height: 1.2; }
    .seats .harness { padding: 0; border: none; font-size: 11px; color: var(--muted); }
    .seats .code-inline { display: inline; }
    .scrim { position: fixed; inset: 0; z-index: -1; background: rgb(0 0 0 / 0.55); }
    .menu[data-flow="sheet"] { position: absolute; top: 100%; left: 0; right: 0; max-height: calc(100dvh - 49px); overflow-y: auto; overscroll-behavior: contain; padding: 0 8px calc(12px + env(safe-area-inset-bottom)); background: var(--ground); border-bottom: 1px solid var(--line); box-shadow: 0 12px 16px rgb(0 0 0 / 0.4); }
    /* With no seat on screen the lists are the page: today's rows, tight, above the host's own panel. */
    .menu[data-flow="page"] { padding: 4px 0; display: flex; flex-direction: column; gap: 4px; }
    .menu[data-flow="page"] h2, .menu[data-flow="page"] .detail { display: none; }
    .menu[data-flow="page"] [role="tablist"] { flex-direction: row; overflow-x: auto; padding: 0 16px; scrollbar-width: none; }
    .menu[data-flow="page"] .tab { width: auto; flex: none; }
    .menu[data-flow="page"] .waiting { display: none; }
  }

  /* The seat sheet, phone and landscape drawer alike: two-line seats, roles in one sideways row, alerts and hosts folded. */
  .menu[data-flow="sheet"] .seat .text { flex-direction: row; flex-wrap: wrap; column-gap: 8px; }
  .menu[data-flow="sheet"] .seat .name-row { flex-basis: 100%; }
  .menu[data-flow="sheet"] .seat .state-text { order: 1; }
  .menu[data-flow="sheet"] .seat .text > .detail:not(.state-text) { order: 2; }
  .menu[data-flow="sheet"] .seat .text > .detail:not(.state-text)::before { content: "// "; }
  .menu[data-flow="sheet"] .seat .activity { order: 3; flex-basis: 100%; }
  .menu[data-flow="sheet"] .seat .degraded { order: 4; flex-basis: 100%; }
  .menu[data-flow="sheet"] .chips { flex-direction: row; gap: 8px; overflow-x: auto; padding: 2px 10px 8px; scrollbar-width: none; }
  .menu[data-flow="sheet"] .chips .tab { flex: none; width: 92px; min-height: 76px; flex-direction: column; justify-content: center; gap: 4px; padding: 6px 4px; text-align: center; border: 1px solid var(--line); }
  .menu[data-flow="sheet"] .chips .tab[aria-selected="true"] { border-color: var(--accent); }
  .menu[data-flow="sheet"] .chips .text { align-items: center; }
  .menu[data-flow="sheet"] .chips .name { font-size: 12px; line-height: 1.2; white-space: normal; text-align: center; overflow: visible; }
  .menu[data-flow="sheet"] .chips .detail { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .more { margin-top: 8px; border-top: 1px solid var(--line); }
  .more > summary { list-style: none; min-height: 48px; display: flex; flex-wrap: wrap; align-items: center; column-gap: 8px; padding: 6px 10px; cursor: pointer; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  .more > summary::-webkit-details-marker { display: none; }
  .more > summary:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; border-radius: 8px; }
  .more > summary > span:first-child { flex: 1; }
  .more .said { order: 3; flex-basis: 100%; font: 400 12px var(--font-body); letter-spacing: 0; text-transform: none; }
  .more .chev { flex: none; }
  .more[open] .chev { transform: rotate(180deg); }

  /* Reading view, a phone on its side: the bar becomes a rail, so the seat keeps the full height. Chosen in script, since the width is past the phone breakpoint. */
  .sidebar[data-mode="reading"] { width: 52px; flex: none; position: relative; z-index: 7; padding: 0; gap: 0; overflow: visible; border-right: 1px solid var(--line); border-bottom: none; }
  .sidebar[data-mode="reading"] .bar { flex-direction: column; align-items: center; min-height: 0; height: 100%; padding: 4px; }
  .sidebar[data-mode="reading"] .switcher { flex: none; width: 44px; min-height: 44px; padding: 0; justify-content: center; position: relative; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; }
  .sidebar[data-mode="reading"] .switcher[aria-expanded="true"] { border-color: var(--accent, var(--brand)); background: #1d1729; }
  .sidebar[data-mode="reading"] :is(.who, .state, .chevron, .elsewhere .word) { display: none; }
  .sidebar[data-mode="reading"] .elsewhere { position: absolute; top: -4px; right: -4px; min-width: 18px; padding: 0 4px; font-size: 11px; line-height: 18px; text-align: center; border-radius: 9px; background: var(--brand); color: var(--brand-ink); }
  .sidebar[data-mode="reading"] .scrim { position: fixed; inset: 0; z-index: -1; background: rgb(0 0 0 / 0.55); }
  .sidebar[data-mode="reading"] .menu[data-flow="sheet"] { position: fixed; top: 0; bottom: 0; left: calc(env(safe-area-inset-left) + 52px); right: auto; width: min(340px, 70vw); max-height: none; overflow-y: auto; padding: 0 8px calc(12px + env(safe-area-inset-bottom)); background: var(--ground); border-right: 1px solid var(--line); border-bottom: none; box-shadow: 8px 0 16px rgb(0 0 0 / 0.4); }
</style>
