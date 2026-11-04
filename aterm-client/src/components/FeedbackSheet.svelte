<script lang="ts">
  import { untrack } from "svelte";
  import { app } from "../lib/app.svelte";
  import { FEEDBACK_TEXT_MAX, canSend, contextLine, feedbackRefusal, prepareScreenshots, sendingLine } from "../lib/feedback";
  import { sheet } from "../lib/feedback.svelte";
  import { IMAGE_TYPES, MAX_IMAGES, chipLabel, pastedImages } from "../lib/images";

  let dialog = $state<HTMLDialogElement>();
  let picker = $state<HTMLInputElement>();
  let field = $state<HTMLTextAreaElement>();
  // The note and screenshots outlive the sheet, so closing it by accident loses nothing.
  let text = $state("");
  let chips = $state<{ id: number; file: File; url: string }[]>([]);
  let nextChip = 0;
  let note = $state("");
  let info = $state("");
  let phase = $state<"editing" | "sending" | "sent">("editing");
  let progress = $state({ done: 0, total: 0 });
  let record = $state("");
  let preparing = $state(false);

  const channel = $derived(app.connection?.feedback);
  const context = $derived(sheet.context);

  // The native dialog traps focus, answers Escape, and returns focus to the button that opened it.
  $effect(() => {
    if (!dialog) return;
    if (sheet.open && !dialog.open) {
      dialog.showModal();
      untrack(() => field?.focus());
    } else if (!sheet.open && dialog.open) dialog.close();
  });

  function dismissed(): void {
    sheet.open = false;
    if (phase === "sent") phase = "editing";
  }

  async function addFiles(files: File[]): Promise<void> {
    if (files.length === 0 || !channel) return;
    preparing = true;
    const prepared = await prepareScreenshots(files, chips.length, channel);
    preparing = false;
    note = prepared.refusal ?? "";
    info = prepared.info ?? "";
    for (const file of prepared.ready) chips.push({ id: nextChip++, file, url: URL.createObjectURL(file) });
  }

  function removeChip(id: number): void {
    const gone = chips.find((chip) => chip.id === id);
    if (gone) URL.revokeObjectURL(gone.url);
    chips = chips.filter((chip) => chip.id !== id);
    note = "";
    info = "";
  }

  function pasted(event: ClipboardEvent): void {
    const images = pastedImages(event.clipboardData);
    if (images.length === 0) return;
    event.preventDefault();
    void addFiles(images);
  }

  function dropped(event: DragEvent): void {
    if (!Array.from(event.dataTransfer?.types ?? []).includes("Files")) return;
    event.preventDefault();
    void addFiles(Array.from(event.dataTransfer?.files ?? []));
  }

  function picked(): void {
    void addFiles(Array.from(picker?.files ?? []));
    if (picker) picker.value = "";
  }

  async function send(): Promise<void> {
    if (!channel || !context || phase === "sending" || !canSend(text, chips.length)) return;
    phase = "sending";
    note = "";
    progress = { done: 0, total: chips.length };
    try {
      record = await channel.send({ text: text.trim(), context, images: chips.map((chip) => chip.file) }, (done, total) => (progress = { done, total }));
      for (const chip of chips) URL.revokeObjectURL(chip.url);
      chips = [];
      text = "";
      info = "";
      phase = "sent";
    } catch (error) {
      phase = "editing";
      note = `Not sent. ${error instanceof Error && error.message ? error.message : "The host did not answer."} Your note and screenshots stay here.`;
    }
  }

  function again(): void {
    phase = "editing";
    record = "";
    untrack(() => field?.focus());
  }
</script>

<!-- The dialog is the sheet. A click on its dimmed edge is a click on the dialog itself, which puts it away. -->
<dialog
  bind:this={dialog}
  class="sheet"
  aria-labelledby="feedback-title"
  onclose={dismissed}
  onclick={(event) => event.target === dialog && (sheet.open = false)}
  ondragover={(event) => event.preventDefault()}
  ondrop={dropped}
