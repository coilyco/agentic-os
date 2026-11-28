<script lang="ts">
  import { onDestroy } from "svelte";
  import MediaViewer from "./MediaViewer.svelte";
  import type { MediaItem } from "../lib/media";
  import type { ImageChannel, MediaChannel } from "../lib/protocol";
  import { fileName, hostThumb, localCopy } from "../lib/sent-images";

  // The pictures of one sent message: this device's copy at once, else the host's thumbnail (COI-3800).
  let { paths, sessionId, channel }: { paths: string[]; sessionId: string; channel: ImageChannel | undefined } = $props();

  type Shown = { url?: string; error?: string };
  let shown = $state<Record<string, Shown>>({});
  const made: string[] = [];
  const urlOf = (blob: Blob): string => {
    const url = URL.createObjectURL(blob);
    made.push(url);
    return url;
  };

  $effect(() => {
    for (const path of paths) {
      if (shown[path]) continue;
      const mine = localCopy(path);
      if (mine) {
        shown[path] = { url: urlOf(mine) };
        continue;
      }
      const read = channel?.read;
      if (!read) {
        shown[path] = { error: "This host cannot show sent images yet. It needs a newer aterm." };
        continue;
      }
      shown[path] = {};
      hostThumb(read, sessionId, path).then(
        (blob) => (shown[path] = { url: urlOf(blob) }),
        (error: unknown) => (shown[path] = { error: error instanceof Error && error.message ? error.message : "The host would not send this image." }),
      );
    }
  });

  onDestroy(() => {
    for (const url of made) URL.revokeObjectURL(url);
  });

  const count = $derived(paths.length);
  const nth = (at: number) => (count > 1 ? ` ${at + 1} of ${count}` : "");

  // Full size is this device's copy when it has one, and the host's whole file otherwise.
  let viewing = $state<string | undefined>();
  const items = $derived<MediaItem[]>(paths.map((path) => ({ id: path, session: sessionId, name: fileName(path), mime: "image/*", kind: "image", size: localCopy(path)?.size ?? 0 })));
  // The transcript is its own stacking context, so a viewer left inside it sits under the seat's header.
  // Moving a node drops the focus the viewer gave its Close button, so it is given back.
  function toBody(node: HTMLElement): () => void {
    document.body.append(node);
    queueMicrotask(() => node.querySelector<HTMLElement>(".close")?.focus());
    return () => node.remove();
  }

  const full: MediaChannel = {
    read: async (item) => {
      const mine = localCopy(item.id);
      if (mine) return mine;
      if (!channel?.read) throw new Error("This host cannot send sent images back yet.");
      return channel.read(sessionId, item.id, true);
    },
  };
</script>

<ul class="sent-images" aria-label={count === 1 ? "Image you sent" : `${count} images you sent`}>
  {#each paths as path, at (path)}
    {@const one = shown[path]}
    <li>
      {#if one?.url}
        <button type="button" class="thumb" aria-label={`Open image${nth(at)} full size`} onclick={() => (viewing = path)}>
          <img src={one.url} alt={`Image you sent${nth(at)}`} />
        </button>
      {:else if one?.error}
        <p class="missing" role="note">Image{nth(at)} not shown. {one.error}</p>
      {:else}
        <p class="loading" role="status" aria-busy="true">Loading image{nth(at)}.</p>
      {/if}
    </li>
  {/each}
</ul>

{#if viewing}
  <div {@attach toBody}>
    <MediaViewer channel={full} {items} startId={viewing} seat="you" label="Image you sent" onclose={() => (viewing = undefined)} onseen={() => {}} />
  </div>
{/if}

<style>
  .sent-images { list-style: none; margin: 6px 0 0; padding: 0; display: flex; flex-wrap: wrap; gap: 8px; }
  li { min-width: 0; max-width: 100%; }
  /* Fixed pixels, not a percentage: a percentage cap inside a shrink-to-fit button leaves the button at the picture's natural width. */
  .thumb { display: block; padding: 0; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); overflow: hidden; cursor: zoom-in; min-width: 44px; min-height: 44px; line-height: 0; }
  .thumb:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  img { display: block; max-width: 240px; max-height: 240px; width: auto; height: auto; }
  .loading { margin: 0; width: 160px; max-width: 100%; height: 120px; display: grid; place-items: center; border: 1px dashed var(--line); border-radius: 8px; font: 400 12px var(--font-body); color: var(--muted); }
  .missing { margin: 0; padding: 8px 10px; border: 1px dashed var(--line); border-radius: 8px; font: 400 12px var(--font-body); color: var(--muted); }
</style>
