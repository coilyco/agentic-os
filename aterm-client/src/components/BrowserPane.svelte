<script lang="ts">
  import { tick } from "svelte";
  import { app } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";
  import type { SharedBrowser } from "../lib/screencast";
  import { driverLabel, isDriving, isStalled, keyParams, mouseParams, paneSize, toPagePoint, wheelParams } from "../lib/screencast";

  let { session }: { session: Session } = $props();

  const channel = $derived(app.connection?.browser);
  const browser = $derived(app.browsers[session.id]);
  const driving = $derived(isDriving(browser));
  const heldElsewhere = $derived(browser?.state === "live" && browser.driver === "person" && !browser.heldHere);
  /** Mouse moves with no button held are sent at most this often. */
  const MOVE_EVERY_MS = 40;
  /** The stage's `min-height` below, so a pane squeezed to nothing still asks for a frame. */
  const STAGE_MIN_HEIGHT = 240;
  /** A pane that settles within this many pixels of the size last sent keeps its stream. */
  const RESIZE_STEP_PX = 8;
  /** A restart the host never answers hands the button back after this long. */
  const RESTART_WAIT_MS = 8000;

  let root: HTMLDivElement | undefined = $state();
  let bar: HTMLDivElement | undefined = $state();
  let stage: HTMLDivElement | undefined = $state();
  let handBack: HTMLButtonElement | undefined = $state();
  let now = $state(Date.now());
  let address = $state("");
  let lastMove = 0;
  let restarting = $state(false);
  let refocus = false;
  /** The closed browser a restart began from, so its echo is not taken for an answer. */
  let closedAtRestart: SharedBrowser | undefined;
  const restartBar = $derived(browser?.state === "closed" || (restarting && browser?.state !== "live"));

  const sizeNow = () => paneSize(root?.getBoundingClientRect(), STAGE_MIN_HEIGHT);
  let sentSize: { width: number; height: number } | undefined;

  function watchNow(): void {
    sentSize = sizeNow();
    channel?.watch(session.id, sentSize);
  }

  // Waits for `root`, which binds after the first run, so the watch carries a size.
  $effect(() => {
    const live = channel;
    const id = session.id;
    if (!live || !root) return;
    watchNow();
    return () => live.unwatch(id);
  });

  // The pane settles after the first measure, and a window or phone can resize
  // it. A repeat watch changes the stream's size without leaving it. It waits
  // for a live browser, because watching a closed one starts it again.
  $effect(() => {
    const box = root;
    if (!box || !channel) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const observer = new ResizeObserver(() => {
      clearTimeout(timer);
      timer = setTimeout(() => {
        const size = sizeNow();
        const moved = size && (!sentSize || Math.abs(size.width - sentSize.width) >= RESIZE_STEP_PX || Math.abs(size.height - sentSize.height) >= RESIZE_STEP_PX);
        if (moved && browser?.state === "live") watchNow();
      }, 300);
    });
    observer.observe(box);
    return () => {
      clearTimeout(timer);
      observer.disconnect();
    };
  });

  // The host echoes the closed state and pushes `none` while it restarts, which
  // is not an answer. A live browser, or a closed one with a new reason, is.
  $effect(() => {
    if (!restarting || !browser) return;
    if (browser.state === "live" || (browser.state === "closed" && browser.reason !== closedAtRestart?.reason)) restarting = false;
  });

  $effect(() => {
    if (!restarting) return;
    const timer = setTimeout(() => ((restarting = false), (refocus = false)), RESTART_WAIT_MS);
    return () => clearTimeout(timer);
  });

  // The Restart button leaves with the closed state, so focus moves to the
  // bar's next control instead of dropping to the page.
  $effect(() => {
    if (browser?.state !== "live" || !refocus) return;
    refocus = false;
    void tick().then(() => bar?.querySelector("button")?.focus());
  });

  function restart(): void {
    if (restarting || !channel) return;
    restarting = true;
    refocus = true;
    closedAtRestart = browser;
    // Watching again starts a closed browser, so this stays a watcher throughout.
    watchNow();
  }

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

<div class="browser" bind:this={root}>
  {#if !channel}
    <p class="empty">This host doesn't stream a browser yet. Once its daemon does, you'll see the page {session.identity} is working in here, and you can take control of it.</p>
  {:else if restartBar}
    <div class="bar" bind:this={bar} data-driver="closed">
      <p class="who" role="status">{restarting ? `Starting a new browser for ${session.identity}.` : browser ? driverLabel(browser, session.identity) : ""}</p>
      <button type="button" class="button primary" aria-disabled={restarting} onclick={restart}>{restarting ? "Restarting" : "Restart browser"}</button>
    </div>
  {:else if !browser || browser.state === "none"}
    <p class="empty">{session.identity} hasn't opened a browser yet. The page shows up here as soon as it loads one.</p>
    {#if browser?.reason}<p class="empty">{browser.reason}</p>{/if}
  {:else}
    <div class="bar" bind:this={bar} data-driver={heldElsewhere ? "elsewhere" : browser.driver}>
      <p class="who" role="status">{driverLabel(browser, session.identity)}</p>
      {#if driving}
        <button bind:this={handBack} type="button" class="button" onclick={() => channel?.control(session.id, false)}>Hand back to {session.identity}</button>
      {:else if heldElsewhere}
        <button type="button" class="button" onclick={() => channel?.control(session.id, true, true)}>Take over on this screen</button>
      {:else}
        <button type="button" class="button primary" onclick={() => channel?.control(session.id, true)}>Take control</button>
      {/if}
    </div>
    {#if browser.state === "live" && browser.reason}
      <p class="note">{browser.reason}</p>
    {/if}
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
      aria-label={driving ? `${browser.title || "Page"}. Keys go to the page. Escape leaves it.` : `${browser.title || "Page"}, ${heldElsewhere ? "controlled from another screen" : `driven by ${session.identity}`}`}
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
  .bar[data-driver="elsewhere"] { border-color: #4a3d25; background: var(--warn-fill); }
  .note { margin: 0; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  .bar[data-driver="closed"] { border-color: #5a3a41; background: var(--danger-fill); }
  .who { margin: 0; flex: 1 1 160px; font-size: 14px; }
  .bar .button[aria-disabled="true"] { opacity: 0.6; cursor: progress; }
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
