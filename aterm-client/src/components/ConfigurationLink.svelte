<script lang="ts">
  import { openConfiguration } from "../lib/app.svelte";

  /** Called after Configuration opens, so a phone's open menu can put itself away. */
  let { onopen }: { onopen?: () => void } = $props();
</script>

<button type="button" class="link" onclick={() => { openConfiguration(); onopen?.(); }}>Configuration</button>

<style>
  .link { min-height: 44px; width: 100%; margin-top: 8px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); text-align: left; }
  .link:hover { border-color: var(--brand); }
  .link:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
