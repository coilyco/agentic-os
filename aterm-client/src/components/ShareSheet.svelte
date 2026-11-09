<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { app, reconnecting, selectSession } from "../lib/app.svelte";
  import { canSend, prepareScreenshots, sendingLine } from "../lib/feedback";
  import { chipLabel } from "../lib/images";
  import { checkShares, discard, share } from "../lib/share.svelte";
  import { sendToSeat } from "../lib/share-send";
  import { typingNotice } from "../lib/typing";

  // What Android's Share menu handed the app: a picture, and which seat gets it.
  let dialog = $state<HTMLDialogElement>();
  let text = $state("");
  let chosen = $state<string | null>(null);
  let urls = $state<string[]>([]);
  let note = $state("");
  let info = $state("");
  let phase = $state<"editing" | "sending">("editing");
  let progress = $state({ done: 0, total: 0 });

  const files = $derived(share.pending?.files ?? []);
  // The connection's feature list is not reactive, but `terminalSupport` leaves "unknown" when the welcome arrives, so reading it first re-runs this then.
  const welcomed = $derived(app.terminalSupport !== "unknown");
  const channel = $derived(welcomed ? app.connection?.images : undefined);
  const seats = $derived(app.sessions.filter((session) => session.state !== "failed"));
  const seat = $derived(seats.find((session) => session.id === chosen));
  // The worst states first: no host, a host that cannot take images, locked typing, no seat, a link that is down.
  const blocker = $derived(
    !app.connection || !welcomed
      ? "Not connected to a host yet. Your screenshot waits here, and Send works once the app connects. If it does not, discard this, connect a host, and share again."
      : reconnecting()
        ? "The host is reconnecting. Send works again when it is back."
        : !channel
          ? "This host cannot take images. It needs a newer aterm daemon."
          : !app.typing.allowed
            ? typingNotice(app.typing)
            : seats.length === 0
              ? "No seat is running on this host. Start one, then share again. The original stays where it came from."
              : "",
  );

  $effect(() => {
    if (!dialog) return;
    if (share.open && !dialog.open) {
      dialog.showModal();
    } else if (!share.open && dialog.open) dialog.close();
  });

  // Each share starts from its own words, and the sheet starts with no stale line from the last one.
  $effect(() => {
    const pending = share.pending;
    untrack(() => {
      if (pending && !text) text = pending.text;
      if (!pending) {
        text = "";
        note = "";
        info = "";
      }
    });
  });

  // The seat on screen is the likely one. With one seat there is no choice to make, and with several the person picks.
  $effect(() => {
    if (!share.open || seats.some((session) => session.id === chosen)) return;
    const onScreen = seats.find((session) => session.id === app.selectedSession);
    untrack(() => (chosen = onScreen?.id ?? (seats.length === 1 ? (seats[0]?.id ?? null) : null)));
  });

  $effect(() => {
    const made = files.map((file) => URL.createObjectURL(file));
    urls = made;
    return () => made.forEach((url) => URL.revokeObjectURL(url));
  });

  onMount(() => {
    const look = () => {
      if (document.visibilityState === "visible") void checkShares();
    };
    look();
    // A share while the app sits in the background brings it to the front, which is a visibility change.
    document.addEventListener("visibilitychange", look);
    return () => document.removeEventListener("visibilitychange", look);
  });

  function remove(index: number): void {
    if (!share.pending || phase === "sending") return;
    share.pending = { ...share.pending, files: share.pending.files.filter((_, at) => at !== index) };
    note = "";
    info = "";
  }

  const ready = $derived(!blocker && seat !== undefined && canSend(text, files.length) && phase === "editing");

  async function send(): Promise<void> {
    const connection = app.connection;
    const target = seat;
    if (!ready || !connection || !channel || !target) return;
    phase = "sending";
    note = "";
    info = "";
    progress = { done: 0, total: files.length };
    try {
      const prepared = await prepareScreenshots(files, 0, channel);
      if (prepared.ready.length < files.length) throw new Error(prepared.refusal ?? "A screenshot could not be made small enough.");
      if (prepared.info) info = prepared.info;
      const line = await sendToSeat(
        channel,
        { input: (id, data) => connection.input(id, data), later: (run, ms) => void setTimeout(run, ms) },
        { id: target.id, seat: target.seat, paste: target.paste },
        text,
        prepared.ready,
        (done, total) => (progress = { done, total }),
      );
      // The host may have started reconnecting while the images went up, and then the paste was lost with the socket.
      if (reconnecting()) throw new Error("The host is reconnecting, so the seat may not have it.");
      const id = target.id;
      discard();
      selectSession(id);
      if (line) app.notice = line;
    } catch (error) {
      note = `Not sent. ${error instanceof Error && error.message ? error.message : "The upload failed."} Your screenshot and words stay here.`;
    } finally {
      phase = "editing";
    }
  }
</script>

<!-- The dialog is the sheet. A click on its dimmed edge, or Escape, drops what it holds. The original stays where it came from. -->
<dialog
  bind:this={dialog}
  class="sheet"
  aria-labelledby="share-title"
  onclose={() => share.open && discard()}
  onclick={(event) => event.target === dialog && phase === "editing" && discard()}
