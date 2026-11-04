<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { restoreRefused } from "../lib/drafts";
  import { ESCAPE, HOLD_MS, INTERRUPT, showStop } from "../lib/stop";
  import { findMention, insertMention, mentionMatches } from "../lib/mentions";
  import type { HostConnection, Session } from "../lib/protocol";
  import Unlock from "./Unlock.svelte";
  import type { Place } from "../lib/build";
  import { typingNotice, type Typing } from "../lib/typing";
  import { IMAGE_TYPES, chipLabel, pastedImages, screenImages, withImagePaths } from "../lib/images";

  // `sessions` is the list the sidebar holds, so `@` completion adds no daemon verb.
  // `offline` is the host not answering: the draft stays editable and Send waits, since a frame typed into a dead link is lost.
  // `draft` and `ondraft` keep the text outside this component, so a seat change or a remount does not take it. `onsent` says a send left.
  let { connection, session, sessions = [], typing = { allowed: true }, offline = false, draft = "", ondraft, onsent, onfocuschange, place, working = false }: {
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
    /** Where the page unlocks from, which changes what a locked device is told. */
    place?: Place;
    /** The seat is busy, so an empty field offers Stop in Send's place. */
    working?: boolean;
  } = $props();

  let text = $state(untrack(() => draft));
  $effect(() => ondraft?.(text));
  // Input has no reply, so a lock that follows a send moments later is that send refused, and its text comes back.
  let lastSent: { body: string; at: number } | null = null;
  // A refused send takes its images back with its text, since a screenshot may not be one to take twice.
  let lastImages: File[] = [];
  // Chips hold the files until Send. Nothing goes to the host for an image that is never sent.
  let chips = $state<{ id: number; file: File; url: string }[]>([]);
  let nextChip = 0;
  // The reason an image was left out or an upload failed. It stays on screen, since a refused paste is otherwise silent.
  let imageNote = $state("");
  let uploading = $state<{ done: number; total: number } | null>(null);
  let dropping = $state(false);
  let picker = $state<HTMLInputElement>();
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
        if (lastImages.length) untrack(() => take(lastImages));
      }
      lastImages = [];
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
      for (const chip of chips) URL.revokeObjectURL(chip.url);
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

  function take(files: File[]): void {
    for (const file of files) chips.push({ id: nextChip++, file, url: URL.createObjectURL(file) });
  }

  function addFiles(files: File[]): void {
    if (files.length === 0) return;
    if (uploading) {
      imageNote = "An upload is out. Add more once it finishes.";
      return;
    }
    const screened = screenImages(files, chips.length, connection.images);
    imageNote = screened.refusal ?? "";
    take(screened.accepted);
    if (screened.accepted.length) status = `Added ${screened.accepted.length === 1 ? "an image" : `${screened.accepted.length} images`}. ${chips.length} waiting.`;
  }

  function removeChip(id: number): void {
    if (uploading) return;
    const gone = chips.find((chip) => chip.id === id);
    if (gone) URL.revokeObjectURL(gone.url);
    chips = chips.filter((chip) => chip.id !== id);
    imageNote = "";
    status = `Removed an image. ${chips.length} waiting.`;
    void tick().then(() => field?.focus());
  }

  function pasted(event: ClipboardEvent): void {
    const images = pastedImages(event.clipboardData);
    if (images.length === 0) return;
    event.preventDefault();
    addFiles(images);
  }

  function carriesFiles(event: DragEvent): boolean {
    return Array.from(event.dataTransfer?.types ?? []).includes("Files");
  }

  function dragOver(event: DragEvent): void {
    if (!carriesFiles(event)) return;
    event.preventDefault();
    dropping = true;
  }

  function dragLeave(event: DragEvent): void {
    // Moving over a child fires a leave on the form too, so only leaving the form ends the cue.
    if (!(event.currentTarget as Node).contains(event.relatedTarget as Node | null)) dropping = false;
  }

  function dropped(event: DragEvent): void {
    if (!carriesFiles(event)) return;
    event.preventDefault();
    dropping = false;
    addFiles(Array.from(event.dataTransfer?.files ?? []));
  }

  function picked(): void {
    addFiles(Array.from(picker?.files ?? []));
    if (picker) picker.value = "";
  }

  // A program in bracketed-paste mode takes the text as one paste, then the
  // Enter a beat later so it submits instead of landing as a newline.
  // Images upload first, one after another, and only then does anything reach the seat: a failed upload sends nothing and keeps the draft.
  async function send(): Promise<void> {
    const typed = text.trim();
    if (!typed && chips.length === 0) return;
    if (uploading) return;
    if (offline) {
      status = "Not sent. The host is reconnecting, and your message stays here.";
      return;
    }
    // The seat on screen may change while an upload is out, and this message still belongs to the one it was written for.
    const target = session;
    const sending = chips;
    let body = typed;
    if (sending.length > 0) {
      const channel = connection.images;
      if (!channel) {
        imageNote = "Not sent. This host cannot take images. Remove them to send the text.";
        return;
      }
      const paths: string[] = [];
      imageNote = "";
      try {
        for (const [index, chip] of sending.entries()) {
          uploading = { done: index, total: sending.length };
          paths.push(await channel.upload(target.id, chip.file));
        }
      } catch (error) {
        imageNote = `Not sent. ${error instanceof Error && error.message ? error.message : "The upload failed."} Your message and images stay here.`;
        return;
      } finally {
        uploading = null;
      }
      if (offline) {
        status = "Not sent. The host is reconnecting, and your message stays here.";
        return;
      }
      body = withImagePaths(typed, paths);
    }
    if (!body) return;
    if (target.paste) {
      connection.input(target.id, `${PASTE_OPEN}${body}${PASTE_CLOSE}`);
      setTimeout(() => connection.input(target.id, "\r"), 300);
    } else {
      connection.input(target.id, `${body.replace(/\r?\n/g, " ")}\r`);
    }
    lastSent = { body: typed, at: Date.now() };
    lastImages = sending.map((chip) => chip.file);
    for (const chip of sending) URL.revokeObjectURL(chip.url);
    chips = [];
    imageNote = "";
    text = "";
    caret = 0;
    status = `Sent to ${target.identity}.`;
    sentFlash = true;
    clearTimeout(flashTimer);
    flashTimer = setTimeout(() => (sentFlash = false), 1500);
    onsent?.();
  }

  const stopping = $derived(showStop({ working, hasText: text.trim() !== "" || chips.length > 0, justSent: sentFlash, offline }));
  let holdTimer: ReturnType<typeof setTimeout> | undefined;
  let held = false;

  function press(key: string, name: string): void {
    connection.input(session.id, key);
    status = `Sent ${name} to ${session.identity}.`;
  }

  function holdStart(): void {
    held = false;
    clearTimeout(holdTimer);
    holdTimer = setTimeout(() => {
      held = true;
      press(INTERRUPT, "Control-C");
    }, HOLD_MS);
  }

  function holdEnd(): void {
    clearTimeout(holdTimer);
  }

  // A hold already sent Control-C, so the click that ends it sends nothing more.
  function tapStop(): void {
    if (held) held = false;
    else press(ESCAPE, "Escape");
  }

  function stopKey(event: KeyboardEvent): void {
    if (event.key !== "Enter" || !event.shiftKey) return;
    event.preventDefault();
    press(INTERRUPT, "Control-C");
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
      void send();
    }
  }
