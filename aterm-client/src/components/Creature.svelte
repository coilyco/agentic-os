<script lang="ts">
  const art = import.meta.glob<string>("../assets/creatures/*.webp", { eager: true, query: "?url", import: "default" });

  let { role, color, size = 40 }: { role: string; color: string; size?: number } = $props();
  const src = $derived(art[`../assets/creatures/${role}.webp`]);
</script>

<!-- Decorative: every place a creature appears also names the seat in text. -->
{#if src}
  <img class="creature" {src} alt="" width={size} height={size} style:--accent={color} />
{:else}
  <span class="swatch" style:--accent={color} style:width={`${size}px`} style:height={`${size}px`} aria-hidden="true"></span>
{/if}

<style>
  .creature { flex: none; border-radius: 22%; background: color-mix(in srgb, var(--accent) 18%, var(--ground)); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 55%, transparent); object-fit: contain; }
  .swatch { flex: none; border-radius: 22%; background: color-mix(in srgb, var(--accent) 45%, var(--ground)); box-shadow: inset 0 0 0 1px var(--accent); }
</style>
