<script lang="ts">
  import Creature from "./Creature.svelte";
  import { stateLabel } from "../lib/messages";
  import type { PeerMessage } from "../lib/protocol";

  let { messages, selfRole, colorOf }: { messages: PeerMessage[]; selfRole: string; colorOf: (role: string) => string } = $props();
</script>

<aside class="panel" aria-labelledby="messages-heading">
  <h2 id="messages-heading">Messages</h2>
  {#if messages.length === 0}
    <p class="empty">No other seat has written to this one yet. Messages they send show up here and in the terminal.</p>
  {:else}
    <ol>
      {#each messages as message (message.id)}
        {@const outgoing = message.from.role === selfRole}
        <li data-state={message.state}>
          <div class="meta">
            <Creature role={outgoing ? message.to.role : message.from.role} color={colorOf(outgoing ? message.to.role : message.from.role)} size={24} />
            <strong>{outgoing ? "this seat" : message.from.identity}</strong>
            <span class="dir">{outgoing ? `to ${message.to.identity ?? message.to.role}` : "to this seat"}</span>
            <span class="state">{stateLabel[message.state]}</span>
          </div>
          <p>{message.body.replaceAll("\r\n", " ")}</p>
          {#if message.state === "queued"}
            <p class="hint">Lands when {message.to.identity ?? message.to.role}'s turn ends.</p>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
  <p class="footnote">Anything you type in the terminal is yours and carries no envelope. Only other seats' messages are stamped.</p>
</aside>

<style>
  .panel { display: flex; flex-direction: column; gap: 12px; padding: 16px; border-left: 1px solid var(--line); overflow-y: auto; }
  h2 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 18px; }
  ol { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  li { padding: 12px; border-radius: 10px; background: var(--surface); border: 1px solid var(--line); }
  li[data-state="queued"], li[data-state="held"] { border-color: #4a3d25; background: var(--warn-fill); }
  li[data-state="bounced"] { border-color: #5a3a41; background: var(--danger-fill); }
  .meta { display: flex; align-items: center; gap: 8px; font-size: 13px; flex-wrap: wrap; }
  .dir { color: var(--muted); }
  .state { margin-left: auto; color: var(--ok); }
  [data-state="queued"] .state, [data-state="held"] .state { color: var(--warn-text); }
  [data-state="bounced"] .state { color: var(--danger-text); }
  li p { margin: 6px 0 0; font-size: 14px; color: var(--text-soft); }
  .hint, .empty, .footnote { color: var(--muted); font-size: 13px; }
  .empty { margin: 0; }
  .footnote { margin: auto 0 0; padding-top: 12px; border-top: 1px solid var(--line); }
</style>
