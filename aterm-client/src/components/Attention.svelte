<script lang="ts">
  import { onMount } from "svelte";
  import { app, cueSeats, glowSeats, openSeat, playCue } from "../lib/app.svelte";
  import { detectCue, SETTLE_MS } from "../lib/attention";
  import { unlock } from "../lib/chime";

  let known: Set<string> | null = null;
  let attachedFor: string | null = null;
  let settleUntil = 0;

  // Tracks who is waiting whether or not Sound is on, so switching it on later is not news.
  $effect(() => {
    const ids = cueSeats();
    const open = openSeat();
    if (app.attachedHostId !== attachedFor) {
      attachedFor = app.attachedHostId;
      known = null;
      settleUntil = performance.now() + SETTLE_MS;
    }
    const settling = known === null || performance.now() < settleUntil;
    const result = detectCue(settling ? null : known, ids, open);
    known = result.known;
    if (result.fire) playCue();
  });

  // Coming back to a tab whose open seat finished is the opening of it.
  $effect(() => {
    const open = openSeat();
    if (open && app.attention[open]) delete app.attention[open];
  });

  onMount(() => {
    const visibility = () => (app.pageVisible = document.visibilityState === "visible");
    const touch = () => {
      if (app.alerts.sound && app.alerts.soundBlocked) void unlock();
    };
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("pointerdown", touch);
    window.addEventListener("keydown", touch);
    return () => {
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("pointerdown", touch);
      window.removeEventListener("keydown", touch);
    };
  });

  const glow = $derived(app.alerts.visual ? glowSeats() : []);
  const colors = $derived([...new Set(glow.map((seat) => seat.color))]);
  const gradient = $derived(`conic-gradient(${[...colors, colors[0]].join(", ")})`);
</script>

{#if colors.length}
  <div class="glow" aria-hidden="true" style:background={gradient}></div>
{/if}

<style>
  /* Opaque at the page edge, clear toward the middle, so it reads from the corner of the eye and covers nothing. */
  .glow {
    position: fixed; inset: 0; z-index: 50; pointer-events: none; opacity: 0.65;
    -webkit-mask-image: radial-gradient(ellipse farthest-side at center, transparent 86%, #000 100%);
    mask-image: radial-gradient(ellipse farthest-side at center, transparent 86%, #000 100%);
    animation: breathe 2.4s ease-in-out infinite;
  }
  @keyframes breathe { 0%, 100% { opacity: 0.45; } 50% { opacity: 0.85; } }
  @media (prefers-reduced-motion: reduce) {
    .glow { animation: none; opacity: 0.65; }
  }
</style>
