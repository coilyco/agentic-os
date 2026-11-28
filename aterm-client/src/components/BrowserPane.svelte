<script lang="ts">
  import { tick, untrack } from "svelte";
  import { ageText } from "../lib/age";
  import { app } from "../lib/app.svelte";
  import { openLink } from "../lib/link-notice.svelte";
  import { pageLabel, SHOW_WAIT_MS } from "../lib/pages";
  import type { Session } from "../lib/protocol";
  import type { SharedBrowser } from "../lib/screencast";
  import { driverLabel, isDriving, keyParams, mouseParams, paneSize, tabLabel, toPagePoint, wheelParams } from "../lib/screencast";

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
  let address = $state("");
  let lastMove = 0;
  let restarting = $state(false);
  let refocus = false;
  /** The closed browser a restart began from, so its echo is not taken for an answer. */
  let closedAtRestart: SharedBrowser | undefined;
  const tab = $derived(browser ? tabLabel(browser) : undefined);
  /** Spoken when the followed page changes. A title alone changes on every load, so it stays out of it. */
  let followed = $state("");
  let lastTab: string | undefined;
  const restartBar = $derived(browser?.state === "closed" || (restarting && browser?.state !== "live"));
  // Close page (COI-3820): a page the seat is driving asks once, and the empty state then says who closed it.
  let confirmClose = $state(false);
  let closedHere = $state(false);
  let keepIt: HTMLButtonElement | undefined = $state();
  $effect(() => {
    void browser?.url;
    if (browser?.state !== "live") confirmClose = false;
    else closedHere = false;
  });
  function closePage(confirmed = false): void {
    if (!channel?.close) return;
    if (browser?.driver === "agent" && !confirmed) {
      confirmClose = true;
      // Focus waits on the safe choice, so a second Enter does not end the seat's page.
      void tick().then(() => keepIt?.focus());
      return;
    }
    channel.close(session.id);
    confirmClose = false;
    closedHere = true;
  }

  // A hung page (COI-3825). It can clear on its own, so amber, not red.
  const stalled = $derived(browser?.state === "live" && browser.stalled === true);
  let now = $state(Date.now());
  $effect(() => {
    if (!stalled) return;
    now = Date.now();
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  });
  const pictureAge = $derived.by(() => {
    // Not `frame.at`: every watch replays the last frame, which would read as new.
    const at = browser?.frame?.paintedAt;
    if (at === undefined) return "";
    const age = ageText(Math.max(0, (now - at) / 1000));
    return age ? `${age} ago` : "just now";
  });
  // The strip leaving is silent to a screen reader, so the recovery is said.
  let answering = $state(false);
  let wasStalled = false;
  $effect(() => {
    const hung = stalled;
    answering = wasStalled && !hung && untrack(() => browser?.state) === "live";
    wasStalled = hung;
  });

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
    address = browser?.url ?? "";
  });

  // A click on a page card (COI-3258): hold control, then load the page the seat asked to open.
  const toShow = $derived(app.pageToShow?.sessionId === session.id ? app.pageToShow : null);
  let tookFor = 0;
  $effect(() => {
    const want = toShow;
    // The watch above starts the browser, and the page waits for it to be live.
    if (!want || !channel || browser?.state !== "live") return;
    if (driving) {
      channel.navigate(session.id, want.url);
      app.pageToShow = null;
    } else if (!heldElsewhere && tookFor !== want.since) {
      tookFor = want.since;
      channel.control(session.id, true);
    }
  });
  // A page that never loads says so, and names the way to do it by hand.
  $effect(() => {
    const want = toShow;
    if (!want) return;
    const timer = setTimeout(() => {
      if (app.pageToShow?.since !== want.since) return;
      app.pageToShow = null;
      app.notice = `${pageLabel(want.url)} did not load here. Take control in the Browser tab and enter it.`;
    }, Math.max(SHOW_WAIT_MS - (Date.now() - want.since), 0));
    return () => clearTimeout(timer);
  });

  // The daemon follows the newest page, so the pane moves tab on its own. Say so,
  // since a screen reader would otherwise hear nothing while the page changes.
  $effect(() => {
    const now = browser?.state === "live" ? `${browser.tab ?? 0}/${browser.tabs ?? 0}` : undefined;
    const label = untrack(() => tab);
    const title = untrack(() => browser?.title);
    if (lastTab !== undefined && now !== undefined && now !== lastTab) {
      const place = label ? `Now showing ${label.toLowerCase()}` : "Back to one tab";
      followed = title ? `${place}, ${title}.` : `${place}.`;
    }
    lastTab = now;
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
  {#if toShow}
    <p class="opening" role="status">Opening {pageLabel(toShow.url)} here. This takes control of {session.identity}'s browser until you hand it back.</p>
  {/if}
  {#if !channel}
    <p class="empty">This host doesn't stream a browser yet. Once its daemon does, you'll see the page {session.identity} is working in here, and you can take control of it.</p>
  {:else if restartBar}
    <div class="bar" bind:this={bar} data-driver="closed">
      <p class="who" role="status">{restarting ? `Starting a new browser for ${session.identity}.` : browser ? driverLabel(browser, session.identity) : ""}</p>
      <button type="button" class="button primary" aria-disabled={restarting} onclick={restart}>{restarting ? "Restarting" : "Restart browser"}</button>
    </div>
  {:else if !browser || browser.state === "none"}
    <p class="empty" role="status">{closedHere ? `You closed the page. ${session.identity}'s next page shows up here.` : `${session.identity} hasn't opened a browser yet. The page shows up here as soon as it loads one.`}</p>
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
      {#if channel?.close && browser.state === "live" && !confirmClose}
        <button type="button" class="button" onclick={() => closePage()}>Close page</button>
      {/if}
    </div>
    {#if confirmClose}
      <div class="confirm" role="group" aria-label="Close this page?">
        <p role="alert">{session.identity} is using this page. Closing it ends the page in {session.identity}'s browser.</p>
        <button bind:this={keepIt} type="button" class="button" onclick={() => (confirmClose = false)}>Keep it</button>
        <button type="button" class="button" onclick={() => closePage(true)}>Close it anyway</button>
      </div>
    {/if}
    <!-- Kept while live: a region inserted with its words is not reliably read. -->
    <div class={stalled ? "stall" : "visually-hidden"} role="status">
      {#if stalled}
        <p>The page stopped answering.</p>
        <p class="detail">{#if pictureAge}<span aria-live="off">Last picture {pictureAge}.</span>{" "}{/if}A script stuck in a loop, or a dialog open on the page, does this.{channel?.close ? " Close page ends it." : ""}</p>
      {:else if answering}
        The page is answering again.
      {/if}
    </div>
    {#if browser.state === "live" && browser.cloudflareCheck && browser.url}
      <!-- The check fails any browser driven over CDP, a click through this pane included, so the pane never tries it (COI-3803). -->
      {@const checked = browser.url}
      <div class="check" role="group" aria-label="Cloudflare check">
        <p>This page is a Cloudflare check, which can't pass in {session.identity}'s browser.</p>
        <button type="button" class="button primary" onclick={() => openLink(checked)}>Open in your browser</button>
      </div>
    {/if}
    {#if browser.state === "live" && browser.reason}
      <p class="note">{browser.reason}</p>
    {/if}
    <p class="tab">
      {#if tab}<span class="badge">{tab}</span>{/if}
      <span class="title">{browser.title || (browser.state === "live" ? "Untitled page" : "")}</span>
    </p>
    <p class="visually-hidden" role="status">{followed}</p>
    <form class="address" onsubmit={go}>
      <label class="visually-hidden" for={`address-${session.id}`}>Page address</label>
      <input id={`address-${session.id}`} class="mono" bind:value={address} readonly={!driving} spellcheck="false" autocomplete="off" />
      {#if driving}<button type="submit" class="button">Go</button>{/if}
    </form>
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
  .opening { margin: 0 0 8px; padding: 8px 12px; border: 1px solid var(--brand); border-radius: 8px; font-size: 14px; line-height: 1.4; color: var(--text); }
  .browser { display: flex; flex-direction: column; gap: 10px; min-height: 0; flex: 1; }
  .empty { margin: 0; color: var(--muted); font-size: 13px; }
  .bar { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--line); background: var(--surface); }
  .bar[data-driver="person"] { border-color: var(--brand); }
  .bar[data-driver="elsewhere"] { border-color: var(--warn-line); background: var(--warn-fill); }
  .check { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--brand); background: var(--surface); }
  .check p { margin: 0; flex: 1 1 200px; font-size: 14px; line-height: 1.4; }
  .stall { display: flex; flex-direction: column; gap: 4px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--warn-line); background: var(--warn-fill); }
  .stall p { margin: 0; font-size: 14px; line-height: 1.4; color: var(--warn-text); }
  .stall .detail { font-size: 13px; }
  .note { margin: 0; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  .bar[data-driver="closed"] { border-color: var(--danger-line); background: var(--danger-fill); }
  .who { margin: 0; flex: 1 1 160px; font-size: 14px; }
  .confirm { display: flex; align-items: center; gap: 8px 12px; flex-wrap: wrap; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--warn-line); background: var(--warn-fill); }
  .confirm p { margin: 0; flex: 1 1 200px; font-size: 14px; line-height: 1.4; color: var(--warn-text); }
  .bar .button[aria-disabled="true"] { opacity: 0.6; cursor: progress; }
  .bar[data-driver="closed"] .who { color: var(--danger-text); }
  .tab { display: flex; align-items: center; gap: 8px; margin: 0; min-width: 0; font-size: 13px; }
  .tab .badge { flex: none; padding: 2px 8px; border-radius: 999px; border: 1px solid var(--control-line); color: var(--text-soft); font-size: 12px; white-space: nowrap; }
  .tab .title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-soft); }
  .address { display: flex; gap: 8px; }
  .address input { flex: 1; min-width: 0; min-height: 40px; padding: 0 10px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text-soft); font-size: 13px; }
  .address input[readonly] { color: var(--muted); }
  .stage { position: relative; flex: 1; min-height: 240px; border-radius: 10px; border: 1px solid var(--line); background: var(--terminal); display: grid; place-items: center; overflow: hidden; touch-action: none; }
  .stage.driving { border-color: var(--brand); cursor: default; }
  .stage img { width: 100%; height: 100%; object-fit: contain; user-select: none; pointer-events: none; }
  .stage .empty { padding: 16px; }
</style>