>
  <form method="dialog" class="body" onsubmit={(event) => { event.preventDefault(); void send(); }}>
    <h2 id="share-title">Send to a seat</h2>
    {#if blocker}<p class="refusal" role="alert">{blocker}</p>{/if}
    <!-- Up here so a failed send is seen without scrolling past the seats. -->
    {#if note}<p class="refusal" role="alert">{note}</p>{/if}
    {#if files.length}
      <ul class="chips" aria-label="Screenshots to send">
        {#each files as file, index (file)}
          <li class="chip">
            <img src={urls[index]} alt="" />
            <span class="chip-label">{chipLabel(index, file.size)}</span>
            <button class="chip-remove" type="button" aria-label={`Remove image ${index + 1}`} disabled={phase === "sending"} onclick={() => remove(index)}>
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" focusable="false"><path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
            </button>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="plain">Nothing is left to send. Close this, and share again from the picture.</p>
    {/if}
    {#each share.pending?.notes ?? [] as line (line)}<p class="plain" role="status">{line}</p>{/each}
    {#if seats.length}
      <fieldset class="seats" disabled={phase === "sending"}>
        <legend>Send to</legend>
        {#each seats as session (session.id)}
          <label class="seat">
            <input type="radio" name="share-seat" value={session.id} bind:group={chosen} />
            <span class="seat-name">{session.identity}</span>
            <span class="seat-role">{session.role}</span>
          </label>
        {/each}
      </fieldset>
    {/if}
    <label for="share-words">Words to go with it, if any</label>
    <textarea id="share-words" bind:value={text} rows="3" disabled={phase === "sending"}></textarea>
    <div class="status" role="status" aria-live="polite">{#if phase === "sending"}{sendingLine(progress.done, progress.total)}{:else if info}{info}{/if}</div>
    <div class="actions">
      <button class="button" type="button" disabled={phase === "sending"} onclick={discard}>Discard</button>
      <button class="button primary" type="submit" disabled={!ready}>{phase === "sending" ? "Sending" : seat ? `Send to ${seat.identity}` : "Pick a seat"}</button>
    </div>
  </form>
</dialog>

<style>
  .sheet { padding: 0; border: 1px solid var(--control-line); border-radius: 14px; background: var(--surface); color: var(--text); width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; }
  .sheet::backdrop { background: rgb(0 0 0 / 0.6); }
  .body { display: flex; flex-direction: column; gap: 10px; padding: 16px; }
  h2 { margin: 0; font: 600 20px/1.2 var(--font-display); }
  label { font-size: 15px; color: var(--text); }
  .plain { margin: 0; font-size: 14px; line-height: 1.45; color: var(--text-soft); }
  textarea { width: 100%; min-height: 72px; max-height: 30dvh; resize: vertical; padding: 11px 14px; border-radius: 10px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font: 15px/1.4 var(--font-body); }
  .chips { display: flex; flex-wrap: wrap; gap: 12px; margin: 0; padding: 6px 0 0; list-style: none; }
  .chip { position: relative; }
  .chip img { display: block; width: 88px; height: 88px; object-fit: cover; border-radius: 8px; background: var(--terminal); }
  .chip-label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .chip-remove { position: absolute; top: -6px; right: -6px; width: 28px; height: 28px; display: inline-flex; align-items: center; justify-content: center; padding: 0; border: 1px solid var(--control-line); border-radius: 50%; background: var(--surface); color: var(--text); }
  .chip-remove::after { content: ""; position: absolute; inset: -8px; }
  .seats { margin: 0; padding: 0; border: 0; display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .seats legend { padding: 0 0 6px; font-size: 15px; }
  .seat { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 12px; border: 1px solid var(--control-line); border-radius: 10px; }
  .seat:has(input:checked) { border-color: var(--brand); }
  .seat:has(input:focus-visible) { outline: 2px solid var(--brand); outline-offset: 2px; }
  .seat-name { font-weight: 600; overflow-wrap: anywhere; }
  .seat-role { margin-left: auto; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; text-align: right; }
  .status { min-height: 1.4em; font-size: 14px; color: var(--text-soft); }
  .refusal { margin: 0; padding: 10px 12px; border-radius: 8px; background: var(--danger-fill); color: var(--danger-text); font-size: 14px; line-height: 1.4; }
  /* Send stays in view while the seats and words scroll under it. */
  .actions { position: sticky; bottom: 0; display: flex; justify-content: flex-end; gap: 8px; padding: 10px 0 2px; background: var(--surface); }
  .button:disabled { opacity: 0.5; cursor: not-allowed; }
  /* A phone gets the screen's width and sits at its foot, where the thumb is. */
  @media (max-width: 720px) {
    .sheet { width: 100vw; max-width: 100vw; margin: auto 0 0; border-radius: 14px 14px 0 0; border-width: 1px 0 0; padding-bottom: env(safe-area-inset-bottom); }
    .actions .button { flex: 1; justify-content: center; }
  }
</style>
