<script lang="ts">
  import Creature from "./Creature.svelte";
  import { gist, stateLabel } from "../lib/messages";
  import type { PeerMessage } from "../lib/protocol";

  let { messages, selfRole, colorOf }: { messages: PeerMessage[]; selfRole: string; colorOf: (role: string) => string } = $props();

  // Opening a row only reads it. Nothing here answers a message or clears a waiting cue.
  let open = $state<Record<string, boolean>>({});
</script>

<div class="messages">
  {#if messages.length === 0}
    <p class="empty">No other seat has written to this one yet. Messages they send show up here and in the terminal.</p>
  {:else}
    <ol>
      {#each messages as message (message.id)}
        {@const outgoing = message.from.role === selfRole}
        {@const expanded = open[message.id] === true}
        {@const preview = gist(message.text)}
        <li data-state={message.state}>
          <button type="button" class="row" aria-expanded={expanded} aria-controls={`message-${message.id}`} onclick={() => (open[message.id] = !expanded)}>
            <span class="meta">
              <Creature role={outgoing ? message.target : message.from.role} color={colorOf(outgoing ? message.target : message.from.role)} size={24} />
              <strong>{outgoing ? "this seat" : message.from.identity}</strong>
              <span class="dir">{outgoing ? `to ${message.target}` : "to this seat"}</span>
              <span class="state">{stateLabel[message.state]}</span>
              <svg class="chevron" class:turned={expanded} viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg>
            </span>
            {#if preview && !expanded}<span class="gist">{preview}</span>{/if}
          </button>
          {#if message.reason}
            <p class="hint">{message.reason}</p>
          {/if}
          <div id={`message-${message.id}`} class="full" hidden={!expanded}>
            {#if message.text}
              <!-- Focusable so a long message scrolls by keyboard. -->
              <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
              <pre class="text" role="region" tabindex="0" aria-label={`Full text from ${outgoing ? "this seat" : message.from.identity}`}>{message.text}</pre>
              {#if message.textCut}<p class="hint">The host sent only the first 32 KiB of this message.</p>{/if}
            {:else}
              <p class="hint">This host did not send the text of this message, so only its delivery shows here.</p>
            {/if}
          </div>
        </li>
      {/each}
    </ol>
  {/if}
  <p class="footnote">The daemon stamps other seats' messages. Anything you type in the terminal is yours and carries no stamp.</p>
</div>

<style>
  .messages { flex: 1; display: flex; flex-direction: column; gap: 12px; min-width: 0; }
  ol { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  li { padding: 12px; border-radius: 10px; background: var(--surface); border: 1px solid var(--line); min-width: 0; }
  li[data-state="queued"], li[data-state="held"], li[data-state="launching"] { border-color: #4a3d25; background: var(--warn-fill); }
  li[data-state="failed"] { border-color: #5a3a41; background: var(--danger-fill); }
  .row { display: block; width: 100%; min-height: 44px; margin: -12px; padding: 12px; box-sizing: content-box; border: 0; border-radius: 10px; background: none; color: inherit; font: inherit; text-align: left; cursor: pointer; }
  .row:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .meta { display: flex; align-items: center; gap: 8px; font-size: 13px; flex-wrap: wrap; }
  .dir { color: var(--muted); }
  .state { margin-left: auto; color: var(--ok); }
  [data-state="queued"] .state, [data-state="held"] .state, [data-state="launching"] .state { color: var(--warn-text); }
  [data-state="failed"] .state { color: var(--danger-text); }
  .chevron { flex: none; color: var(--muted); transition: transform 0.15s; }
  .chevron.turned { transform: rotate(180deg); }
  .gist { display: block; margin-top: 8px; font-size: 13px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hint, .empty, .footnote { color: var(--muted); font-size: 13px; }
  .hint { margin: 6px 0 0; }
  .empty { margin: 0; }
  .footnote { margin: auto 0 0; padding-top: 12px; border-top: 1px solid var(--line); }
  .full { margin-top: 12px; }
  .full[hidden] { display: none; }
  .text { margin: 0; padding: 10px; max-height: min(40vh, 320px); overflow: auto; border-radius: 8px; background: var(--terminal); color: var(--text); font: 13px/1.5 var(--font-body); white-space: pre-wrap; overflow-wrap: anywhere; }
  .text:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
