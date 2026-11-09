<script lang="ts">
  import { onMount } from "svelte";
  import { app, cueSeats, openSeat, playCue } from "../lib/app.svelte";
  import { detectCue, SETTLE_MS } from "../lib/attention";
  import { unlock } from "../lib/chime";
  import { closeSeatNotifications } from "../lib/push";
  import { openSeatFromNotification, settleSeat, syncPush, takeLaunchSeat } from "../lib/push.svelte";

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

  // The switch reads the browser's subscription again whenever the host it belongs to changes.
  $effect(() => {
    void app.attachedHostId;
    void app.link.state;
    void app.webPush;
    void syncPush();
  });

  // A tapped notification names a seat, and the host may not have listed it yet.
  $effect(() => {
    void app.attachedHostId;
    void app.sessions;
    settleSeat();
  });

  // The seat is open, so its notification has done its work.
  $effect(() => {
    const open = openSeat();
    if (open && app.pageVisible) void closeSeatNotifications(open);
  });

  onMount(() => {
    takeLaunchSeat();
    const tapped = (event: MessageEvent) => {
      const { type, session } = (event.data ?? {}) as { type?: unknown; session?: unknown };
      if (type === "aterm-open-seat" && typeof session === "string" && session) openSeatFromNotification(session);
    };
    const worker = "serviceWorker" in navigator ? navigator.serviceWorker : undefined;
    worker?.addEventListener("message", tapped);
    // addEventListener does not start the message queue the way onmessage does.
    worker?.startMessages();
    const visibility = () => (app.pageVisible = document.visibilityState === "visible");
    const touch = () => {
      if (app.alerts.sound && app.alerts.soundBlocked) void unlock();
    };
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("pointerdown", touch);
    window.addEventListener("keydown", touch);
    return () => {
      worker?.removeEventListener("message", tapped);
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("pointerdown", touch);
      window.removeEventListener("keydown", touch);
    };
  });
</script>
