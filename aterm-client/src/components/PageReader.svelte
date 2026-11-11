<script lang="ts">
  import { tick } from "svelte";
  import { app, closePage } from "../lib/app.svelte";
  import { fromText, PAGE_SANDBOX, pageDocument, pageRows } from "../lib/artifact-pages";
  import { openExternal } from "../lib/open-external";

  // One of a seat's pages, drawn from the source the seat published it from (COI-3330).
  // claude.ai refuses to be framed, so the real page opens outside, in the device's browser.
  const page = $derived(app.pageOpen ? app.artifacts[app.pageOpen] : undefined);
  const row = $derived(page ? pageRows([page], app.sessions)[0] : undefined);
  const doc = $derived(page?.html ? pageDocument(page.html) : "");
  let dialog = $state<HTMLDivElement>();
  let back = $state<HTMLButtonElement>();
  let failed = $state("");
  let returnTo: Element | null = null;

  // Focus moves in when it opens and back to where it came from when it closes.
  $effect(() => {
    if (app.pageOpen === null) return;
    failed = "";
    returnTo = document.activeElement;
    void tick().then(() => back?.focus());
    return () => {
      if (returnTo instanceof HTMLElement && returnTo.isConnected) returnTo.focus();
    };
  });

  async function openOutside(): Promise<void> {
    if (!page) return;
    const result = await openExternal(page.url);
    failed = result.ok ? "" : result.message;
  }

  // Escape closes from anywhere while it is open, since focus can be out in the browser's own controls.
  function escape(event: KeyboardEvent): void {
    if (event.key !== "Escape" || app.pageOpen === null) return;
    event.preventDefault();
    closePage();
  }

  function keys(event: KeyboardEvent): void {
    if (event.key !== "Tab" || !dialog) return;
    // The page behind stays out of the tab order while this is open.
    const items = [...dialog.querySelectorAll<HTMLElement>("button, iframe, [href]")].filter((item) => !item.hasAttribute("disabled"));
    const first = items[0];
    const last = items[items.length - 1];
    if (!first || !last) return;
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
</script>

<svelte:window onkeydown={escape} />

{#if app.pageOpen !== null}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="reader" role="dialog" aria-modal="true" aria-labelledby="reader-title" tabindex="-1" bind:this={dialog} onkeydown={keys}>
    <header>
      <button type="button" class="back" bind:this={back} onclick={closePage}>Back</button>
      <div class="who">
        <h2 id="reader-title">{row?.title ?? "This page is gone"}</h2>
        {#if row}<p class="from">{fromText(row)}</p>{/if}
      </div>
      {#if row?.waiting}<span class="chip">Waiting</span>{/if}
    </header>

    {#if !page || !row}
      <p class="note" role="status">The seat closed this page, so there is nothing left to show. Go Back to the list.</p>
    {:else}
      <div class="actions">
        {#if row.openable}
          <button type="button" class="primary" onclick={openOutside}>Open on claude.ai</button>
        {:else}
          <p class="note" role="status">That address is not a claude.ai artifact, so it is not offered here.</p>
        {/if}
      </div>
      {#if failed}
        <p class="warn" role="alert">{failed}</p>
        <p class="address">Copy this address instead: <code>{page.url}</code></p>
      {/if}
      <p class="note" role="status">
        {#if row.answerOnPage}
          This page saves answers on claude.ai, and a copy here cannot. <strong>Answer on the page.</strong> Below is the source {row.from} published it from.
        {:else}
          This is the source {row.from} published this page from. Open it on claude.ai to answer or to see anything it saves.
        {/if}
        {#if row.seatEnded} That seat has ended, so nothing here can reach it.{/if}
      </p>
      {#if doc}
        <iframe title={`${row.title}, a copy of its source`} sandbox={PAGE_SANDBOX} srcdoc={doc} referrerpolicy="no-referrer"></iframe>
      {:else}
        <p class="warn" role="status">This page came with no source to show. Open it on claude.ai.</p>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .reader {
    position: fixed; inset: 0; z-index: 70; display: flex; flex-direction: column; gap: 10px; background: var(--ground); color: var(--text);
    padding: calc(env(safe-area-inset-top, 0px) + 8px) calc(env(safe-area-inset-right, 0px) + 12px) calc(env(safe-area-inset-bottom, 0px) + 8px) calc(env(safe-area-inset-left, 0px) + 12px);
  }
  header { display: flex; align-items: center; gap: 10px; }
  .who { flex: 1; min-width: 0; }
  h2 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 18px; line-height: 1.2; overflow-wrap: anywhere; }
  .from { margin: 2px 0 0; font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
  .back, .primary { min-height: 44px; min-width: 44px; padding: 0 16px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .primary { width: 100%; border-color: var(--brand); background: color-mix(in srgb, var(--brand) 18%, var(--surface)); }
  .back:focus-visible, .primary:focus-visible, iframe:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .chip { flex: none; padding: 2px 8px; border-radius: 999px; font: 600 11px var(--font-mono); letter-spacing: 0.04em; text-transform: uppercase; color: var(--brand-ink); background: var(--brand); }
  .actions { display: flex; }
  .note { margin: 0; font-size: 13px; color: var(--muted); }
  .warn { margin: 0; font-size: 14px; color: var(--warn-text); }
  .address { margin: 0; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  code { font-family: var(--font-mono); user-select: all; }
  iframe { flex: 1; min-height: 0; width: 100%; border: 1px solid var(--line); border-radius: 10px; background: #fff; }
</style>
