<script lang="ts">
  import { onMount, tick } from "svelte";
  import Markdown from "./Markdown.svelte";
  import { replyAge } from "../lib/last-reply";
  import { openLink } from "../lib/link-notice.svelte";
  import type { TranscriptChannel } from "../lib/protocol";
  import { linkTarget } from "../lib/terminal-extras";
  import { mergeItems, rowsFor, TranscriptRefused, type Level, type PeerRef, type TranscriptItem } from "../lib/transcript";
  import { inDemo } from "../lib/app.svelte";
  import { cacheKey, catchUp, loadSnapshot, markerRow, saveSnapshot } from "../lib/transcript-cache";

  // `restore` is false for a device that has not unlocked, which never shows a kept page.
  let { channel, sessionId, hostId, restore, level, marker, onread, onrefused, onpeer }: { channel: TranscriptChannel; sessionId: string; hostId: string; restore: boolean; level: Level; marker: boolean; onread: () => void; onrefused: (reason: string) => void; onpeer: (peer: PeerRef) => void } = $props();

  // Read once: a demo keeps no page, and leaving it must not write one on the way out.
  const demo = inDemo();
  const PAGE = 50;
  const POLL_MS = 2000;
  const MARKER_MS = 8000;
  let items = $state<TranscriptItem[]>([]);
  let cursor = "";
  // Sent back to ask for the page before the first item held.
  let older = $state<string | undefined>();
  let loadingOlder = $state(false);
  let phase = $state<"loading" | "ready">("loading");
  let stale = $state(false);
  let updatedAt = $state<number | undefined>();
  // The page kept from last time is shown while the host is asked, and `catching` says so.
  let catching = $state(false);
  // The first item this device has not seen, from the catch-up on open. Live arrivals never set it.
  let newFrom = $state<string | undefined>();
  let heard = false;
  // svelte-ignore state_referenced_locally
  const key = cacheKey(hostId, sessionId);
  // svelte-ignore state_referenced_locally
  const kept = restore && !demo ? loadSnapshot(key) : Promise.resolve(null);
  let firstRead = true;
  let now = $state(Date.now());
  let list = $state<HTMLDivElement>();
  // Whether the view sits at the newest line, so arriving text follows and a person's place is kept.
  let pinned = $state(true);
  const rows = $derived(rowsFor(items, level));
  const markerKey = $derived(markerRow(items, rows.map((row) => row.key), newFrom));
  const age = $derived(replyAge(updatedAt, now));

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
    cursor = reply.cursor;
    updatedAt = Date.now();
    phase = "ready";
    stale = false;
    catching = false;
    heard = true;
    onread();
    if (newFrom) void showMarker();
    else void follow();
    keep();
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
    if (demo) return;
    // A $state array is a proxy, which IndexedDB cannot clone, so a snapshot is stored.
    saving = setTimeout(() => void saveSnapshot(key, $state.snapshot(items), older), 1000);
  }

  // The marker has done its job once the person has been at the newest line a while.
  $effect(() => {
    if (!newFrom || !pinned) return;
    const timer = setTimeout(() => (newFrom = undefined), MARKER_MS);
    return () => clearTimeout(timer);
  });

  let busy = false;
  async function poll(): Promise<void> {
    if (busy || document.visibilityState === "hidden") return;
    busy = true;
    try {
      if (!cursor) await newest();
      else {
        const reply = await channel.read(sessionId, { since: cursor });
        if (reply.items.length) items = mergeItems(items, reply.items);
        cursor = reply.cursor || cursor;
        updatedAt = Date.now();
        stale = false;
        if (reply.items.length) (void follow(), keep());
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
      if (heard && !demo) void saveSnapshot(key, $state.snapshot(items), older);
    };
  });

  // The page before the first item, put above it with the view kept where it was.
  async function earlier(): Promise<void> {
    if (!older || loadingOlder || !list) return;
    loadingOlder = true;
    const before = list.scrollHeight - list.scrollTop;
    try {
      const reply = await channel.read(sessionId, { before: older, limit: PAGE });
      items = mergeItems(reply.items, items);
      older = reply.older;
      await tick();
      if (list) list.scrollTop = list.scrollHeight - before;
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
    pinned = true;
    void tick().then(() => list && (list.scrollTop = list.scrollHeight));
  });

  function jump(): void {
    pinned = true;
    if (list) list.scrollTop = list.scrollHeight;
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
    {:else if rows.length === 0}
      <p class="status" role="status">No messages yet. Verbose has the raw screen.</p>
    {/if}
    {#each rows as row (row.key)}
      {#if row.key === markerKey}<p class="since" role="separator" data-since>New since you were last here</p>{/if}
      {#if row.kind === "prompt"}
        <div class="turn me"><span class="who">You</span><p class="plain">{row.text}</p>{#if row.cut}<p class="maybe">The host cut this message.</p>{/if}</div>
      {:else if row.kind === "reply"}
        <div class="turn"><Markdown source={row.text} onlink={open} />{#if row.cut}<p class="maybe">The host cut this reply.</p>{/if}{#if row.peer}{@render maybe(row.peer)}{/if}</div>
      {:else if row.kind === "tool"}
        <div class="step" data-failed={row.ok === false}>
          <p class="line">{row.summary}{#if row.ok === false}<span class="bad">failed</span>{/if}</p>
          {#if row.lines.length}<pre>{row.lines.join("\n")}</pre>{/if}
          {#if row.hidden}<p class="more">{row.hidden} more {row.hidden === 1 ? "line" : "lines"}. Verbose shows them.</p>{/if}
          {#if row.cut}<p class="more">The host cut this output.</p>{/if}
          {#if row.peer}{@render maybe(row.peer)}{/if}
        </div>
      {:else}
        <p class="note">{row.text}</p>
      {/if}
    {/each}
  </div>
  {#if !pinned}
    <button type="button" class="jump" onclick={jump}><span aria-hidden="true">↓</span> Jump to latest</button>
  {/if}
  <p class="source" role="status">{catching ? (stale ? `Showing what this device read ${age}. The host did not answer.` : `Showing what this device read ${age}. Catching up.`) : stale ? "Not updating. The host did not answer." : phase === "ready" ? `From the transcript, ${age}.` : ""}</p>
</section>

<style>
  .transcript { position: absolute; inset: 0; z-index: 4; display: flex; flex-direction: column; min-height: 0; background: var(--terminal); color: var(--text-soft); }
  .feed { flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 12px 16px; display: flex; flex-direction: column; gap: 12px; overscroll-behavior: contain; }
  .feed > :first-child { margin-top: auto; }
  .turn { min-width: 0; overflow-wrap: anywhere; font: 400 15px/1.5 var(--font-body); }
  .turn.me { border-left: 3px solid var(--brand); padding-left: 10px; color: var(--text); }
  .who { display: block; font: 600 11px var(--font-body); letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
  .plain { margin: 2px 0 0; white-space: pre-wrap; }
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
