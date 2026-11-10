<script lang="ts">
  import { onMount, tick } from "svelte";
  import Markdown from "./Markdown.svelte";
  import { replyAge } from "../lib/last-reply";
  import { openLink } from "../lib/link-notice.svelte";
  import type { TranscriptChannel } from "../lib/protocol";
  import { linkTarget } from "../lib/terminal-extras";
  import { mergeItems, rowsFor, TranscriptRefused, type Level, type TranscriptItem } from "../lib/transcript";

  let { channel, sessionId, level, onrefused }: { channel: TranscriptChannel; sessionId: string; level: Level; onrefused: (reason: string) => void } = $props();

  const PAGE = 50;
  const POLL_MS = 2000;
  let items = $state<TranscriptItem[]>([]);
  let cursor = "";
  let phase = $state<"loading" | "ready">("loading");
  let stale = $state(false);
  let updatedAt = $state<number | undefined>();
  let now = $state(Date.now());
  let list = $state<HTMLDivElement>();
  // Whether the view sits at the newest line, so arriving text follows and a person's place is kept.
  let pinned = $state(true);
  const rows = $derived(rowsFor(items, level));
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
    items = reply.items;
    cursor = reply.cursor;
    updatedAt = Date.now();
    phase = "ready";
    stale = false;
    void follow();
  }

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
        if (reply.items.length) void follow();
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
    const timer = setInterval(() => void poll(), POLL_MS);
    const tickAge = setInterval(() => (now = Date.now()), 10_000);
    return () => {
      clearInterval(timer);
      clearInterval(tickAge);
    };
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

<section class="transcript" aria-label="Transcript">
  <div class="feed" bind:this={list} onscroll={() => (pinned = atEnd())} tabindex="-1">
    {#if phase === "loading"}
      <p class="status" role="status">Reading this seat's transcript.</p>
    {:else if rows.length === 0}
      <p class="status" role="status">No messages yet. Verbose has the raw screen.</p>
    {/if}
    {#each rows as row (row.key)}
      {#if row.kind === "prompt"}
        <div class="turn me"><span class="who">You</span><p class="plain">{row.text}</p></div>
      {:else if row.kind === "reply"}
        <div class="turn"><Markdown source={row.text} onlink={open} />{#if row.maybePeer}<p class="maybe">May answer another seat's message.</p>{/if}</div>
      {:else if row.kind === "tool"}
        <div class="step" data-failed={row.ok === false}>
          <p class="line">{row.summary}{#if row.ok === false}<span class="bad">failed</span>{/if}</p>
          {#if row.lines.length}<pre>{row.lines.join("\n")}</pre>{/if}
          {#if row.hidden}<p class="more">{row.hidden} more {row.hidden === 1 ? "line" : "lines"}. Verbose shows them.</p>{/if}
          {#if row.cut}<p class="more">The host cut this output.</p>{/if}
          {#if row.maybePeer}<p class="more">May answer another seat's message.</p>{/if}
        </div>
      {:else}
        <p class="note">{row.text}</p>
      {/if}
    {/each}
  </div>
  {#if !pinned}
    <button type="button" class="jump" onclick={jump}><span aria-hidden="true">↓</span> Jump to latest</button>
  {/if}
  <p class="source" role="status">{stale ? "Not updating. The host did not answer." : phase === "ready" ? `From the transcript, ${age}.` : ""}</p>
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
  .more, .turn .maybe { margin: 4px 0 0; font: 400 12px var(--font-body); color: var(--muted); }
  .note, .status { margin: 0; font: 400 13px var(--font-body); color: var(--muted); }
  .source { flex: none; margin: 0; padding: 6px 16px; border-top: 1px solid var(--line); font: 400 12px var(--font-body); color: var(--muted); min-height: 30px; }
  .jump { position: absolute; right: 12px; bottom: 44px; min-height: 44px; display: inline-flex; align-items: center; gap: 8px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--brand); background: var(--surface); color: var(--text); font: 600 14px var(--font-body); box-shadow: 0 6px 24px #000a; }
  .jump:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .turn :global(p) { margin: 0 0 8px; }
  .turn :global(p:last-child) { margin-bottom: 0; }
  .turn :global(pre) { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
