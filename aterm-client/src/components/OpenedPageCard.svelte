<script lang="ts">
  import { app, dismissPage, sessionById, showPage } from "../lib/app.svelte";
  import { clearLinkNotice, linkNotice, openLink } from "../lib/link-notice.svelte";
  import { inApp, isArtifactUrl } from "../lib/open-external";
  import { cardText, pageLabel } from "../lib/pages";

  // The newest link leads, since it is the one a seat is waiting on. The rest are counted.
  const page = $derived(app.openedPages.at(-1));
  const seat = $derived(page ? sessionById(page.sessionId) : undefined);
  const others = $derived(Math.max(app.openedPages.length - 1, 0));
  const label = $derived(page ? pageLabel(page.url) : "");
  // The device's own browser opens an artifact, where Kai is signed in, since the seat's streamed
  // browser is not and claude.ai refuses framing. It also opens every page when the host serves no browser.
  const outside = $derived(page !== undefined && inApp() && (isArtifactUrl(page.url) || !app.connection?.browser));

  async function openOutside(id: number, url: string): Promise<void> {
    // A failed open keeps the card, so the address stays to copy and the person can try again.
    if (await openLink(url)) dismissPage(id);
  }
</script>

<!-- Present from the first frame, since a status inserted with its text is not always announced. -->
<div class="region" role="status" aria-live="polite">
  {#if linkNotice.text}
    <section class="card failed" aria-label="A link did not open">
      <p class="who" role="alert">{linkNotice.text}</p>
      <div class="actions"><button type="button" class="button" onclick={clearLinkNotice}>Close</button></div>
    </section>
  {/if}
  {#if page}
    <section class="card" aria-label="A seat opened a page">
      <p class="who">{cardText(seat?.identity ?? page.sessionId, others)}</p>
      <p class="url">{label}</p>
      <div class="actions">
        {#if outside}
          <button type="button" class="button primary" aria-label={`Open in browser: ${label}`} onclick={() => openOutside(page.id, page.url)}>Open in browser</button>
        {:else}
          <button type="button" class="button primary" aria-label={`Show here: ${label}`} onclick={() => showPage(page.id)}>Show here</button>
        {/if}
        <button type="button" class="button" onclick={() => dismissPage(page.id)}>Not now</button>
      </div>
    </section>
  {/if}
</div>

<style>
  /* Below the seat header so its controls stay reachable, and above the brief toast. It stays until answered. */
  .region { position: fixed; z-index: 61; top: calc(env(safe-area-inset-top, 0px) + 76px); right: calc(env(safe-area-inset-right, 0px) + 16px); width: min(380px, calc(100vw - 32px)); display: flex; flex-direction: column; align-items: flex-end; gap: 8px; pointer-events: none; }
  .card { pointer-events: auto; width: 100%; display: flex; flex-direction: column; gap: 6px; padding: 12px 16px; border-radius: 10px; border: 1px solid var(--brand); background: var(--surface); color: var(--text); box-shadow: 0 6px 24px var(--shade); font-family: var(--font-display); }
  .card.failed { border-color: var(--danger-text); }
  .who { margin: 0; font-weight: 600; font-size: 15px; }
  .url { margin: 0; font: 13px/1.4 var(--font-mono); color: var(--text-soft); overflow-wrap: anywhere; }
  .actions { display: flex; gap: 8px; margin-top: 4px; }
  .actions :global(.button) { min-height: 44px; flex: 1; justify-content: center; }
  @media (max-width: 720px) {
    .region { top: calc(env(safe-area-inset-top, 0px) + 64px); left: calc(env(safe-area-inset-left, 0px) + 12px); right: calc(env(safe-area-inset-right, 0px) + 12px); width: auto; align-items: center; }
  }
</style>
