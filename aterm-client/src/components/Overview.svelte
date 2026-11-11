<script lang="ts">
  import { onMount } from "svelte";
  import { canopy } from "../lib/theme.svelte";
  import CloseSeat from "./CloseSeat.svelte";
  import Creature from "./Creature.svelte";
  import PagesList from "./PagesList.svelte";
  import { app, askLineFor, needsKaiIds, openByPerson, openPage, openSettings, pinnedNow, selectedHost, showAllRoles, showLogo } from "../lib/app.svelte";
  import { pageRows } from "../lib/artifact-pages";
  import { ageText } from "../lib/age";
  import { splitHidden } from "../lib/hidden-roles";
  import { groupSeats, type GroupKey } from "../lib/overview";
  import type { Session } from "../lib/protocol";
  import { activityFlag } from "../lib/seat-activity";
  import { orderSessions, roleFor, sessionLabel, splitSessions } from "../lib/sessions";

  // Where the app rests (COI-3518): every seat, sorted by what it wants of Kai. The host is a label.
  const host = $derived(selectedHost());
  const needs = $derived(needsKaiIds());
  // A hidden role's quiet seats stay off the screen, as they do in the list. A pinned one stays, and pins sort first.
  const split = $derived(splitHidden(orderSessions(app.sessions, app.roles, pinnedNow), app.hiddenRoles, (seat) => seat.state === "failed" || pinnedNow(seat) || needs.has(seat.id)));
  const groups = $derived(groupSeats(split.shown, { needs, unseen: app.unseen }));
  const running = $derived(splitSessions(app.sessions).running.length);
  const pages = $derived(pageRows(Object.values(app.artifacts), app.sessions));
  // The stylesheet's own breakpoint, as the list tracks it, since Close stacks under a row on a phone.
  let narrow = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px)").matches);
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const read = () => (narrow = query.matches);
    read();
    query.addEventListener("change", read);
    return () => query.removeEventListener("change", read);
  });

  // Times count up on their own, so the rows are redrawn every ten seconds.
  let now = $state(Date.now());
  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 10_000);
    return () => clearInterval(timer);
  });

  function since(session: Session): string {
    const from = session.stateSince ?? session.quietSince;
    return from === undefined ? "" : ageText(Math.max(0, (now - from) / 1000));
  }

  /** The words at the right of a row: what Kai can do, else how long it has been so. */
  function trailing(session: Session, key: GroupKey): string {
    if (session.state === "failed") return "launch failed";
    if (key === "needs") return askLineFor(session.id) ? "Answer" : "Open";
    if (key === "working") return since(session);
    const age = since(session);
    if (app.unseen[session.id]) return age ? `your turn ${age}` : "your turn";
    return age ? `idle ${age}` : "idle";
  }

  /** The one line under the name: what the seat asks, else what it says it is doing, else its last reply. */
  function line(session: Session, key: GroupKey): { text: string; old: string } {
    const asking = key === "needs" ? askLineFor(session.id) : null;
    if (asking) return { text: asking, old: "" };
    if (session.state === "failed") return { text: session.failure ?? "It did not start.", old: "" };
    if (session.activity) return { text: session.activity.text, old: activityFlag(session.activity, now, session.stateSince) };
    return { text: session.lastReply ?? "", old: "" };
  }

  function name(session: Session, key: GroupKey, title: string): string {
    const role = roleFor(session, app.roles);
    const said = line(session, key);
    return sessionLabel(session, role, [title, trailing(session, key), said.text].filter(Boolean).join(", "));
  }
</script>

