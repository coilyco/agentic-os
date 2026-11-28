<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import Markdown from "./Markdown.svelte";
  import SentImages from "./SentImages.svelte";
  import { replyAge } from "../lib/last-reply";
  import { openLink } from "../lib/link-notice.svelte";
  import type { TranscriptChannel } from "../lib/protocol";
  import { linkTarget } from "../lib/terminal-extras";
  import { mergeItems, rowsFor, TranscriptRefused, type Level, type PeerRef, type TranscriptItem } from "../lib/transcript";
  import { cacheKey, catchUp, loadSnapshot, markerRow, MAX_ITEMS, saveSnapshot } from "../lib/transcript-cache";
  import { cancelPending, pendingFor, readPast, settlePending, shownText, waitingSince } from "../lib/pending-sends.svelte";
  import { app, layout } from "../lib/app.svelte";
  import { parseSent } from "../lib/sent-images";

  // `restore` is false for a device that has not unlocked, which never shows a kept page.
  let { channel, sessionId, hostId, restore, level, marker, onread, onrefused, onpeer }: { channel: TranscriptChannel; sessionId: string; hostId: string; restore: boolean; level: Level; marker: boolean; onread: () => void; onrefused: (reason: string) => void; onpeer: (peer: PeerRef) => void } = $props();

  const PAGE = 50;
  // The most items held at once, so a seat watched for hours costs what a fresh one does (COI-3648).
  const HOLD = 400;
  const POLL_MS = 2000;
  const MARKER_MS = 8000;
  let items = $state<TranscriptItem[]>([]);
  let cursor = "";
  // Sent back to ask for the page before the first item held.
  let older = $state<string | undefined>();
  let loadingOlder = $state(false);
  let phase = $state<"loading" | "ready">("loading");
  let stale = $state(false);
  // The newest items were dropped to make room for earlier ones. Reading stops until the person jumps to latest.
  let detached = $state(false);
  let updatedAt = $state<number | undefined>();
  // The page kept from last time is shown while the host is asked, and `catching` says so.
  let catching = $state(false);
  // The first item this device has not seen, from the catch-up on open. Live arrivals never set it.
  let newFrom = $state<string | undefined>();
  let heard = false;
  // svelte-ignore state_referenced_locally
  const key = cacheKey(hostId, sessionId);
  // svelte-ignore state_referenced_locally
  const kept = restore ? loadSnapshot(key) : Promise.resolve(null);
  let firstRead = true;
  let now = $state(Date.now());
  let list = $state<HTMLDivElement>();
  // Whether the view sits at the newest line, so arriving text follows and a person's place is kept.
  let pinned = $state(true);
  const rows = $derived(rowsFor(items, level));
  const sending = $derived(pendingFor(key));
  const markerKey = $derived(markerRow(items, rows.map((row) => row.key), newFrom));
  const age = $derived(replyAge(updatedAt, now));
  // A working seat queues a message and writes it only when it takes it, minutes later at
  // times. So an entry asks to be dismissed only once the seat has stopped without it (COI-3690).
  const QUIET_MS = 5000;
  const working = $derived(app.sessions.find((one) => one.id === sessionId)?.state === "working");
  let quiet = $state(true);
  $effect(() => {
    if (working) {
      quiet = false;
      return;
    }
    const timer = setTimeout(() => (quiet = true), QUIET_MS);
    return () => clearTimeout(timer);
  });
  const shownState = (state: "pending" | "unsent") => (state === "unsent" && !quiet ? "queued" : state);

  const atEnd = () => !list || list.scrollHeight - list.scrollTop - list.clientHeight < 4;
  const follow = () => tick().then(() => pinned && list && (list.scrollTop = list.scrollHeight));

  function refused(error: unknown): boolean {
    if (!(error instanceof TranscriptRefused)) return false;
    onrefused(error.reason);
    return true;
  }

  async function newest(): Promise<void> {
    const reply = await channel.read(sessionId, { limit: PAGE });
    const held = firstRead ? await kept : null;
    firstRead = false;
    const joined = held ? catchUp(held, reply.items, reply.older) : null;
    items = joined ? joined.items : reply.items;
    older = joined ? joined.older : reply.older;
    // The marker is for opening a seat. Coming back from another view reads the page it kept a moment ago (COI-3647).
    newFrom = marker ? joined?.newFrom : undefined;
    settlePending(key, reply.items, false);
    void reachBack(reply.items, reply.older);
    cursor = reply.cursor;
    updatedAt = Date.now();
    phase = "ready";
    stale = false;
    detached = false;
    catching = false;
    heard = true;
    onread();
    if (newFrom) void showMarker();
    else void follow();
    keep();
  }

  // A seat may take a message while this view is closed, and a busy one buries it under the
  // newest page by the time it opens again. Reads back, without showing it, until each waiting
  // send settles or the pages pass the earliest of them (COI-3690).
  const REACH_PAGES = 10;
  let reaching = false;
  async function reachBack(page: readonly TranscriptItem[], from: string | undefined): Promise<void> {
    if (reaching) return;
    reaching = true;
    try {
      for (let read = 0; from && read < REACH_PAGES; read++) {
        const since = waitingSince(key);
        if (since === undefined || readPast(page, since)) return;
        const reply = await channel.read(sessionId, { before: from, limit: 200 });
        settlePending(key, reply.items, false);
        page = reply.items;
        from = reply.older;
      }
    } catch {
      // A failed read leaves the entry waiting, and its own timeout says so.
    } finally {
      reaching = false;
    }
  }

  // Puts the first new line at the top, since the end of a long catch-up is not where the person left.
  async function showMarker(): Promise<void> {
    pinned = false;
    await tick();
    list?.querySelector("[data-since]")?.scrollIntoView({ block: "start" });
    // A page that fits does not scroll, so no scroll event says whether the end is in view.
    pinned = atEnd();
  }

  let saving: ReturnType<typeof setTimeout> | undefined;
  function keep(): void {
    clearTimeout(saving);
    // A $state array is a proxy, which IndexedDB cannot clone, so a snapshot is stored.
    saving = setTimeout(() => void saveSnapshot(key, $state.snapshot(items.slice(-MAX_ITEMS)), older), 1000);
  }

  // A message the person just sent is where they look next, whatever they had scrolled to.
  let sentBefore = 0;
  $effect(() => {
    const count = sending.length;
    if (count > untrack(() => sentBefore)) {
      pinned = true;
      void follow();
    }
    sentBefore = count;
  });

  // The marker has done its job once the person has been at the newest line a while.
  $effect(() => {
    if (!newFrom || !pinned) return;
    const timer = setTimeout(() => (newFrom = undefined), MARKER_MS);
    return () => clearTimeout(timer);
  });

  let busy = false;
  async function poll(): Promise<void> {
    if (busy || detached || document.visibilityState === "hidden") return;
    busy = true;
    try {
      if (!cursor) await newest();
      else {
        const reply = await channel.read(sessionId, { since: cursor });
        if (reply.items.length) items = mergeItems(items, reply.items);
        // In the same update as the merge, so the real entry never shows beside its pending one.
        settlePending(key, reply.items, true);
        cursor = reply.cursor || cursor;
        updatedAt = Date.now();
        stale = false;
        if (reply.items.length) (void follow(), keep());
        // The older pointer belongs to the first item held, so the tail starts over from a fresh read.
        if (items.length > HOLD && pinned) await newest();
      }
    } catch (error) {
      if (error instanceof TranscriptRefused && error.reason === "bad_cursor") cursor = "";
      else if (!refused(error)) stale = true;
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    void poll();
    // A kept page paints at once if the host has not answered by the time it is read.
    void kept.then((held) => {
      if (!held || heard) return;
      items = held.items;
      older = held.older;
      updatedAt = held.at;
      phase = "ready";
      catching = true;
      void follow();
    });
    const timer = setInterval(() => void poll(), POLL_MS);
    const tickAge = setInterval(() => (now = Date.now()), 10_000);
    return () => {
      clearInterval(timer);
      clearInterval(tickAge);
      clearTimeout(saving);
      if (heard) void saveSnapshot(key, $state.snapshot(items.slice(-MAX_ITEMS)), older);
    };
  });

  // The page before the first item, put above it with the view kept where it was.
  async function earlier(): Promise<void> {
    if (!older || loadingOlder || !list) return;
    loadingOlder = true;
    // The oldest row on screen stays where it is, whatever is added above it or dropped below.
    const anchor = list.querySelector<HTMLElement>("[data-row]");
    const was = anchor?.getBoundingClientRect().top ?? 0;
    try {
      const reply = await channel.read(sessionId, { before: older, limit: PAGE });
      items = mergeItems(reply.items, items);
      older = reply.older;
      if (items.length > HOLD) {
        items = items.slice(0, HOLD);
        detached = true;
      }
      await tick();
      if (list && anchor?.isConnected) list.scrollTop += anchor.getBoundingClientRect().top - was;
    } catch (error) {
      // A pointer from an earlier visit may have gone stale. That ends the way back, not the view.
      if (error instanceof TranscriptRefused && error.reason === "bad_cursor") older = undefined;
      else if (!refused(error)) stale = true;
    } finally {
      loadingOlder = false;
    }
  }

  // A new level re-derives every row under the same scroll offset, so it starts again at the newest line (COI-3647).
  $effect(() => {
    void level;
    untrack(() => void jump());
    void tick().then(() => !detached && list && (list.scrollTop = list.scrollHeight));
  });

  // Held items that stop short of the newest are replaced by a fresh read of the newest page.
  async function jump(): Promise<void> {
    pinned = true;
    if (!detached) {
      if (list) list.scrollTop = list.scrollHeight;
      return;
    }
    if (busy) return;
    busy = true;
    try {
      await newest();
    } catch (error) {
      if (!refused(error)) stale = true;
    } finally {
      busy = false;
    }
  }

  function open(href: string): void {
    const target = linkTarget(href);
    if (target) void openLink(target);
  }
