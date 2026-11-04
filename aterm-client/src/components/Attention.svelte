<script lang="ts">
  import { onMount } from "svelte";
  import { app, cueSeats, openSeat, playCue } from "../lib/app.svelte";
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
</script>
