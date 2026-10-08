<script lang="ts">
  import { tick } from "svelte";
  import { findMention, insertMention, mentionMatches } from "../lib/mentions";
  import type { HostConnection, Session } from "../lib/protocol";
  import PasskeyUnlock from "./PasskeyUnlock.svelte";
  import { typingNotice, type Typing } from "../lib/typing";

  // `sessions` is the list the sidebar holds, so `@` completion adds no daemon verb.
  let { connection, session, sessions = [], typing = { allowed: true } }: { connection: HostConnection; session: Session; sessions?: readonly Session[]; typing?: Typing } = $props();

  let text = $state("");
  let status = $state("");
  let caret = $state(0);
  let active = $state(0);
  // The `@` whose menu Escape closed. Typing again opens it, so a dismissal is never a lockout.
  let dismissedAt = $state<number | null>(null);
  let field = $state<HTMLTextAreaElement>();
  // The lock lifting is news to say aloud, and the unlock button that held focus is gone.
  let unlocked = $state("");
  let wasLocked = false;
  $effect(() => {
    if (!typing.allowed) {
      wasLocked = true;
      unlocked = "";
    } else if (wasLocked) {
      wasLocked = false;
      unlocked = "Unlocked. You can type again.";
      void tick().then(() => field?.focus());
    }
  });
  const PASTE_OPEN = "\x1b[200~";
  const PASTE_CLOSE = "\x1b[201~";

  const mention = $derived(findMention(text, caret));
  const matches = $derived(mention ? mentionMatches(sessions, mention.query) : []);
  const open = $derived(mention !== null && matches.length > 0 && dismissedAt !== mention.start);
  const current = $derived(Math.min(active, Math.max(matches.length - 1, 0)));
  const note = $derived(
    open ? `${matches.length} live ${matches.length === 1 ? "session matches" : "sessions match"}. Up and down choose, Tab or Enter inserts, Escape closes.` : status,
  );

  // A program in bracketed-paste mode takes the text as one paste, then the
  // Enter a beat later so it submits instead of landing as a newline.
  function send(): void {
    const body = text.trim();
    if (!body) return;
    if (session.paste) {
      connection.input(session.id, `${PASTE_OPEN}${body}${PASTE_CLOSE}`);
      setTimeout(() => connection.input(session.id, "\r"), 300);
    } else {
      connection.input(session.id, `${body.replace(/\r?\n/g, " ")}\r`);
    }
    text = "";
    caret = 0;
    status = `Sent to ${session.identity}.`;
  }

  function syncCaret(): void {
    caret = field?.selectionStart ?? text.length;
  }

  function typed(): void {
    syncCaret();
    active = 0;
    dismissedAt = null;
  }

  async function pick(chosen: Session): Promise<void> {
    if (!mention) return;
    const next = insertMention(text, mention, chosen.id);
    text = next.text;
    caret = next.caret;
    active = 0;
    status = `Inserted ${chosen.id}.`;
    await tick();
    field?.focus();
    field?.setSelectionRange(next.caret, next.caret);
  }

  function keydown(event: KeyboardEvent): void {
    if (event.isComposing) return;
    if (open && mention) {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        active = (current + (event.key === "ArrowDown" ? 1 : matches.length - 1)) % matches.length;
        return;
      }
      if ((event.key === "Tab" && !event.shiftKey) || (event.key === "Enter" && !event.shiftKey)) {
        event.preventDefault();
        const chosen = matches[current];
        if (chosen) void pick(chosen);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        dismissedAt = mention.start;
        return;
      }
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      send();
    }
  }
</script>

<p class="visually-hidden" role="status">{unlocked}</p>
{#if !typing.allowed}
  <div class="composer locked">
    <p role="status"><strong>Read only.</strong> {typingNotice(typing)}</p>
    <PasskeyUnlock />
  </div>
{:else}
<form class="composer" onsubmit={(event) => { event.preventDefault(); send(); }}>
  <div class="field">
    {#if open}
      <ul class="mentions" id="composer-mentions" role="listbox" aria-label="Live sessions">
        {#each matches as match, index (match.id)}
          <!-- The textarea keeps focus and the keyboard drives the list, so the options take no key events. -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            id={`mention-${index}`}
            class="mention"
            class:active={index === current}
            role="option"
            aria-selected={index === current}
            onmousedown={(event) => { event.preventDefault(); void pick(match); }}
          >
            <span class="who">{match.identity}</span>
            <span class="role">{match.role}</span>
            <code class="name">{match.id}</code>
          </li>
        {/each}
      </ul>
    {/if}
    <label for="composer-input" class="visually-hidden">Message {session.identity}</label>
    <textarea
      id="composer-input"
      bind:this={field}
      bind:value={text}
      onkeydown={keydown}
      oninput={typed}
      onkeyup={syncCaret}
      onclick={syncCaret}
      onfocus={syncCaret}
      role="combobox"
      aria-autocomplete="list"
      aria-expanded={open}
      aria-controls="composer-mentions"
      aria-activedescendant={open ? `mention-${current}` : undefined}
      rows="1"
      placeholder={`Type or dictate to ${session.identity}. Enter sends, Shift+Enter for a new line, @ names a session.`}
      autocomplete="off"
      spellcheck="true"
    ></textarea>
  </div>
  <button class="button primary" type="submit" disabled={!text.trim()}>Send</button>
  <p class="visually-hidden" role="status">{note}</p>
</form>
{/if}

<style>
  .composer { display: flex; gap: 10px; align-items: flex-end; padding: 10px 12px; border-top: 1px solid var(--line); background: var(--ground); }
  /* Above the terminal, which overflows its row when the layout is squeezed. */
  .composer.locked { display: block; position: relative; z-index: 1; }
  .locked p { margin: 0; color: var(--text-soft); }
  .field { position: relative; flex: 1; min-width: 0; display: flex; }
  textarea {
    flex: 1; min-width: 0; min-height: 44px; max-height: 40vh; field-sizing: content; resize: none;
    padding: 11px 14px; border-radius: 10px; border: 1px solid var(--control-line);
    background: var(--terminal); color: var(--text); font: 15px/1.4 var(--font-body);
  }
  textarea:focus-visible { border-color: var(--brand); }
  button:disabled { opacity: 0.5; cursor: default; }
  .mentions {
    position: absolute; left: 0; right: 0; bottom: calc(100% + 6px); z-index: 5; margin: 0; padding: 4px; list-style: none;
    max-height: 40vh; overflow-y: auto; border: 1px solid var(--control-line); border-radius: 10px; background: var(--surface);
  }
  .mention { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 10px; padding: 8px 10px; border-radius: 8px; cursor: pointer; color: var(--text-soft); }
  .mention.active { background: var(--brand); color: var(--brand-ink); }
  .who { font-weight: 600; }
  .role { font-size: 13px; color: var(--muted); }
  .name { flex-basis: 100%; font: 13px/1.3 var(--font-mono); overflow-wrap: anywhere; }
  .mention.active .role { color: var(--brand-ink); }
</style>
