<script lang="ts">
  import { tick } from "svelte";
  import { inDemo, leaveDemo } from "../lib/app.svelte";

  // The banner goes with the demo, and focus on its button would fall to the page, so it goes to the way back in.
  async function leave(): Promise<void> {
    leaveDemo();
    await tick();
    document.getElementById("try-demo")?.focus();
  }
</script>

<!-- Present on every screen of the demo, so no screenshot of it can pass for a real host. -->
{#if inDemo()}
  <div class="banner" role="region" aria-label="Demo mode">
    <span class="tag mono">DEMO</span>
    <p>Made-up seats.<span class="more"> No network, no real shell.</span></p>
    <button class="button" type="button" aria-label="Leave demo" onclick={leave}>Leave<span class="more"> demo</span></button>
  </div>
{/if}

<style>
  .banner { flex: none; display: flex; align-items: center; gap: 10px; padding: calc(env(safe-area-inset-top) + 4px) calc(env(safe-area-inset-right) + 12px) 4px calc(env(safe-area-inset-left) + 12px); min-height: 52px; border-bottom: 1px dashed var(--warn); background: var(--warn-fill); }
  /* On a narrow phone the long half goes, and the entry card has already said it. */
  @media (max-width: 480px) { .more { display: none; } }
  .tag { flex: none; padding: 2px 8px; border-radius: 4px; background: var(--warn); color: var(--ground); font-size: 12px; font-weight: 700; letter-spacing: 0.1em; }
  p { flex: 1; min-width: 0; margin: 0; font-size: 14px; color: var(--warn-text); }
  .button { flex: none; }
</style>
