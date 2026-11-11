<script lang="ts">
  import canopy from "../assets/treehouse-canopy.svg?url";

  import { hideLogo } from "../lib/app.svelte";
</script>

<!-- The logo and nothing else (COI-3518). One tap, or Escape, goes back to the seats. -->
<svelte:window onkeydown={(event) => event.key === "Escape" && hideLogo()} />
<section class="home" aria-label="Treehouse">
  <button type="button" class="logo" aria-label="Treehouse. Back to the seats" onclick={hideLogo}>
    <img src={canopy} alt="" width="168" height="168" />
  </button>
</section>

<style>
  .home { flex: 1; min-height: 60dvh; display: flex; align-items: center; justify-content: center; padding: 24px 16px; }
  .logo { min-width: 44px; min-height: 44px; padding: 16px; border: none; border-radius: 16px; background: transparent; }
  .logo:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  img { display: block; width: clamp(120px, 32vw, 200px); height: auto; }
</style>
