<script lang="ts">
  import { app } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";
  import { driverLabel, isStalled, keyParams, mouseParams, toPagePoint, wheelParams } from "../lib/screencast";

  let { session }: { session: Session } = $props();

  const channel = $derived(app.connection?.browser);
  const browser = $derived(app.browsers[session.id]);
  const driving = $derived(browser?.state === "live" && browser.driver === "person");
  /** Mouse moves with no button held are sent at most this often. */
  const MOVE_EVERY_MS = 40;

  let stage: HTMLDivElement | undefined = $state();
  let handBack: HTMLButtonElement | undefined = $state();
  let now = $state(Date.now());
  let address = $state("");
  let lastMove = 0;

  $effect(() => {
    const live = channel;
    const id = session.id;
    if (!live) return;
    live.watch(id);
    return () => live.unwatch(id);
  });

  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  });

  $effect(() => {
    address = browser?.url ?? "";
  });

  // A wheel listener has to be active to stop the pane itself scrolling.
  $effect(() => {
    const box = stage;
    if (!box || !driving) return;
    const onWheel = (event: WheelEvent) => {
      const point = pointOf(event);
      if (!point) return;
      event.preventDefault();
      channel?.input(session.id, "wheel", wheelParams(point, event));
    };
    box.addEventListener("wheel", onWheel, { passive: false });
    return () => box.removeEventListener("wheel", onWheel);
  });

  function pointOf(event: MouseEvent): { x: number; y: number } | null {
    if (!stage || !browser?.frame) return null;
    return toPagePoint(event.clientX, event.clientY, stage.getBoundingClientRect(), browser.frame.metadata);
  }

  function mouse(type: "mousePressed" | "mouseReleased" | "mouseMoved", event: PointerEvent): void {
    if (!driving) return;
    if (type === "mouseMoved" && !event.buttons && event.timeStamp - lastMove < MOVE_EVERY_MS) return;
    lastMove = event.timeStamp;
    const point = pointOf(event);
    if (!point) return;
    if (type === "mousePressed") stage?.focus();
    channel?.input(session.id, "mouse", mouseParams(type, point, event));
  }

  function key(type: "keyDown" | "keyUp", event: KeyboardEvent): void {
    if (!driving) return;
    // Escape leaves the page, so the keyboard is never trapped in it.
    if (event.key === "Escape") {
      if (type === "keyDown") handBack?.focus();
      return;
    }
    event.preventDefault();
    channel?.input(session.id, "key", keyParams(type, event));
  }

  function go(event: SubmitEvent): void {
    event.preventDefault();
    const url = address.trim();
    if (!url || !driving) return;
    channel?.navigate(session.id, /^[a-z][a-z0-9+.-]*:/i.test(url) ? url : `https://${url}`);
  }
</script>

<div class="browser">
  {#if !channel}
    <p class="empty">This host doesn't stream a browser yet. Once its daemon does, you'll see the page {session.identity} is working in here, and you can take control of it.</p>
  {:else if !browser || browser.state === "none"}
    <p class="empty">{session.identity} hasn't opened a browser yet. The page shows up here as soon as it loads one.</p>
  {:else}
    <div class="bar" data-driver={browser.state === "closed" ? "closed" : browser.driver}>
      <p class="who" role="status">{driverLabel(browser, session.identity)}</p>
      {#if browser.state !== "closed"}
        {#if driving}
          <button bind:this={handBack} type="button" class="button" onclick={() => channel?.control(session.id, false)}>Hand back to {session.identity}</button>
        {:else}
          <button type="button" class="button primary" onclick={() => channel?.control(session.id, true)}>Take control</button>
        {/if}
      {/if}
    </div>
    <form class="address" onsubmit={go}>
      <label class="visually-hidden" for={`address-${session.id}`}>Page address</label>
      <input id={`address-${session.id}`} class="mono" bind:value={address} readonly={!driving} spellcheck="false" autocomplete="off" />
      {#if driving}<button type="submit" class="button">Go</button>{/if}
    </form>
    {#if isStalled(browser, now)}
      <p class="strip" role="status">The stream stalled. The last frame is {Math.round((now - (browser.frame?.at ?? now)) / 1000)} seconds old.</p>
    {/if}
    <!-- The page is a picture of the host's browser, so its own elements are not here to focus. The application role hands keys and pointer to it whole. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      bind:this={stage}
      class="stage"
      class:driving
      role="application"
      aria-roledescription="remote page"
      aria-label={driving ? `${browser.title || "Page"}. Keys go to the page. Escape leaves it.` : `${browser.title || "Page"}, driven by ${session.identity}`}
      tabindex={driving ? 0 : -1}
      onpointerdown={(event) => mouse("mousePressed", event)}
      onpointerup={(event) => mouse("mouseReleased", event)}
      onpointermove={(event) => mouse("mouseMoved", event)}
      oncontextmenu={(event) => driving && event.preventDefault()}
      onkeydown={(event) => key("keyDown", event)}
      onkeyup={(event) => key("keyUp", event)}
    >
      {#if browser.frame}
        <img src={browser.frame.src} alt="" draggable="false" />
      {:else}
        <p class="empty">Waiting on the first frame.</p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .browser { display: flex; flex-direction: column; gap: 10px; min-height: 0; flex: 1; }
  .empty { margin: 0; color: var(--muted); font-size: 13px; }
  .bar { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--line); background: var(--surface); }
  .bar[data-driver="person"] { border-color: var(--brand); }
  .bar[data-driver="closed"] { border-color: #5a3a41; background: var(--danger-fill); }
  .who { margin: 0; flex: 1 1 160px; font-size: 14px; }
  .bar[data-driver="closed"] .who { color: var(--danger-text); }
  .address { display: flex; gap: 8px; }
  .address input { flex: 1; min-width: 0; min-height: 40px; padding: 0 10px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text-soft); font-size: 13px; }
  .address input[readonly] { color: var(--muted); }
  .strip { margin: 0; padding: 8px 10px; border-radius: 8px; font-size: 13px; color: var(--warn-text); background: var(--warn-fill); }
  .stage { position: relative; flex: 1; min-height: 240px; border-radius: 10px; border: 1px solid var(--line); background: var(--terminal); display: grid; place-items: center; overflow: hidden; touch-action: none; }
  .stage.driving { border-color: var(--brand); cursor: default; }
  .stage img { width: 100%; height: 100%; object-fit: contain; user-select: none; pointer-events: none; }
  .stage .empty { padding: 16px; }
</style>
