<script lang="ts">
  import { onMount } from "svelte";
  import { describe, stepped, turned, type MediaItem } from "../lib/media";
  import type { MediaChannel } from "../lib/protocol";
  import { formatSize } from "../lib/images";

  let { channel, items, startId, seat, onclose, onseen }: { channel: MediaChannel; items: MediaItem[]; startId?: string; seat: string; onclose: () => void; onseen: () => void } = $props();

  // svelte-ignore state_referenced_locally
  let at = $state(Math.max(0, startId ? items.findIndex((item) => item.id === startId) : items.length - 1));
  // Sitting on the newest, so a frame that arrives takes the view. Stepping back leaves it be.
  // svelte-ignore state_referenced_locally
  let following = $state(at === items.length - 1);
  const current = $derived(items[Math.min(at, items.length - 1)]);
  const total = $derived(items.length);

  $effect(() => {
    if (items.length === 0) onclose();
    else if (following) at = items.length - 1;
    else if (at >= items.length) at = items.length - 1;
  });

  type Load = { url?: string; error?: string; bytes?: number };
  let loads = $state<Record<string, Load>>({});
  let turns = $state<Record<string, number>>({});
  let note = $state("");
  let root = $state<HTMLDivElement>();
  let closeButton = $state<HTMLButtonElement>();
  const urls: string[] = [];
  const asked = new Set<string>();

  async function ensure(item: MediaItem): Promise<string | undefined> {
    const held = loads[item.id]?.url;
    if (held) return held;
    if (asked.has(item.id)) return undefined;
    asked.add(item.id);
    loads[item.id] = { bytes: 0 };
    try {
      const blob = await channel.read(item, (bytes) => (loads[item.id] = { bytes }));
      const url = URL.createObjectURL(blob);
      urls.push(url);
      loads[item.id] = { url };
      return url;
    } catch (error) {
      loads[item.id] = { error: error instanceof Error ? error.message : "The host would not send that file." };
      return undefined;
    } finally {
      asked.delete(item.id);
    }
  }

  // A file the viewer cannot show waits for Download. An image or video loads as it comes up.
  $effect(() => {
    const item = current;
    if (item && item.kind !== "file" && !loads[item.id]) void ensure(item);
    if (item && at === items.length - 1) onseen();
  });

  function go(direction: 1 | -1): void {
    at = stepped(at, total, direction);
    following = at === total - 1;
    note = "";
  }

  async function save(item: MediaItem): Promise<void> {
    note = "";
    const url = await ensure(item);
    if (!url) return;
    const link = document.createElement("a");
    link.href = url;
    link.download = item.name;
    document.body.append(link);
    link.click();
    link.remove();
    note = `Asked this device to save ${item.name}. If nothing was saved, its browser blocked the download.`;
  }

  function retry(item: MediaItem): void {
    delete loads[item.id];
    void ensure(item);
  }

  function key(event: KeyboardEvent): void {
    if (event.key === "Escape") {
      event.preventDefault();
      onclose();
    } else if (event.key === "ArrowLeft" && !(event.target instanceof HTMLVideoElement)) go(-1);
    else if (event.key === "ArrowRight" && !(event.target instanceof HTMLVideoElement)) go(1);
    else if (event.key === "Tab" && root) {
      const stops = [...root.querySelectorAll<HTMLElement>("button:not([disabled]), video[controls]")];
      if (stops.length === 0) return;
      const first = stops[0]!;
      const last = stops[stops.length - 1]!;
      if (event.shiftKey && document.activeElement === first) (event.preventDefault(), last.focus());
      else if (!event.shiftKey && document.activeElement === last) (event.preventDefault(), first.focus());
    }
  }

  onMount(() => {
    const before = document.activeElement as HTMLElement | null;
    closeButton?.focus();
    return () => {
      for (const url of urls) URL.revokeObjectURL(url);
      before?.focus?.();
    };
  });

  const load = $derived(current ? loads[current.id] : undefined);
  const turn = $derived(current ? (turns[current.id] ?? 0) : 0);
