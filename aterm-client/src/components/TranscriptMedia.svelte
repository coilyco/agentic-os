<script lang="ts">
  import { onDestroy } from "svelte";
  import { app, mediaOf } from "../lib/app.svelte";
  import { describe, findShown, type Shown } from "../lib/media";

  // A file a seat shared, under the step that shared it (COI-3458).
  let { shown, at, sessionId, onopen }: { shown: Shown; at?: number; sessionId: string; onopen: (id: string) => void } = $props();

  // A larger image waits for the viewer.
  const INLINE_BYTES = 4_000_000;
  const channel = $derived(app.connection?.media);
  const item = $derived(channel ? findShown(mediaOf(sessionId), shown, at) : undefined);
  const inline = $derived(item?.kind === "image" && item.size <= INLINE_BYTES);

  let url = $state<string | undefined>();
  let failed = $state(false);
  let box = $state<HTMLElement>();
  let loadedFor = "";

  // Read once in view, so a seat's camera burst is not fetched on open.
  $effect(() => {
    const target = box;
    const want = inline ? item : undefined;
    const via = channel;
    if (!target || !want || !via || loadedFor === want.id) return;
    const watch = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      watch.disconnect();
      loadedFor = want.id;
      via.read(want).then(
        (blob) => {
          if (url) URL.revokeObjectURL(url);
          url = URL.createObjectURL(blob);
        },
        () => (failed = true),
      );
    });
    watch.observe(target);
    return () => watch.disconnect();
  });

  onDestroy(() => {
    if (url) URL.revokeObjectURL(url);
  });
</script>

<div class="shared" bind:this={box}>
  {#if !app.typing.allowed}
    <p class="note">Shared {shown.name}. Unlock this device to open the files a seat shares.</p>
  {:else if !item}
    <p class="note">Shared {shown.name}, which the host no longer has. A seat's shared files go when the seat ends or its host restarts.</p>
  {:else if inline && url && !failed}
    <button type="button" class="picture" aria-label={`Open ${item.name}, ${describe(item)}`} onclick={() => onopen(item.id)}>
      <img src={url} alt={item.name} />
    </button>
    <p class="caption"><span class="name">{item.name}</span> {describe(item)}</p>
  {:else}
    <button type="button" class="card" onclick={() => onopen(item.id)}>
      <span class="words"><span class="name">{item.name}</span><span class="what">{inline && !failed ? `${describe(item)}, loading` : describe(item)}</span></span>
      <span class="view">View</span>
    </button>
  {/if}
</div>

<style>
  .shared { margin: 6px 0 2px; max-width: 100%; font-family: var(--font-body); }
  .note { margin: 0; font-size: 13px; color: var(--muted); }
  .picture { display: block; max-width: 100%; padding: 0; border: 1px solid var(--line); border-radius: 8px; background: var(--terminal); overflow: hidden; }
  .picture img { display: block; max-width: 100%; max-height: 320px; object-fit: contain; }
  .caption { margin: 4px 0 0; font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
  .caption .name { color: var(--text-soft); }
  .card { display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; max-width: 420px; min-height: 44px; padding: 6px 6px 6px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); color: var(--text); text-align: left; font: inherit; }
  .words { display: flex; flex-direction: column; min-width: 0; }
  .card .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
  .what { font-size: 12px; color: var(--muted); }
  .view { flex: none; padding: 6px 14px; border-radius: 999px; border: 1px solid var(--brand); font-weight: 600; font-size: 14px; }
  .picture:focus-visible, .card:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
