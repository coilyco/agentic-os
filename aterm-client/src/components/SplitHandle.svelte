<script lang="ts">
  import { onMount } from "svelte";
  import { clampSide, keyedSide, SIDE_MIN } from "../lib/split";

  let { onchange, onreset }: { onchange: (width: number) => void; onreset: () => void } = $props();

  let handle: HTMLDivElement;
  let now = $state(0);
  let max = $state(0);
  let dragging = $state(false);

  const panel = () => handle.nextElementSibling as HTMLElement | null;
  const container = () => handle.parentElement?.clientWidth ?? 0;

  function measure(): void {
    now = Math.round(panel()?.getBoundingClientRect().width ?? 0);
    max = clampSide(Number.POSITIVE_INFINITY, container());
  }

  onMount(() => {
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(handle.parentElement!);
    const side = panel();
    if (side) observer.observe(side);
    return () => observer.disconnect();
  });

  function drag(event: PointerEvent): void {
    if (!dragging) return;
    const edge = handle.parentElement!.getBoundingClientRect().right;
    onchange(clampSide(edge - event.clientX, container()));
  }

  function key(event: KeyboardEvent): void {
    const next = keyedSide(event.key, event.shiftKey, now, container());
    if (next === null) return;
    event.preventDefault();
    onchange(next);
  }
</script>

<!-- A focusable separator is the window splitter pattern: a widget, which Svelte's a11y rules read as inert. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
  bind:this={handle}
  class="handle"
  class:dragging
  role="separator"
  aria-orientation="vertical"
  aria-label="Resize the side panel"
  aria-controls="side-panel"
  aria-valuenow={now}
  aria-valuemin={SIDE_MIN}
  aria-valuemax={max}
  tabindex="0"
  title="Drag, or press Left and Right. Double-click resets."
  onpointerdown={(event) => {
    dragging = true;
    handle.setPointerCapture(event.pointerId);
  }}
  onpointermove={drag}
  onpointerup={() => (dragging = false)}
  onpointercancel={() => (dragging = false)}
  ondblclick={onreset}
  onkeydown={key}
></div>

<style>
  .handle { position: relative; cursor: col-resize; touch-action: none; }
  .handle::after { content: ""; position: absolute; inset: 0 5px; background: var(--line); border-radius: 2px; transition: background 120ms; }
  .handle:hover::after, .handle.dragging::after { background: var(--accent, var(--brand)); }
  .handle:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
  @media (max-width: 1000px) {
    .handle { display: none; }
  }
</style>
