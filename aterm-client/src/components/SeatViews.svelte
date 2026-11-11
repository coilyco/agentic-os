<script lang="ts">
  import { LEVELS, type Level } from "../lib/transcript";

  let { level, raw, onlevel, onraw }: { level: Level; raw: boolean; onlevel: (next: Level) => void; onraw: (next: boolean) => void } = $props();

  const label = (each: Level) => each[0]!.toUpperCase() + each.slice(1);
  let group = $state<HTMLDivElement>();

  // One control, three stops: arrows move between them as a radio group does.
  function key(event: KeyboardEvent): void {
    const step = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : event.key === "ArrowLeft" || event.key === "ArrowUp" ? -1 : 0;
    if (!step) return;
    event.preventDefault();
    const next = LEVELS[(LEVELS.indexOf(level) + step + LEVELS.length) % LEVELS.length]!;
    onlevel(next);
    queueMicrotask(() => group?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus());
  }
</script>

<div class="views">
  <div class="stops" role="radiogroup" aria-label="Seat view" bind:this={group}>
    {#each LEVELS as each (each)}
      <button type="button" role="radio" aria-checked={level === each} tabindex={level === each ? 0 : -1} onclick={() => onlevel(each)} onkeydown={key}>{label(each)}</button>
    {/each}
  </div>
  {#if level === "verbose"}
    <button type="button" class="raw" aria-pressed={raw} onclick={() => onraw(!raw)}>Raw screen</button>
  {/if}
</div>

<style>
  .views { flex: none; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 6px 12px; background: var(--terminal); border-bottom: 1px solid var(--line); }
  .stops { flex: 1 1 auto; min-width: 0; display: flex; border: 1px solid var(--control-line); border-radius: 10px; overflow: hidden; }
  button { min-height: 44px; border: 0; background: transparent; color: var(--muted); font: 600 14px var(--font-body); }
  .stops button { flex: 1 1 0; min-width: 0; padding: 0 8px; }
  .stops button + button { border-left: 1px solid var(--control-line); }
  .stops button[aria-checked="true"] { background: color-mix(in srgb, var(--brand) 22%, var(--surface)); color: var(--text); }
  .raw { flex: none; padding: 0 12px; border: 1px solid var(--control-line); border-radius: 10px; color: var(--text); }
  .raw[aria-pressed="true"] { border-color: var(--brand); background: color-mix(in srgb, var(--brand) 22%, var(--surface)); }
  /* On a narrow phone the screen toggle takes its own row, so no stop is squeezed. */
  @media (max-width: 380px) { .stops, .raw { flex: 1 1 100%; } }
  button:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
  button:hover { color: var(--text); }
</style>
