<script lang="ts">
  import type { Preview } from "../lib/preview";

  // Never takes focus or the pointer: the seat row under it stays what the person is on.
  let { preview, top, left, color }: { preview: Preview; top: number; left: number; color: string } = $props();
</script>

<div id="seat-preview" class="preview" role="tooltip" style:top={`${top}px`} style:left={`${left}px`} style:--accent={color}>
  <p class="who"><span class="name">{preview.identity}</span> <span class="kind" data-kind={preview.kind}>{preview.heading}</span></p>
  {#if preview.detail}<p class="detail">{preview.detail}</p>{/if}
  <p class="hint">Click to open</p>
</div>

<style>
  .preview { position: fixed; z-index: 70; width: 320px; max-width: calc(100vw - 24px); padding: 10px 12px; border-radius: 8px; border: 1px solid var(--accent); border-left-width: 4px; background: var(--surface); color: var(--text); box-shadow: 0 6px 24px #000a; pointer-events: none; }
  p { margin: 0; }
  .who { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 8px; }
  .name { font-family: var(--font-display); font-weight: 600; font-size: 15px; }
  .kind { font-size: 12px; color: var(--muted); }
  .kind[data-kind="asking"] { color: var(--accent); }
  .detail { margin-top: 6px; font-size: 13px; line-height: 1.4; color: var(--text-soft); display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
  .hint { margin-top: 6px; font-size: 11px; color: var(--muted); }
</style>
