<script lang="ts">
  import { onMount } from "svelte";
  import { app, openByPerson, openSeat, waitingSeats } from "../lib/app.svelte";
  import { detectCue, SETTLE_MS } from "../lib/attention";
  import { TOAST_MS, toastFor } from "../lib/toast";

  // A phone or any touch screen has no hover, so a seat that starts waiting says so here.
  let touch = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px), (pointer: coarse)").matches);
  let visible = $state(false);
  let known: Set<string> | null = null;
  let attachedFor: string | null = null;
  let settleUntil = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  onMount(() => {
    const query = matchMedia("(max-width: 720px), (pointer: coarse)");
    const read = () => (touch = query.matches);
    read();
    query.addEventListener("change", read);
    return () => {
      query.removeEventListener("change", read);
      clearTimeout(timer);
    };
  });

  function arm(): void {
    clearTimeout(timer);
    timer = setTimeout(() => (visible = false), TOAST_MS);
  }

  // Asks replay on attach, so the first moments after connecting are a baseline, not news.
  $effect(() => {
    const ids = waitingSeats().map((seat) => seat.sessionId);
    const open = openSeat();
    if (app.attachedHostId !== attachedFor) {
      attachedFor = app.attachedHostId;
      known = null;
      settleUntil = performance.now() + SETTLE_MS;
    }
    const settling = known === null || performance.now() < settleUntil;
    const result = detectCue(settling ? null : known, ids, open);
    known = result.known;
    if (result.fire && touch) {
      visible = true;
      arm();
    }
  });

  const toast = $derived(visible && touch ? toastFor(waitingSeats(), openSeat()) : null);

  function openFirst(): void {
    // Read before hiding: hiding empties the toast.
    const id = toast?.sessionId;
    if (!id) return;
    visible = false;
    clearTimeout(timer);
    openByPerson(id);
  }
</script>

<!-- Present from the first frame, since a status inserted with its text is not always announced. -->
<div class="region" role="status" aria-live="polite" onfocusin={() => clearTimeout(timer)} onfocusout={arm}>
  {#if toast}
    <button type="button" class="toast" onclick={openFirst}>
      <span class="text">{toast.text}</span>
      <span class="hint" aria-hidden="true">Tap to open</span>
    </button>
  {/if}
</div>

<style>
  /* At the top, under the seat bar, so it never covers the seats, the composer or the soft keyboard. */
  .region { position: fixed; z-index: 60; top: calc(env(safe-area-inset-top, 0px) + 64px); left: 12px; right: 12px; display: flex; justify-content: center; pointer-events: none; }
  .toast { pointer-events: auto; min-height: 48px; max-width: 100%; display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 8px 16px; border-radius: 10px; border: 1px solid var(--brand); background: var(--surface); color: var(--text); text-align: left; font-family: var(--font-display); box-shadow: 0 6px 24px #000a; }
  .text { font-weight: 600; font-size: 15px; }
  .hint { font-size: 12px; color: var(--muted); }
  .toast:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
