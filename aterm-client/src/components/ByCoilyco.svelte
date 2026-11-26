<script lang="ts">
  import mark from "../assets/by-coilyco.svg?url";
  import { canopy } from "../lib/theme.svelte";

  // foot: a menu's last line. hero: the top of the seat overview. screen: the logo screen.
  let { size = "foot", inline = false }: { size?: "foot" | "hero" | "screen"; inline?: boolean } = $props();
</script>

<!-- Kai, COI-3740: the coil, then the canopy, then the words, and on a home screen the largest thing there. -->
<svelte:element this={inline ? "span" : "p"} class="by" data-size={size}>
  <span class="marks"><img class="coil" src={mark} alt="" /><img class="canopy" src={canopy()} alt="" /></span>
  <span class="words">Treehouse by coilyco</span>
</svelte:element>

<style>
  .by { display: flex; align-items: center; flex-wrap: wrap; margin: 0; }
  .marks { display: flex; align-items: center; flex: none; }
  img { display: block; flex: none; }
  .words { font-family: var(--font-display); font-weight: 600; line-height: 1.05; color: var(--text); }

  .by[data-size="foot"] { gap: 8px; margin: 14px 2px 0; }
  .by[data-size="foot"] .marks { gap: 4px; }
  .by[data-size="foot"] img { width: 18px; height: 18px; }
  .by[data-size="foot"] .coil { border-radius: 5px; }
  .by[data-size="foot"] .words { font-family: var(--font-body); font-weight: 400; font-size: 12px; color: var(--muted); }

  /* Wraps under the marks at a phone's width, one line beside them on a desktop. */
  .by[data-size="hero"] { gap: 10px 16px; }
  .by[data-size="hero"] .marks { gap: 8px; }
  .by[data-size="hero"] img { width: clamp(48px, 13vw, 64px); height: auto; }
  .by[data-size="hero"] .coil { border-radius: 14px; }
  .by[data-size="hero"] .words { font-size: clamp(26px, 8.4vw, 46px); }

  .by[data-size="screen"] { flex-direction: column; gap: 20px; text-align: center; }
  .by[data-size="screen"] .marks { gap: clamp(12px, 4vw, 24px); }
  .by[data-size="screen"] img { width: clamp(96px, 26vw, 168px); height: auto; }
  .by[data-size="screen"] .coil { border-radius: 28px; }
  .by[data-size="screen"] .words { font-size: clamp(26px, 7.6vw, 60px); }
</style>
