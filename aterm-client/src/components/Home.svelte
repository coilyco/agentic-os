<script lang="ts">
  import { hideLogo } from "../lib/app.svelte";
  import ByCoilyco from "./ByCoilyco.svelte";
</script>

<!-- The logo screen (COI-3518), the pair and the words since COI-3740. One tap, or Escape, goes back. -->
<svelte:window onkeydown={(event) => event.key === "Escape" && hideLogo()} />
<section class="home" aria-label="Treehouse">
  <button type="button" class="logo" aria-label="Treehouse by coilyco. Back to the seats" onclick={hideLogo}>
    <ByCoilyco size="screen" inline />
  </button>
</section>

<style>
  .home { flex: 1; min-height: 60dvh; display: flex; align-items: center; justify-content: center; padding: 24px 16px; }
  .logo { min-width: 44px; min-height: 44px; max-width: 100%; padding: 16px; border: none; border-radius: 16px; background: transparent; color: inherit; }
  .logo:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
