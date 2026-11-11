<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { restoreRefused } from "../lib/drafts";
  import { ESCAPE, HOLD_MS, INTERRUPT, showStop } from "../lib/stop";
  import { findMention, insertMention, mentionMatches } from "../lib/mentions";
  import type { HostConnection, Session } from "../lib/protocol";
  import Unlock from "./Unlock.svelte";
  import type { Place } from "../lib/build";
  import { typingNotice, type Typing } from "../lib/typing";
  import { anchorAt, dictationAvailable, placeSpeech, startDictation, type Anchor, type Session as Dictation } from "../lib/dictation";
  import { IMAGE_TYPES, attachNote, chipLabel, fitImage, formatSize, pastedImages, pasteSteps, screenImages, withImagePaths } from "../lib/images";

  // `sessions` is the list the sidebar holds, so `@` completion adds no daemon verb.
  // `offline` is the host not answering: the draft stays editable and Send waits, since a frame typed into a dead link is lost.
  // `draft` and `ondraft` keep the text outside this component, so a seat change or a remount does not take it. `onsent` says a send left.
  // `dialogOpen` is a dialog open on the seat: typed text would answer it or dismiss it, so Send waits and the message stays as a draft (COI-3322).
  // `reading` is a phone on its side, where height is scarce: the composer is one row (COI-3371).
  // `saveddraft` reads a newer copy of the words once, for a draft local storage lost (COI-3619).
  // `savedimages` reads the images kept for this draft once, and `onimages` hands over the chips each time they change (COI-3582).
  let { connection, session, sessions = [], typing = { allowed: true }, offline = false, dialogOpen = false, reading = false, draft = "", ondraft, saveddraft, savedimages, onimages, onsent, onfocuschange, place, working = false }: {
    connection: HostConnection;
    session: Session;
    sessions?: readonly Session[];
    typing?: Typing;
    offline?: boolean;
    dialogOpen?: boolean;
    reading?: boolean;
    draft?: string;
    ondraft?: (text: string) => void;
    saveddraft?: () => Promise<string | undefined>;
    savedimages?: () => Promise<File[]>;
    onimages?: (files: File[]) => Promise<"kept" | "too_big" | "off">;
    onsent?: () => void;
    /** Whether the field has focus, for chrome that gives way to the keyboard. */
    onfocuschange?: (focused: boolean) => void;
    /** Where the page unlocks from, which changes what a locked device is told. */
    place?: Place;
    /** The seat is busy, so an empty field offers Stop in Send's place. */
    working?: boolean;
  } = $props();

  let text = $state(untrack(() => draft));
  // Saving waits for the saved words, so an empty box at mount never clears their mirror (COI-3619).
  let wordsRead = $state(!untrack(() => saveddraft));
  $effect(() => {
    if (wordsRead) ondraft?.(text);
  });
  // Input has no reply, so a lock that follows a send moments later is that send refused, and its text comes back.
  let lastSent: { body: string; at: number } | null = null;
  // A refused send takes its images back with its text, since a screenshot may not be one to take twice.
  let lastImages: File[] = [];
  // Chips hold the files until Send. Nothing goes to the host for an image that is never sent.
  let chips = $state<{ id: number; file: File; url: string }[]>([]);
  let nextChip = 0;
  const NOT_KEPT = "These images will not be kept if the page closes. They stay here until you send them.";
  // Saving waits for the read, so an empty box at mount never clears what is about to come back.
  let imagesRead = $state(false);
  onMount(() => {
    const first = text;
    void (saveddraft?.() ?? Promise.resolve(undefined)).then((back) => {
      if (back && text === first) text = back;
      wordsRead = true;
    });
    void (savedimages?.() ?? Promise.resolve<File[]>([])).then((files) => {
      if (files.length) {
        take(files);
        status = `${files.length === 1 ? "An image" : `${files.length} images`} from your draft came back. ${chips.length} waiting.`;
      }
      imagesRead = true;
    });
  });
  $effect(() => {
    const files = chips.map((chip) => chip.file);
    if (!imagesRead) return;
    void onimages?.(files).then((kept) => {
      if (kept !== "kept") imageInfo = NOT_KEPT;
      else if (imageInfo === NOT_KEPT) imageInfo = "";
    });
  });
  // The reason an image was left out or an upload failed. It stays on screen, since a refused paste is otherwise silent.
  let imageNote = $state("");
  // Said, not refused: an image was shrunk to fit, or the seat's harness will not attach one.
  let imageInfo = $state("");
  let resizing = $state(false);
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
  // Touch is how a phone is told, not width: a narrow desktop window keeps Enter to send (COI-3514).
  let touch = $state(typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches);
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const read = () => (narrow = query.matches);
    const coarse = matchMedia("(pointer: coarse)");
    const readTouch = () => (touch = coarse.matches);
    read();
    readTouch();
    query.addEventListener("change", read);
    coarse.addEventListener("change", readTouch);
    return () => {
      coarse.removeEventListener("change", readTouch);
      query.removeEventListener("change", read);
      onfocuschange?.(false);
      for (const chip of chips) URL.revokeObjectURL(chip.url);
    };
  });
  // The mic shows only where the app has a speech recognizer. Speech lands in the draft at the caret and stays editable until Send (COI-3460).
  let canDictate = $state(false);
  let dictating = $state(false);
  let dictation: Dictation | null = null;
  let dictAnchor: Anchor = { before: "", after: "" };
  // Typing mid-dictation takes the keyboard's side: the words so far stay and later speech is dropped.
  let dictDetached = false;
  void dictationAvailable().then((ready) => (canDictate = ready));
  onMount(() => () => void dictation?.cancel());

  function speech(words: string): void {
    if (dictDetached) return;
    const placed = placeSpeech(dictAnchor, words);
    text = placed.text;
    caret = placed.caret;
    void tick().then(() => field?.setSelectionRange(placed.caret, placed.caret));
  }

  async function toggleDictation(): Promise<void> {
    if (dictating) {
      status = "Finishing what you said.";
      await dictation?.stop();
      return;
    }
    const from = field?.selectionStart ?? text.length;
    dictAnchor = anchorAt(text, from, field?.selectionEnd ?? from);
    dictDetached = false;
    imageNote = "";
    status = "";
    try {
      dictating = true;
      dictation = await startDictation({
        onText: (words) => speech(words),
        onEnd: (note) => {
          dictating = false;
          dictation = null;
          status = note ?? "Dictation finished. Edit the text, then send.";
          if (note) imageNote = note;
          void tick().then(() => field?.focus());
        },
      });
      status = "Listening. Tap the mic again to stop.";
    } catch (error) {
      dictating = false;
      imageNote = error instanceof Error ? error.message : "Dictation would not start.";
    }
  }

  const HELD_NOTE = "Not sent. A question is waiting for your answer, and your message stays here until you answer it.";
  const PASTE_OPEN = "\x1b[200~";
  const PASTE_CLOSE = "\x1b[201~";
  const STEP_MS = 200;

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

  async function addFiles(files: File[]): Promise<void> {
    if (files.length === 0) return;
    if (uploading || resizing) {
      imageNote = "Wait for the image in progress, then add more.";
      return;
    }
    const limit = connection.images?.maxBytes ?? 0;
    const screened = screenImages(files, chips.length, connection.images);
    imageNote = screened.refusal ?? "";
    imageInfo = "";
    const ready: File[] = [];
    const shrunk: string[] = [];
    resizing = screened.accepted.some((file) => file.size > limit);
    try {
      for (const file of screened.accepted) {
        if (file.size <= limit) {
          ready.push(file);
          continue;
        }
        const fitted = await fitImage(file, limit);
        if (fitted) {
          ready.push(fitted);
          shrunk.push(`${file.name || "An image"} went from ${formatSize(file.size)} to ${formatSize(fitted.size)}`);
        } else {
          imageNote ||= `${file.name || "That image"} is ${formatSize(file.size)} and could not be shrunk under ${formatSize(limit)}.`;
        }
      }
    } finally {
      resizing = false;
    }
    take(ready);
    if (shrunk.length) imageInfo = `Shrunk to fit the ${formatSize(limit)} limit. ${shrunk.join(". ")}.`;
    if (ready.length) status = `Added ${ready.length === 1 ? "an image" : `${ready.length} images`}. ${chips.length} waiting.`;
  }

  function removeChip(id: number): void {
    if (uploading) return;
    const gone = chips.find((chip) => chip.id === id);
    if (gone) URL.revokeObjectURL(gone.url);
    chips = chips.filter((chip) => chip.id !== id);
    imageNote = "";
    imageInfo = "";
    status = `Removed an image. ${chips.length} waiting.`;
    void tick().then(() => field?.focus());
  }

  function pasted(event: ClipboardEvent): void {
    const images = pastedImages(event.clipboardData);
    if (images.length === 0) return;
    event.preventDefault();
    void addFiles(images);
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
    void addFiles(Array.from(event.dataTransfer?.files ?? []));
  }

  function picked(): void {
    void addFiles(Array.from(picker?.files ?? []));
    if (picker) picker.value = "";
  }

  // A program in bracketed-paste mode takes the text as one paste, then the
  // Enter a beat later so it submits instead of landing as a newline.
  // Images upload first, one after another, and only then does anything reach the seat: a failed upload sends nothing and keeps the draft.
  // A press on a button takes focus from the field, the keyboard closes, the layout moves and the tap is lost (COI-3500).
  const keepFocus = (event: MouseEvent): void => event.preventDefault();

  async function send(): Promise<void> {
    const typed = text.trim();
    if (!typed && chips.length === 0) return;
    if (uploading || resizing) return;
    if (offline) {
      status = "Not sent. The host is reconnecting, and your message stays here.";
      return;
    }
    if (dialogOpen) {
      status = HELD_NOTE;
      return;
    }
    // Speech that lands after a send would bring the sent text back, so a send ends the session first.
    if (dictating) {
      dictDetached = true;
      void dictation?.cancel();
    }
    // The seat on screen may change while an upload is out, and this message still belongs to the one it was written for.
    const target = session;
    const sending = chips;
    const paths: string[] = [];
    if (sending.length > 0) {
      const channel = connection.images;
      if (!channel) {
        imageNote = "Not sent. This host cannot take images. Remove them to send the text.";
        return;
      }
      imageNote = "";
      imageInfo = "";
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
      // A dialog may have opened while the images went up.
      if (dialogOpen) {
        status = HELD_NOTE;
        return;
      }
    }
    if (!typed && paths.length === 0) return;
    if (target.paste) {
      // Each path is its own paste, which is what a harness reads as an attachment (references/image-upload.md).
      const steps = pasteSteps(paths, typed, PASTE_OPEN, PASTE_CLOSE);
      for (const [index, step] of steps.entries()) {
        if (index === 0) connection.input(target.id, step);
        else setTimeout(() => connection.input(target.id, step), index * STEP_MS);
      }
      setTimeout(() => connection.input(target.id, "\r"), (steps.length - 1) * STEP_MS + 300);
    } else {
      // No bracketed paste, so a path is only text. The harness may still open the file.
      connection.input(target.id, `${withImagePaths(typed, paths).replace(/\r?\n/g, " ")}\r`);
    }
    lastSent = { body: typed, at: Date.now() };
    lastImages = sending.map((chip) => chip.file);
    for (const chip of sending) URL.revokeObjectURL(chip.url);
    chips = [];
    imageNote = "";
    imageInfo = (paths.length && attachNote(target.seat)) || "";
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
    if (dictating) {
      dictDetached = true;
      void dictation?.stop();
    }
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
    // On touch the return key is for a newline and the Send button sends.
    if (event.key === "Enter" && !event.shiftKey && !touch) {
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
  data-reading={reading}
  onsubmit={(event) => { event.preventDefault(); void send(); }}
  ondragover={dragOver}
  ondragleave={dragLeave}
  ondrop={dropped}
>
  {#if uploading || resizing || imageNote || imageInfo || chips.length}
    <div class="attachments">
      {#if uploading}
        <p class="image-note" role="status">Uploading image {uploading.done + 1} of {uploading.total}. Nothing is sent until they are all up.</p>
      {:else if resizing}
        <p class="image-note" role="status">Shrinking the image to fit the host's {formatSize(connection.images?.maxBytes ?? 0)} limit.</p>
      {:else if imageNote}
        <p class="image-note warn" role="alert">{imageNote}</p>
      {:else if imageInfo}
        <p class="image-note" role="status">{imageInfo}</p>
      {/if}
      {#if chips.length}
        <ul class="chips" aria-label="Images to send">
          {#each chips as chip, index (chip.id)}
            <li class="chip">
              <img src={chip.url} alt="" />
              <span class="chip-label">{chipLabel(index, chip.file.size)}</span>
              <button class="chip-remove" type="button" aria-label={`Remove image ${index + 1}`} disabled={uploading !== null} onmousedown={keepFocus} onclick={() => removeChip(chip.id)}>
                <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" focusable="false"><path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
  {#if dialogOpen && (text.trim() || chips.length)}
    <p class="hold" role="status">A question is waiting. Answer it, then send. Your message stays here.</p>
  {/if}
  {#if canDictate}
    <!-- Same box as Add image. The field keeps focus on a press, so the keyboard stays and the layout holds still. -->
    <button class="add-image mic" class:listening={dictating} type="button" aria-label={dictating ? "Stop dictating" : "Dictate"} aria-pressed={dictating} title={dictating ? "Stop dictating" : "Dictate"} disabled={offline || uploading !== null} onmousedown={keepFocus} onclick={() => void toggleDictation()}>
      <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true" focusable="false"><rect x="9" y="3.5" width="6" height="11" rx="3" fill="none" stroke="currentColor" stroke-width="1.8" /><path d="M5.5 11.5a6.5 6.5 0 0 0 13 0M12 18v2.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" /></svg>
    </button>
  {/if}
  {#if connection.images}
    <!-- The button is the way a phone adds an image, and the way to one with no clipboard image. -->
    <input bind:this={picker} hidden type="file" accept={IMAGE_TYPES.join(",")} multiple onchange={picked} />
    <button class="add-image" type="button" aria-label="Add image" title="Add image" disabled={uploading !== null || resizing || offline} onmousedown={keepFocus} onclick={() => picker?.click()}>
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
      enterkeyhint={touch ? "enter" : "send"}
      aria-autocomplete="list"
      aria-controls={open ? "composer-mentions" : undefined}
      aria-activedescendant={open ? `mention-${current}` : undefined}
      rows="1"
      placeholder={narrow || reading ? `Message ${session.identity}` : `Type or dictate to ${session.identity}. Enter sends, Shift+Enter for a new line, @ names a session.${connection.images ? " Paste or drop an image to attach it." : ""}`}
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
    <button class="button primary" type="submit" onmousedown={keepFocus} disabled={(!text.trim() && chips.length === 0) || offline || dialogOpen || uploading !== null || resizing}>{offline ? "Waits for host" : dialogOpen && (text.trim() || chips.length) ? "Waits for answer" : uploading ? "Uploading" : sentFlash && !text.trim() && chips.length === 0 ? "Sent" : "Send"}</button>
  {/if}
  <p class="visually-hidden" role="status">{note}</p>
</form>
{/if}

<style>
  .composer { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; padding: 10px 12px; border-top: 1px solid var(--line); background: var(--ground); }
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
  .add-image.listening, .add-image.listening:hover:not(:disabled) { color: var(--brand-ink); background: var(--brand); border-color: var(--brand); }
  .add-image:focus-visible, .chip-remove:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .attachments { flex: 0 0 100%; min-width: 0; }
  .hold { flex: 0 0 100%; margin: 0; font-size: 14px; line-height: 1.4; color: var(--warn-text); }
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
  /* A phone on its side is wider than 720px, so the rules above miss it. Add, field and Send share one 44px row, and the field grows only while it holds text. */
  .composer[data-reading="true"] { gap: 8px; padding: 4px max(8px, env(safe-area-inset-right)) 4px max(8px, env(safe-area-inset-left)); }
  .composer[data-reading="true"] textarea { min-height: 44px; max-height: min(112px, 40vh); padding: 9px 12px; font-size: 15px; }
</style>