>
  <form method="dialog" class="body" onsubmit={(event) => { event.preventDefault(); void send(); }}>
    {#if phase === "sent"}
      <h2 id="feedback-title">Sent</h2>
      <p class="plain" role="status">A frontend seat on the host will pick it up and file it. The record is <code class="mono">{record}</code>.</p>
      <div class="actions">
        <button class="button" type="button" onclick={again}>Send another</button>
        <button class="button primary" type="button" onclick={() => (sheet.open = false)}>Done</button>
      </div>
    {:else}
      <h2 id="feedback-title">Send feedback</h2>
      {#if context}<p class="sent-with mono"><span class="visually-hidden">Sent along with it: </span>{contextLine(context)}</p>{/if}
      <label for="feedback-note">What should change on this screen?</label>
      <textarea
        id="feedback-note"
        bind:this={field}
        bind:value={text}
        maxlength={FEEDBACK_TEXT_MAX}
        rows="4"
        disabled={phase === "sending"}
        onpaste={pasted}
        aria-describedby="feedback-help"
      ></textarea>
      <p id="feedback-help" class="help">Add a screenshot of it, up to {MAX_IMAGES}. On a phone, the picker lists your newest screenshots first.</p>
      {#if chips.length}
        <ul class="chips" aria-label="Screenshots to send">
          {#each chips as chip, index (chip.id)}
            <li class="chip">
              <img src={chip.url} alt="" />
              <span class="chip-label">{chipLabel(index, chip.file.size)}</span>
              <button class="chip-remove" type="button" aria-label={`Remove screenshot ${index + 1}`} disabled={phase === "sending"} onclick={() => removeChip(chip.id)}>
                <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" focusable="false"><path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      <input bind:this={picker} hidden type="file" accept={IMAGE_TYPES.join(",")} multiple onchange={picked} />
      <button class="button add" type="button" disabled={phase === "sending" || preparing || chips.length >= MAX_IMAGES || !channel} onclick={() => picker?.click()}>Add screenshot</button>
      <div class="status" role="status" aria-live="polite">
        {#if phase === "sending"}{sendingLine(progress.done, progress.total)}{:else if preparing}Getting your screenshot ready.{:else if info}{info}{/if}
      </div>
      {#if note}<p class="refusal" role="alert">{note}</p>{/if}
      {#if !channel}<p class="refusal" role="alert">{app.connection ? "This host needs a newer aterm daemon to take feedback." : "Connect to a host to send feedback."}</p>{/if}
      <div class="actions">
        <button class="button" type="button" onclick={() => (sheet.open = false)}>Close</button>
        <button class="button primary" type="submit" disabled={!channel || phase === "sending" || preparing || !canSend(text, chips.length)}>{phase === "sending" ? "Sending" : "Send"}</button>
      </div>
    {/if}
  </form>
</dialog>

<style>
  .sheet { padding: 0; border: 1px solid var(--control-line); border-radius: 14px; background: var(--surface); color: var(--text); width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; }
  .sheet::backdrop { background: rgb(0 0 0 / 0.6); }
  .body { display: flex; flex-direction: column; gap: 10px; padding: 16px; }
  h2 { margin: 0; font: 600 20px/1.2 var(--font-display); }
  label { font-size: 15px; color: var(--text); }
  .sent-with { margin: 0; font-size: 12px; line-height: 1.4; color: var(--muted); overflow-wrap: anywhere; }
  .plain { margin: 0; font-size: 15px; line-height: 1.45; color: var(--text-soft); }
  .help { margin: 0; font-size: 13px; line-height: 1.4; color: var(--muted); }
  textarea { width: 100%; min-height: 96px; max-height: 40dvh; resize: vertical; padding: 11px 14px; border-radius: 10px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font: 15px/1.4 var(--font-body); }
  .chips { display: flex; flex-wrap: wrap; gap: 12px; margin: 0; padding: 6px 0 0; list-style: none; }
  .chip { position: relative; }
  .chip img { display: block; width: 56px; height: 56px; object-fit: cover; border-radius: 6px; background: var(--terminal); }
  .chip-label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .chip-remove { position: absolute; top: -6px; right: -6px; width: 28px; height: 28px; display: inline-flex; align-items: center; justify-content: center; padding: 0; border: 1px solid var(--control-line); border-radius: 50%; background: var(--surface); color: var(--text); }
  .chip-remove::after { content: ""; position: absolute; inset: -8px; }
  .add { align-self: flex-start; }
  .status { min-height: 1.4em; font-size: 14px; color: var(--text-soft); }
  .refusal { margin: 0; padding: 10px 12px; border-radius: 8px; background: var(--danger-fill); color: var(--danger-text); font-size: 14px; line-height: 1.4; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }
  .button:disabled { opacity: 0.5; cursor: not-allowed; }
  /* A phone gets the screen's width and sits at its foot, where the thumb is. */
  @media (max-width: 720px) {
    .sheet { width: 100vw; max-width: 100vw; margin: auto 0 0; border-radius: 14px 14px 0 0; border-width: 1px 0 0; padding-bottom: env(safe-area-inset-bottom); }
    .actions .button { flex: 1; justify-content: center; }
  }
</style>