<section class="overview" aria-labelledby="overview-title">
  <header>
    <button type="button" class="logo" aria-label="Treehouse logo screen" onclick={showLogo}><img src={canopy()} alt="" width="32" height="32" /></button>
    <div class="who">
      <h1 id="overview-title">Seats</h1>
      <p class="host">{host?.label ?? ""} // {running} {running === 1 ? "seat" : "seats"}</p>
    </div>
    <button type="button" class="settings" onclick={openSettings}>Settings</button>
  </header>

  {#if app.notice}<p class="note" role="status">{app.notice}</p>{/if}

  {#if pages.length}<PagesList rows={pages} onopen={openPage} headingId="overview-pages" />{/if}

  {#if groups.length === 0 && split.hidden.length === 0}
    <p class="empty" role="status">Nothing is running on {host?.label ?? "this host"}. Start a seat from the roles in the list.</p>
  {/if}

  {#each groups as group (group.key)}
    <section class="group" data-group={group.key} aria-labelledby={`overview-${group.key}`}>
      <h2 id={`overview-${group.key}`}>{group.title} <span class="n">{group.seats.length}</span></h2>
      <ul>
        {#each group.seats as session (session.id)}
          {@const role = roleFor(session, app.roles)}
          {@const said = line(session, group.key)}
          <li class="row" data-group={group.key} data-seat={session.id} style:--accent={role.color}>
            <button type="button" class="open" aria-label={name(session, group.key, group.title)} onclick={() => openByPerson(session.id)}>
              <Creature role={role.slug} color={role.color} size={36} />
              <span class="text">
                <span class="top"><span class="name">{session.identity}</span><span class="role">{role.displayName}</span></span>
                {#if said.text}
                  <span class="line" data-stale={said.old !== ""}><span class="said">{said.text}</span>{#if said.old}<span class="old">{said.old}</span>{/if}</span>
                {/if}
              </span>
              <span class="act" data-act={group.key === "needs"}>{trailing(session, group.key)}</span>
            </button>
            {#if group.key === "done"}<CloseSeat {session} needsKai={needs.has(session.id)} where="row" compact={narrow} />{/if}
          </li>
        {/each}
      </ul>
    </section>
  {/each}
  {#if split.hidden.length}
    <p class="hidden-note" role="status">{split.hidden.length} {split.hidden.length === 1 ? "seat is" : "seats are"} hidden. <button type="button" class="show" onclick={showAllRoles}>Show them</button></p>
  {/if}
</section>

<style>
  .overview { flex: 1; min-width: 0; max-width: 760px; width: 100%; margin: 0 auto; padding: 20px clamp(12px, 4vw, 40px) 32px; display: flex; flex-direction: column; gap: 18px; }
  header { display: flex; align-items: center; gap: 12px; }
  .who { flex: 1; min-width: 0; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 24px; line-height: 1.1; }
  .host { margin: 2px 0 0; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  .logo { flex: none; width: 44px; height: 44px; display: grid; place-items: center; border-radius: 10px; border: 1px solid var(--control-line); background: transparent; }
  .settings { flex: none; min-height: 44px; padding: 0 16px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .logo:focus-visible, .settings:focus-visible, .open:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .note { margin: 0; color: var(--text-soft); font-size: 14px; }
  .hidden-note { margin: 0; font-size: 14px; color: var(--muted); display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .show { min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); }
  .show:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .empty { margin: 8px 0; color: var(--muted); font-size: 16px; }
  .group { display: flex; flex-direction: column; gap: 6px; }
  h2 { margin: 0 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  .group[data-group="needs"] h2 { color: var(--warn-text); }
  .n { margin-left: 4px; font-weight: 400; }
  ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 6px; }
  .row { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .open { flex: 1; min-width: 0; min-height: 56px; display: flex; align-items: center; gap: 12px; padding: 8px 12px; border-radius: 10px; border: 1px solid var(--line); background: var(--terminal); color: var(--text); text-align: left; font: inherit; }
  .open:hover { border-color: var(--brand); }
  /* The same faint tint and thin rail a waiting seat gets in the list, so the two read as one thing. */
  .row[data-group="needs"] .open { background: color-mix(in srgb, var(--accent) 9%, var(--ground)); box-shadow: inset 2px 0 0 var(--accent); }
  .text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .top { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
  .name { font-weight: 600; font-size: 15px; white-space: nowrap; }
  .role { font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .line { display: flex; gap: 8px; min-width: 0; font-size: 13px; color: var(--text-soft); }
  .said { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .line[data-stale="true"] .said { color: var(--muted); }
  .old { flex: none; color: var(--muted); }
  .act { flex: none; max-width: 40%; font-size: 13px; color: var(--muted); text-align: right; overflow-wrap: anywhere; }
  .act[data-act="true"] { color: var(--warn-text); font-weight: 700; }
  /* At 320px the Close control goes under the row, as it does in the list. */
  @media (max-width: 420px) {
    .row:has(:global(.close), :global(.ask)) { flex-direction: column; align-items: stretch; }
    .act { max-width: 34%; }
  }
</style>
