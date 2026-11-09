<script lang="ts">
  import canopy from "../assets/treehouse-canopy.svg?url";

  import { openSettings } from "../lib/app.svelte";

  let { seats }: { seats: number } = $props();
</script>

<!-- What shows with a host attached and no seat open: the logo, and how to go on. -->
<section class="home" aria-labelledby="home-title">
  <img src={canopy} alt="" width="168" height="168" />
  <h1 id="home-title" class="visually-hidden">Treehouse</h1>
  <p class="hint">{seats} {seats === 1 ? "seat" : "seats"} running. Pick one from the list.</p>
  <button type="button" class="settings" onclick={openSettings}>Settings</button>
</section>

<style>
  .home { flex: 1; min-height: 60dvh; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 20px; padding: 24px 16px; }
  img { width: clamp(120px, 32vw, 200px); height: auto; }
  .hint { margin: 0; color: var(--muted); font-size: 14px; text-align: center; }
  .settings { min-height: 44px; min-width: 120px; padding: 0 18px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .settings:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