</script>

<p class="visually-hidden" role="status">{unlocked}</p>
{#if !typing.allowed}
  <div class="composer locked">
    <p role="status"><strong>Read only.</strong> {typingNotice(typing, place)}</p>
    {#if kept}
      <p class="kept" role="status"><strong>That message was not sent.</strong> It is kept as your draft and comes back here once typing is allowed again.</p>
    {/if}
    {#if text}<pre class="draft">{text}</pre>{/if}
    <Unlock />
  </div>
{:else}
<form
  class="composer"
  class:dropping
  onsubmit={(event) => { event.preventDefault(); void send(); }}
  ondragover={dragOver}
  ondragleave={dragLeave}
  ondrop={dropped}
>
  {#if uploading || imageNote || chips.length}
    <div class="attachments">
      {#if uploading}
        <p class="image-note" role="status">Uploading image {uploading.done + 1} of {uploading.total}. Nothing is sent until they are all up.</p>
      {:else if imageNote}
        <p class="image-note warn" role="alert">{imageNote}</p>
      {/if}
      {#if chips.length}
        <ul class="chips" aria-label="Images to send">
          {#each chips as chip, index (chip.id)}
            <li class="chip">
              <img src={chip.url} alt="" />
              <span class="chip-label">{chipLabel(index, chip.file.size)}</span>
              <button class="chip-remove" type="button" aria-label={`Remove image ${index + 1}`} disabled={uploading !== null} onclick={() => removeChip(chip.id)}>
                <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" focusable="false"><path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
  {#if connection.images}
    <!-- The button is the way a phone adds an image, and the way to one with no clipboard image. -->
    <input bind:this={picker} hidden type="file" accept={IMAGE_TYPES.join(",")} multiple onchange={picked} />
    <button class="add-image" type="button" aria-label="Add image" title="Add image" disabled={uploading !== null || offline} onclick={() => picker?.click()}>
      <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true" focusable="false"><rect x="3.5" y="4.5" width="17" height="15" rx="2.5" fill="none" stroke="currentColor" stroke-width="1.8" /><circle cx="9" cy="10" r="1.6" fill="currentColor" /><path d="M4.5 17.5l4.5-4.5 3.5 3.5 3-3 4 4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" /></svg>
    </button>
  {/if}
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
    <!-- No combobox role: a dictation tool wants a plain text area (COI-3254). The list is aria-controls and activedescendant. -->
    <textarea
      id="composer-input"
      bind:this={field}
      bind:value={text}
      onkeydown={keydown}
      oninput={typed}
      onkeyup={syncCaret}
      onpaste={pasted}
      readonly={uploading !== null}
      onclick={syncCaret}
      onfocus={() => {
        syncCaret();
        onfocuschange?.(true);
      }}
      onblur={() => onfocuschange?.(false)}
      enterkeyhint="send"
      aria-autocomplete="list"
      aria-controls={open ? "composer-mentions" : undefined}
      aria-activedescendant={open ? `mention-${current}` : undefined}
      rows="1"
      placeholder={narrow ? `Message ${session.identity}` : `Type or dictate to ${session.identity}. Enter sends, Shift+Enter for a new line, @ names a session.${connection.images ? " Paste or drop an image to attach it." : ""}`}
      autocomplete="off"
      spellcheck="true"
    ></textarea>
  </div>
  {#if stopping}
    <!-- Focus stays in the field: a blur closes the keyboard, the layout moves, and the first tap on Stop is lost. -->
    <button
      class="button stop"
      type="button"
      aria-label={`Stop ${session.identity}. Sends Escape. Touch and hold, or Shift and Enter, sends Control-C.`}
      aria-keyshortcuts="Shift+Enter"
      onmousedown={(event) => event.preventDefault()}
      onpointerdown={holdStart}
      onpointerup={holdEnd}
      onpointerleave={holdEnd}
      onpointercancel={holdEnd}
      oncontextmenu={(event) => event.preventDefault()}
      onclick={tapStop}
      onkeydown={stopKey}
    >Stop</button>
  {:else}
    <button class="button primary" type="submit" disabled={(!text.trim() && chips.length === 0) || offline || uploading !== null}>{offline ? "Waits for host" : uploading ? "Uploading" : sentFlash && !text.trim() && chips.length === 0 ? "Sent" : "Send"}</button>
  {/if}
  <p class="visually-hidden" role="status">{note}</p>
</form>
{/if}

<style>
  .composer { display: flex; flex-wrap: wrap; gap: 10px; align-items: flex-end; padding: 10px 12px; border-top: 1px solid var(--line); background: var(--ground); }
  /* Above the terminal, which overflows its row when the layout is squeezed. */
  .composer.locked { display: block; position: relative; z-index: 1; }
  .locked p { margin: 0; color: var(--text-soft); }
  .locked p + p { margin-top: 8px; }
  .kept { color: var(--warn-text); }
  .draft { margin: 8px 0 0; padding: 8px 10px; max-height: 6.5em; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; border: 1px solid var(--control-line); border-radius: 8px; background: var(--terminal); color: var(--text-soft); font: 14px/1.4 var(--font-body); }
  .field { position: relative; flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .composer.dropping { outline: 2px dashed var(--brand); outline-offset: -4px; }
  .add-image { flex: none; width: 44px; height: 44px; display: inline-flex; align-items: center; justify-content: center; padding: 0; border: 1px solid var(--control-line); border-radius: 10px; background: var(--terminal); color: var(--text-soft); cursor: pointer; }
  .add-image:hover:not(:disabled) { color: var(--text); border-color: var(--brand); }
  .add-image:focus-visible, .chip-remove:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .attachments { flex: 0 0 100%; min-width: 0; }
  .image-note { margin: 0 0 8px; font-size: 14px; line-height: 1.4; color: var(--text-soft); }
  .image-note.warn { color: var(--warn-text); }
  .chips { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .chip { display: flex; align-items: center; gap: 8px; max-width: 100%; padding: 4px 4px 4px 4px; border: 1px solid var(--control-line); border-radius: 10px; background: var(--surface); }
  .chip img { flex: none; width: 44px; height: 44px; object-fit: cover; border-radius: 6px; background: var(--terminal); }
  .chip-label { font-size: 13px; color: var(--text-soft); white-space: nowrap; }
  .chip-remove { flex: none; width: 44px; height: 44px; display: inline-flex; align-items: center; justify-content: center; padding: 0; border: 0; border-radius: 8px; background: transparent; color: var(--text-soft); cursor: pointer; }
  .chip-remove:hover:not(:disabled) { color: var(--text); }
  textarea {
    flex: 1; min-width: 0; min-height: 44px; max-height: 40vh; field-sizing: content; resize: none;
    padding: 11px 14px; border-radius: 10px; border: 1px solid var(--control-line);
    background: var(--terminal); color: var(--text); font: 15px/1.4 var(--font-body);
  }
  textarea:focus-visible { border-color: var(--brand); }
  button:disabled { opacity: 0.5; cursor: default; }
  .composer :global(.button) { min-width: 76px; justify-content: center; }
  .stop { border-color: var(--warn); background: var(--warn-fill); color: var(--warn-text); font-weight: 600; touch-action: manipulation; user-select: none; -webkit-touch-callout: none; }
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
    /* Thumbnails only, so four images cost one row. The label stays for a screen reader. */
    .chips { gap: 12px; padding-top: 6px; }
    .chip { position: relative; padding: 0; border: 0; background: none; }
    .chip img { width: 56px; height: 56px; }
    .chip-label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .chip-remove { position: absolute; top: -6px; right: -6px; width: 28px; height: 28px; border: 1px solid var(--control-line); border-radius: 50%; background: var(--surface); color: var(--text); }
    .chip-remove::after { content: ""; position: absolute; inset: -8px; }
    textarea { padding: 9px 12px; max-height: 112px; }
  }
</style>
