<script lang="ts">
  import { tick } from "svelte";
  import { enterDemo } from "../lib/app.svelte";

  // This button leaves the page with the demo, and focus on it would fall to the top. The seat control says where you landed.
  async function start(): Promise<void> {
    enterDemo();
    await tick();
    document.querySelector<HTMLElement>('.switcher, [role="tab"][aria-selected="true"]')?.focus();
  }
</script>

<!-- The first screen's way in for anyone with no host to add. Nothing here dials. -->
<section class="demo" aria-labelledby="demo-title">
  <h2 id="demo-title">No host to connect to? Try the demo.</h2>
  <p>Look around with made-up seats. It runs on this device with no network, and its terminal is a script, not a real shell.</p>
  <button id="try-demo" class="button primary" type="button" onclick={start}>Try the demo</button>
</section>

<style>
  .demo { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; padding: 16px; border: 1px dashed var(--warn); border-radius: 10px; background: var(--warn-fill); }
  h2 { margin: 0; font-family: var(--font-display); font-size: 18px; font-weight: 600; color: var(--warn-text); }
  p { margin: 0; color: var(--text-soft); font-size: 15px; }
</style>