</script>

{#if current}
  <div class="viewer" role="dialog" aria-modal="true" aria-label={`Shared by ${seat}`} tabindex="-1" bind:this={root} onkeydown={key}>
    <header>
      <div class="who">
        <strong>{current.name}</strong>
        <span>{describe(current)}{total > 1 ? `, ${at + 1} of ${total}` : ""}</span>
      </div>
      <button type="button" class="close" bind:this={closeButton} onclick={onclose}>Close</button>
    </header>
    <div class="stage" data-turned={turn % 180 !== 0}>
      {#if load?.error}
        <div class="card" role="alert">
          <p>{load.error}</p>
          <button type="button" onclick={() => retry(current)}>Try again</button>
        </div>
      {:else if current.kind === "image"}
        {#if load?.url}
          <img src={load.url} alt={current.name} style:--turn={`${turn}deg`} />
        {:else}
          <p class="status" role="status">Loading {current.name}{load?.bytes ? `, ${formatSize(load.bytes)} so far` : ""}.</p>
        {/if}
      {:else if current.kind === "video"}
        {#if load?.url}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video src={load.url} controls playsinline preload="metadata"></video>
        {:else}
          <p class="status" role="status">Loading {current.name}{load?.bytes ? `, ${formatSize(load.bytes)} so far` : ""}.</p>
        {/if}
      {:else}
        <div class="card">
          <p><strong>{current.name}</strong></p>
          <p>{describe(current)}</p>
          <p>Treehouse cannot show this kind of file. Download it to open it elsewhere.</p>
        </div>
      {/if}
    </div>
    <footer>
      <button type="button" onclick={() => go(-1)} disabled={at === 0} aria-label="Previous file">Previous</button>
      {#if current.kind === "image"}
        <button type="button" onclick={() => (turns[current.id] = turned(turn))}>Rotate</button>
      {/if}
      <button type="button" onclick={() => void save(current)}>Download</button>
      <button type="button" onclick={() => go(1)} disabled={at === total - 1} aria-label="Next file">Next</button>
    </footer>
    <p class="note" role="status">{note}</p>
  </div>
{/if}

<style>
  .viewer { position: fixed; inset: 0; z-index: 60; display: flex; flex-direction: column; background: var(--backdrop); color: var(--text); padding-top: env(safe-area-inset-top, 0px); padding-bottom: env(safe-area-inset-bottom, 0px); }
  header { flex: none; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border-bottom: 1px solid var(--line); }
  .who { min-width: 0; display: flex; flex-direction: column; }
  .who strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 15px var(--font-body); }
  .who span { font: 400 13px var(--font-body); color: var(--muted); }
  .stage { flex: 1 1 auto; min-height: 0; container-type: size; display: grid; place-items: center; overflow: hidden; padding: 8px; }
  img { max-width: 100cqw; max-height: 100cqh; object-fit: contain; transform: rotate(var(--turn, 0deg)); }
  .stage[data-turned="true"] img { max-width: calc(100cqh - 16px); max-height: calc(100cqw - 16px); }
  video { max-width: 100cqw; max-height: 100cqh; }
  .card { max-width: 32rem; padding: 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); display: grid; gap: 8px; }
  .card p, .status { margin: 0; font: 400 15px/1.5 var(--font-body); color: var(--text-soft); overflow-wrap: anywhere; }
  footer { flex: none; display: flex; gap: 6px; padding: 8px 12px; border-top: 1px solid var(--line); }
  footer button { flex: 1 1 0; min-width: 0; }
  button { min-height: 44px; padding: 0 10px; border-radius: 10px; border: 1px solid var(--control-line); background: var(--surface); color: var(--text); font: 600 14px var(--font-body); }
  button:disabled { opacity: 0.4; }
  button:focus-visible, video:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .note { flex: none; margin: 0; min-height: 22px; padding: 0 12px 8px; font: 400 12px var(--font-body); color: var(--muted); }
</style>
