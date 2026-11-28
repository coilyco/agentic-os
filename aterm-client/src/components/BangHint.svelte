<script lang="ts">
  // Where a "!" draft runs (COI-3802). It floats above the field, so showing it never
  // resizes the seat's terminal and its PTY. The parent sets position: relative.
  let { text, id }: { text: string; id: string } = $props();
</script>

<!-- Mounted from the first frame, since a status inserted later is not announced. -->
<p {id} class="visually-hidden" role="status">{text}</p>
{#if text}
  <p class="bubble" aria-hidden="true">{text}</p>
{/if}

<style>
  .bubble { position: absolute; left: 0; right: 0; bottom: calc(100% + 6px); z-index: 4; margin: 0; padding: 6px 10px; border: 1px solid var(--control-line); border-radius: 8px; background: var(--surface); color: var(--text-soft); font-size: 13px; line-height: 1.35; pointer-events: none; box-shadow: 0 2px 8px rgb(0 0 0 / 0.18); }
</style>
