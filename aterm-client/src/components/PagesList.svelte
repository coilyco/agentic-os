<script lang="ts">
  import { fromText, type PageRow } from "../lib/artifact-pages";

  // Pages seats left for Kai (COI-3330). Waiting ones come first, since one is what she came for.
  let { rows, onopen, headingId }: { rows: PageRow[]; onopen: (id: string) => void; headingId: string } = $props();

  const said = (row: PageRow) => `${row.title}, ${fromText(row)}${row.waiting ? ", waiting for you" : ""}`;
</script>

<section class="pages" aria-labelledby={headingId}>
  <h2 id={headingId}>Pages for you <span class="n">{rows.length}</span></h2>
  <ul>
    {#each rows as row (row.id)}
      <li>
        <button type="button" class="page" class:waiting={row.waiting} data-page={row.id} aria-label={said(row)} onclick={() => onopen(row.id)}>
          <span class="text">
            <span class="title">{row.title}</span>
            <span class="from">{fromText(row)}</span>
          </span>
          {#if row.waiting}<span class="chip">Waiting</span>{:else}<span class="act">Open</span>{/if}
        </button>
      </li>
    {/each}
  </ul>
</section>

<style>
  .pages { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  h2 { margin: 0 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  .pages:has(.waiting) h2 { color: var(--warn-text); }
  .n { margin-left: 4px; font-weight: 400; }
  ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 6px; }
  .page { width: 100%; min-width: 0; min-height: 56px; display: flex; align-items: center; gap: 12px; padding: 8px 12px; border-radius: 10px; border: 1px solid var(--line); background: var(--terminal); color: var(--text); text-align: left; }
  .page:hover { border-color: var(--brand); }
  .page.waiting { background: color-mix(in srgb, var(--brand) 9%, var(--ground)); box-shadow: inset 2px 0 0 var(--brand); }
  .page:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .title { font-weight: 600; font-size: 15px; overflow-wrap: anywhere; }
  .from { font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
  .chip { flex: none; padding: 2px 8px; border-radius: 999px; font: 600 11px var(--font-mono); letter-spacing: 0.04em; text-transform: uppercase; color: var(--brand-ink); background: var(--brand); }
  .act { flex: none; font-size: 13px; color: var(--muted); }
</style>
