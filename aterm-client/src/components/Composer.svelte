<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { restoreRefused } from "../lib/drafts";
  import { findMention, insertMention, mentionMatches } from "../lib/mentions";
  import type { HostConnection, Session } from "../lib/protocol";
  import PasskeyUnlock from "./PasskeyUnlock.svelte";
  import { typingNotice, type Typing } from "../lib/typing";

  // `sessions` is the list the sidebar holds, so `@` completion adds no daemon verb.
  // `offline` is the host not answering: the draft stays editable and Send waits, since a frame typed into a dead link is lost.
  // `draft` and `ondraft` keep the text outside this component, so a seat change or a remount does not take it. `onsent` says a send left.
  let { connection, session, sessions = [], typing = { allowed: true }, offline = false, draft = "", ondraft, onsent, onfocuschange }: {
    connection: HostConnection;
    session: Session;
    sessions?: readonly Session[];
    typing?: Typing;
    offline?: boolean;
    draft?: string;
    ondraft?: (text: string) => void;
    onsent?: () => void;
    /** Whether the field has focus, for chrome that gives way to the keyboard. */
    onfocuschange?: (focused: boolean) => void;
  } = $props();

  let text = $state(untrack(() => draft));
  $effect(() => ondraft?.(text));
  // Input has no reply, so a lock that follows a send moments later is that send refused, and its text comes back.
  let lastSent: { body: string; at: number } | null = null;
  let kept = $state(false);
  let sentFlash = $state(false);
  let flashTimer: ReturnType<typeof setTimeout> | undefined;
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
      const back = restoreRefused(untrack(() => text), lastSent, Date.now());
      if (back.restored) {
        text = back.draft;
        kept = true;
      }
      lastSent = null;
    } else if (wasLocked) {
      kept = false;
      wasLocked = false;
      unlocked = "Unlocked. You can type again.";
      void tick().then(() => field?.focus());
    }
  });
  // The long hint wraps to four lines at 320px and, since the field sizes to its placeholder, takes the terminal's room. Its keys are no use on a touch screen.
  let narrow = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px)").matches);
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const read = () => (narrow = query.matches);
    read();
    query.addEventListener("change", read);
    return () => {
      query.removeEventListener("change", read);
      onfocuschange?.(false);
    };
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
    if (offline) {
      status = "Not sent. The host is reconnecting, and your message stays here.";
      return;
    }
    if (session.paste) {
      connection.input(session.id, `${PASTE_OPEN}${body}${PASTE_CLOSE}`);
      setTimeout(() => connection.input(session.id, "\r"), 300);
    } else {
      connection.input(session.id, `${body.replace(/\r?\n/g, " ")}\r`);
    }
    lastSent = { body, at: Date.now() };
    text = "";
    caret = 0;
    status = `Sent to ${session.identity}.`;
    sentFlash = true;
    clearTimeout(flashTimer);
    flashTimer = setTimeout(() => (sentFlash = false), 1500);
    onsent?.();
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
    {#if kept}
      <p class="kept" role="status"><strong>That message was not sent.</strong> It is kept as your draft and comes back here once typing is allowed again.</p>
    {/if}
    {#if text}<pre class="draft">{text}</pre>{/if}
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
      onfocus={() => {
        syncCaret();
        onfocuschange?.(true);
      }}
      onblur={() => onfocuschange?.(false)}
      enterkeyhint="send"
      role="combobox"
      aria-autocomplete="list"
      aria-expanded={open}
      aria-controls="composer-mentions"
      aria-activedescendant={open ? `mention-${current}` : undefined}
      rows="1"
      placeholder={narrow ? `Message ${session.identity}` : `Type or dictate to ${session.identity}. Enter sends, Shift+Enter for a new line, @ names a session.`}
      autocomplete="off"
      spellcheck="true"
    ></textarea>
  </div>
  <button class="button primary" type="submit" disabled={!text.trim() || offline}>{offline ? "Waits for host" : sentFlash && !text.trim() ? "Sent" : "Send"}</button>
  <p class="visually-hidden" role="status">{note}</p>
</form>
{/if}

<style>
  .composer { display: flex; gap: 10px; align-items: flex-end; padding: 10px 12px; border-top: 1px solid var(--line); background: var(--ground); }
  /* Above the terminal, which overflows its row when the layout is squeezed. */
  .composer.locked { display: block; position: relative; z-index: 1; }
  .locked p { margin: 0; color: var(--text-soft); }
  .locked p + p { margin-top: 8px; }
  .kept { color: var(--warn-text); }
  .draft { margin: 8px 0 0; padding: 8px 10px; max-height: 6.5em; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; border: 1px solid var(--control-line); border-radius: 8px; background: var(--terminal); color: var(--text-soft); font: 14px/1.4 var(--font-body); }
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
  /* The one input on a desktop window is also the one Kai types in, so it opens at three lines. */
  @media (min-width: 721px) {
    .composer { padding: 12px 16px; }
    textarea { min-height: 96px; max-height: 50vh; font-size: 16px; }
  }
  /* A phone gives the seat's text the screen. Four lines is the most this field takes. */
  @media (max-width: 720px) {
    .composer { gap: 8px; padding: 6px 8px; }
    textarea { padding: 9px 12px; max-height: 112px; }
  }
</style>
