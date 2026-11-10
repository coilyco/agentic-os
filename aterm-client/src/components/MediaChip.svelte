<script lang="ts">
  let { seat, count, unseen, onopen }: { seat: string; count: number; unseen: number; onopen: () => void } = $props();
</script>

<button type="button" class="chip" onclick={onopen}>
  <span class="words">{seat} shared {count} {count === 1 ? "file" : "files"}{unseen > 0 ? `, ${unseen} new` : ""}</span>
  <span class="view">View</span>
</button>

<style>
  .chip { flex: none; display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; min-height: 44px; padding: 0 12px; border: 0; border-top: 1px solid var(--line); background: var(--surface); color: var(--text); font: 400 14px var(--font-body); text-align: left; }
  .words { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .view { flex: none; padding: 6px 14px; border-radius: 999px; border: 1px solid var(--brand); font-weight: 600; }
  .chip:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
</style>