</script>

<!-- A shared item may be the person's own work or an answer to a peer, so it stays and points at the message. -->
{#snippet maybe(peer: PeerRef)}
  <p class="maybe">May answer {peer.from ? `a message from ${peer.from.identity}` : "another seat's message"}. <button type="button" class="peer" onclick={() => onpeer(peer)}>Show the message</button></p>
{/snippet}

<section class="transcript" aria-label="Transcript">
  <div class="feed" bind:this={list} onscroll={() => (pinned = atEnd())} tabindex="-1">
    {#if older}
      <button type="button" class="earlier" disabled={loadingOlder} onclick={() => void earlier()}>{loadingOlder ? "Reading earlier messages." : "Show earlier"}</button>
    {/if}
    {#if phase === "loading"}
      <p class="status" role="status">Reading this seat's transcript.</p>
    {:else if rows.length === 0 && sending.length === 0}
      <p class="status" role="status">No messages yet. Verbose has the raw screen.</p>
    {/if}
    {#each rows as row (row.key)}
      {#if row.key === markerKey}<p class="since" role="separator" data-since>New since you were last here</p>{/if}
      {#if row.kind === "prompt"}
        {@const sent = parseSent(row.text)}
        <div class="turn me" data-row={row.key}><span class="who">You</span>{#if sent.words}<Markdown source={sent.words} onlink={open} />{/if}{#if sent.paths.length}<SentImages paths={sent.paths} {sessionId} channel={app.connection?.images} />{/if}{#if row.cut}<p class="maybe">The host cut this message.</p>{/if}</div>
      {:else if row.kind === "reply"}
        <div class="turn" data-row={row.key}><Markdown source={row.text} onlink={open} />{#if row.cut}<p class="maybe">The host cut this reply.</p>{/if}{#if row.peer}{@render maybe(row.peer)}{/if}</div>
      {:else if row.kind === "tool"}
        <div class="step" data-row={row.key} data-failed={row.ok === false}>
          <p class="line">{row.summary}{#if row.ok === false}<span class="bad">failed</span>{/if}</p>
          {#if row.lines.length}<pre>{row.lines.join("\n")}</pre>{/if}
          {#if row.hidden}<p class="more">{row.hidden} more {row.hidden === 1 ? "line" : "lines"}. Verbose shows them.</p>{/if}
          {#if row.cut}<p class="more">The host cut this output.</p>{/if}
          {#if row.peer}{@render maybe(row.peer)}{/if}
        </div>
      {:else}
        <p class="note" data-row={row.key}>{row.text}</p>
      {/if}
    {/each}
    {#each sending as send (send.id)}
      <div class="turn me sending" data-row={`sending-${send.id}`} data-state={shownState(send.state)}>
        <span class="who">You</span>{#if send.text}<Markdown source={send.text} onlink={open} />{:else if !send.paths.length}<p class="plain">{shownText(send)}</p>{/if}
        {#if send.paths.length}<SentImages paths={send.paths} {sessionId} channel={app.connection?.images} />{/if}
        {#if shownState(send.state) === "queued"}
          <p class="maybe" role="status">Queued. The seat is working and takes it at its next step.</p>
        {:else if send.state === "unsent"}
          <p class="maybe warn" role="alert">Not in the transcript yet. The seat may not have taken it, for example while a dialog is open. <button type="button" class="peer" onclick={() => cancelPending(send.id)}>Dismiss</button></p>
        {:else}
          <p class="maybe" role="status">Sending.</p>
        {/if}
      </div>
    {/each}
  </div>
  {#if !pinned || detached}
    <button type="button" class="jump" onclick={() => void jump()}><span aria-hidden="true">↓</span> Jump to latest</button>
  {/if}
  <!-- A phone on its side keeps the line only when something is wrong or still catching up (COI-3371). -->
  {#if !layout.reading || catching || stale}
    <p class="source" role="status">{catching ? (stale ? `Showing what this device read ${age}. The host did not answer.` : `Showing what this device read ${age}. Catching up.`) : stale ? "Not updating. The host did not answer." : phase === "ready" ? `From the transcript, ${age}.` : ""}</p>
  {/if}
</section>

<style>
  .transcript { position: absolute; inset: 0; z-index: 4; display: flex; flex-direction: column; min-height: 0; background: var(--terminal); color: var(--text-soft); }
  .feed { flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 12px 16px; display: flex; flex-direction: column; gap: 12px; overscroll-behavior: contain; }
  .feed > :first-child { margin-top: auto; }
  .turn { min-width: 0; overflow-wrap: anywhere; font: 400 15px/1.5 var(--font-body); }
  .turn.me { border-left: 3px solid var(--brand); padding-left: 10px; color: var(--text); }
  .who { display: block; font: 600 11px var(--font-body); letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
  .plain { margin: 2px 0 0; white-space: pre-wrap; }
  /* Dashed until the harness has it, amber if it has not taken it: the colour rule's "take care". */
  .turn.sending { border-left-style: dashed; color: var(--text-soft); }
  .turn.sending[data-state="unsent"] { border-left-color: var(--warn); }
  .turn .maybe.warn { color: var(--warn-text); }
  .turn .maybe.warn .peer { padding-left: 0; }
  .step { min-width: 0; border: 1px solid var(--line); border-radius: 8px; padding: 6px 10px; font: 400 13px/1.45 var(--font-mono); color: var(--muted); }
  .step[data-failed="true"] { border-color: var(--danger-line); }
  .line { margin: 0; overflow-wrap: anywhere; color: var(--text-soft); }
  .bad { margin-left: 0.6em; color: var(--danger-text); font-weight: 600; }
  pre { margin: 6px 0 0; white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; color: var(--muted); }
  .since { margin: 4px 0; padding-top: 8px; border-top: 2px solid var(--brand); font: 600 12px var(--font-body); letter-spacing: 0.04em; color: var(--brand); }
  .more, .turn .maybe { margin: 4px 0 0; font: 400 12px var(--font-body); color: var(--muted); }
  .note, .status { margin: 0; font: 400 13px var(--font-body); color: var(--muted); }
  .source { flex: none; margin: 0; padding: 6px 16px; border-top: 1px solid var(--line); font: 400 12px var(--font-body); color: var(--muted); min-height: 30px; }
  .peer { min-height: 44px; padding: 0 10px; border: 0; background: none; color: var(--brand); font: 600 13px var(--font-body); text-decoration: underline; }
  .peer:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .earlier { flex: none; align-self: center; min-height: 44px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--control-line); background: var(--surface); color: var(--text); font: 600 14px var(--font-body); }
  .earlier:disabled { opacity: 0.6; }
  .earlier:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .jump { position: absolute; right: 12px; bottom: 44px; min-height: 44px; display: inline-flex; align-items: center; gap: 8px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--brand); background: var(--surface); color: var(--text); font: 600 14px var(--font-body); box-shadow: 0 6px 24px var(--shade); }
  .jump:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .turn :global(p) { margin: 0 0 8px; }
  .turn :global(p:last-child) { margin-bottom: 0; }
  .turn :global(pre) { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
