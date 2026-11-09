<script lang="ts">
  import { onMount, tick } from "svelte";
  import { pinnedLine, replyAge, replyKey } from "../lib/last-reply";
  import { parseLine } from "../lib/markdown";
  import { openExternal } from "../lib/open-external";
  import type { HostConnection, Session } from "../lib/protocol";
  import InlineText from "./InlineText.svelte";
  import Markdown from "./Markdown.svelte";

  let { session, connection, hidden = false }: { session: Session; connection: HostConnection; hidden?: boolean } = $props();

  const line = $derived(pinnedLine(session.lastReply));
  let now = $state(Date.now());
  const age = $derived(replyAge(session.lastReplyAt, now));

  let open = $state(false);
  // The whole text, by the reply it belongs to, so a reopen does not ask again.
  let held = $state<{ key: string; text: string; truncated: boolean } | undefined>();
  let failed = $state(false);
  let linkNote = $state("");
  let pin = $state<HTMLButtonElement>();
  let close = $state<HTMLButtonElement>();

  const key = $derived(replyKey(session.id, session.lastReplyAt, line));
  const whole = $derived(held?.key === key ? held : undefined);

  onMount(() => {
    const tickAge = setInterval(() => (now = Date.now()), 30_000);
    return () => clearInterval(tickAge);
  });

  async function show(): Promise<void> {
    open = true;
    failed = false;
    await tick();
    close?.focus();
    const channel = connection.lastReplies;
    if (!channel || whole) return;
    const asked = key;
    try {
      const reply = await channel.fetch(session.id);
      if (reply.text) held = { key: asked, text: reply.text, truncated: reply.truncated };
      else failed = true;
    } catch {
      failed = true;
    }
  }

  async function follow(href: string): Promise<void> {
    const opened = await openExternal(href);
    linkNote = opened.ok ? "" : opened.message;
  }

  async function hide(): Promise<void> {
    open = false;
    linkNote = "";
    await tick();
    pin?.focus();
  }
</script>

<svelte:window onkeydown={(event) => open && event.key === "Escape" && void hide()} />

{#if line && session.lastReplyAt !== undefined && !hidden}
  <button type="button" class="pin" bind:this={pin} onclick={show} aria-haspopup="dialog" aria-label={`Last reply from ${session.identity}${age ? `, ${age}` : ""}. Opens the whole reply.`}>
    <span class="label">Last reply{#if age}<span class="age">{age}</span>{/if}</span>
    <span class="line"><InlineText nodes={parseLine(line)} /></span>
  </button>
{/if}

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="scrim" onclick={() => void hide()}></div>
  <div class="sheet" role="dialog" aria-modal="true" aria-label={`Last reply from ${session.identity}`}>
    <header>
      <h2>Last reply{#if age}<span class="age">{age}</span>{/if}</h2>
      <button type="button" class="close" bind:this={close} onclick={() => void hide()}>Close</button>
    </header>
    <div class="body">
      {#if whole}
        <!-- Focusable so a long reply scrolls by keyboard. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div class="text" tabindex="0" role="region" aria-label="Full text of the last reply"><Markdown source={whole.text} onlink={(href) => void follow(href)} /></div>
        {#if whole.truncated}<p class="note">The host sent only the first 8 KiB of this reply.</p>{/if}
      {:else}
        <p class="text first"><InlineText nodes={parseLine(line)} /></p>
        <p class="note" role="status">{failed ? "The host would not send the whole reply, so only its first line shows." : connection.lastReplies ? "Loading the whole reply." : "This host sends only the first line of a reply."}</p>
      {/if}
      {#if linkNote}<p class="note" role="status">{linkNote}</p>{/if}
    </div>
  </div>
{/if}

<style>
  .pin { flex: none; display: flex; flex-direction: column; gap: 2px; width: 100%; min-height: 44px; margin: 0; padding: 6px 12px; box-sizing: border-box; text-align: left; border: 0; border-top: 1px solid var(--line); background: var(--surface); color: var(--text); font: inherit; cursor: pointer; }
  .pin:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
  .label { display: flex; gap: 8px; font: 600 11px var(--font-body); letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
  .age { font-weight: 500; letter-spacing: 0; text-transform: none; }
  .line { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; font-size: 14px; line-height: 1.35; overflow-wrap: anywhere; }
  .scrim { position: fixed; inset: 0; z-index: 40; background: rgb(0 0 0 / 0.5); }
  .sheet { position: fixed; z-index: 41; left: env(safe-area-inset-left, 0px); right: env(safe-area-inset-right, 0px); bottom: 0; display: flex; flex-direction: column; max-height: 78dvh; background: var(--ground); border-top: 1px solid var(--line); border-radius: 14px 14px 0 0; box-shadow: 0 -12px 24px rgb(0 0 0 / 0.4); padding-bottom: var(--bottom-clear, env(safe-area-inset-bottom, 0px)); }
  header { flex: none; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 12px 4px 16px; border-bottom: 1px solid var(--line); }
  h2 { display: flex; align-items: baseline; gap: 8px; margin: 0; font: 600 16px var(--font-display); }
  h2 .age { font: 400 13px var(--font-body); color: var(--muted); }
  .close { min-width: 44px; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .close:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .body { flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 12px 16px; }
  .text { margin: 0; font: 14px/1.5 var(--font-body); overflow-wrap: anywhere; }
  .text:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .note { margin: 10px 0 0; font-size: 13px; color: var(--muted); }
</style>
