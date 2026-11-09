<script lang="ts">
  import { onMount } from "svelte";
  import { app, openSeat } from "../lib/app.svelte";
  import { unlock } from "../lib/chime";
  import { closeSeatNotifications } from "../lib/push";
  import { openSeatFromNotification, settleSeat, syncPush, takeLaunchSeat } from "../lib/push.svelte";

  // A seat that starts waiting makes no sound here, only the glow and list cues (COI-3517).
  // The coordinator's own ping arrives as a notify frame.

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
