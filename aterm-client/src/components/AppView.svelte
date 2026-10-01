<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { app } from "../lib/app.svelte";
  import { resultText, ViewBridge, viewDocument, type DisplayMode, type View } from "../lib/mcp-apps";

  let { view }: { view: View } = $props();

  /** A view that has not asked to start by now is not going to. */
  const START_MS = 8000;
  const MIN_HEIGHT = 120;
  const MAX_INLINE_HEIGHT = 640;

  let frame: HTMLIFrameElement | undefined = $state();
  let height = $state(240);
  let fullscreen = $state(false);
  let silent = $state(false);
  let showText = $state(false);
  let bridge: ViewBridge | undefined;
  let startTimer: ReturnType<typeof setTimeout> | undefined;
  // A view re-sent with its result carries the same html, so srcdoc does not change and the view does not reload.
  const doc = $derived(viewDocument(view.html, view.csp));

  function setMode(mode: DisplayMode): DisplayMode {
    fullscreen = mode === "fullscreen";
    bridge?.updateContext({ displayMode: mode });
    return mode;
  }

  function onMessage(event: MessageEvent): void {
    // A sandboxed view has an opaque origin, so its window is how it is known.
    if (!frame || event.source !== frame.contentWindow) return;
    bridge?.receive(event.data);
    if (bridge?.started) silent = false;
  }

  onMount(() => {
    bridge = new ViewBridge(
      // Views come from app state, whose proxies cannot be structured-cloned into a postMessage.
      (message) => frame?.contentWindow?.postMessage($state.snapshot(message), "*"),
      {
        callTool: (name, args) => app.connection?.views?.call(view.id, "tools/call", { name, arguments: args }) ?? Promise.reject(new Error("This host no longer forwards view calls.")),
        readResource: (uri) => app.connection?.views?.call(view.id, "resources/read", { uri }) ?? Promise.reject(new Error("This host no longer forwards view calls.")),
        openLink: (url) => window.open(url, "_blank", "noopener,noreferrer"),
        resize: (next) => (height = Math.max(MIN_HEIGHT, Math.ceil(next))),
        displayMode: setMode,
      },
      {
        theme: "dark",
        displayMode: "inline",
        availableDisplayModes: ["inline", "fullscreen"],
        platform: matchMedia("(pointer: coarse)").matches ? "mobile" : "web",
        locale: navigator.language,
        timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        containerDimensions: { maxHeight: MAX_INLINE_HEIGHT, width: frame?.clientWidth ?? 360 },
      },
    );
    window.addEventListener("message", onMessage);
    sync();
    startTimer = setTimeout(() => {
      if (!bridge?.started) silent = true;
    }, START_MS);
  });

  function sync(): void {
    const { toolInput, toolResult, cancelled } = view;
    if (!bridge) return;
    bridge.setInput(toolInput);
    if (toolResult) bridge.setResult(toolResult);
    else if (cancelled !== undefined) bridge.cancel(cancelled);
  }

  $effect(sync);

  onDestroy(() => {
    clearTimeout(startTimer);
    window.removeEventListener("message", onMessage);
  });

  async function close(): Promise<void> {
    await bridge?.teardown("The person closed the view.");
    app.connection?.views?.close(view.id);
    delete app.views[view.id];
  }

  function onKey(event: KeyboardEvent): void {
    if (fullscreen && event.key === "Escape") setMode("inline");
  }
</script>

<svelte:window onkeydown={onKey} />

<article class="view" class:fullscreen class:bordered={view.prefersBorder !== false} aria-label={`${view.tool} view`}>
  <header>
    <span class="tool mono">{view.tool}</span>
    <span class="server">{view.server}</span>
    <div class="actions">
      <button type="button" class="small" aria-pressed={showText} onclick={() => (showText = !showText)}>Text</button>
      <button type="button" class="small" onclick={() => setMode(fullscreen ? "inline" : "fullscreen")}>{fullscreen ? "Exit full screen" : "Full screen"}</button>
      <button type="button" class="small" aria-label={`Close the ${view.tool} view`} onclick={close}>Close</button>
    </div>
  </header>

  {#if view.cancelled !== undefined && !view.toolResult}
    <p class="strip warn" role="status">The call was cancelled{view.cancelled ? `: ${view.cancelled}` : "."}</p>
  {:else if view.toolResult?.isError}
    <p class="strip danger" role="status">The tool reported an error. The view shows what it could.</p>
  {:else if !view.toolResult}
    <p class="strip" role="status">Waiting on the tool's result.</p>
  {/if}
  {#if silent}
    <p class="strip danger" role="alert">The view didn't start. The tool's text answer is below.</p>
  {/if}

  <iframe
    bind:this={frame}
    title={`${view.tool} from ${view.server}`}
    sandbox="allow-scripts"
    srcdoc={doc}
    style:height={fullscreen ? undefined : `${Math.min(height, MAX_INLINE_HEIGHT)}px`}
    hidden={silent}
  ></iframe>

  {#if showText || silent}
    <pre class="text">{resultText(view.toolResult)}</pre>
  {/if}
</article>

<style>
  .view { display: flex; flex-direction: column; border-radius: 10px; background: var(--surface); overflow: hidden; }
  .view.bordered { border: 1px solid var(--line); }
  .view.fullscreen { position: fixed; inset: 0; z-index: 20; border-radius: 0; }
  header { display: flex; align-items: center; gap: 6px 10px; flex-wrap: wrap; padding: 8px 10px; border-bottom: 1px solid var(--line); }
  .tool { font-size: 13px; color: var(--text); overflow-wrap: anywhere; }
  .server { font-size: 12px; color: var(--muted); }
  .actions { margin-left: auto; display: flex; gap: 6px; }
  .small { min-height: 32px; padding: 0 10px; border-radius: 6px; border: 1px solid var(--control-line); background: transparent; color: var(--text-soft); font-size: 12px; }
  .small[aria-pressed="true"] { border-color: var(--brand); color: var(--brand); }
  .strip { margin: 0; padding: 8px 10px; font-size: 13px; color: var(--muted); border-bottom: 1px solid var(--line); }
  .strip.warn { color: var(--warn-text); background: var(--warn-fill); }
  .strip.danger { color: var(--danger-text); background: var(--danger-fill); }
  iframe { display: block; width: 100%; border: 0; background: var(--terminal); }
  iframe[hidden] { display: none; }
  .fullscreen iframe { flex: 1; }
  .text { margin: 0; padding: 10px; max-height: 240px; overflow: auto; white-space: pre-wrap; font-family: var(--font-mono); font-size: 12px; color: var(--text-soft); border-top: 1px solid var(--line); }
</style>
