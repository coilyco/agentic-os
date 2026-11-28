<script lang="ts">
  import { tick } from "svelte";
  import { CHOICES, choiceLabel, choiceShort, type ViewPick } from "../lib/seat-view";
  import { viewChoice } from "../lib/view-choice.svelte";

  const id = $props.id();
  let open = $state(false);
  let button = $state<HTMLButtonElement>();
  let group = $state<HTMLDivElement>();
  const current = $derived(viewChoice.current);

  // One line, three lines, every line, a line folding back, and a line running past the edge.
  const GLYPHS: Record<ViewPick, string> = {
    focus: "M3 8h10",
    default: "M3 4h10M3 8h10M3 12h6",
    verbose: "M3 3h10M3 5.5h10M3 8h10M3 10.5h10M3 13h7",
    "raw-wrapped": "M2 4h9a2.5 2.5 0 0 1 0 5H6M8 7l-2 2 2 2M2 13h5",
    "raw-unwrapped": "M2 6h12M11 3l3 3-3 3M2 11h6",
  };

  async function show(): Promise<void> {
    open = true;
    await tick();
    group?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus();
  }

  function hide(): void {
    open = false;
    void tick().then(() => button?.focus());
  }

  function choose(choice: ViewPick): void {
    viewChoice.pick?.(choice);
    hide();
  }

  // Arrows move between the five as a radio group does. Escape goes back to the seat.
  function key(event: KeyboardEvent): void {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      hide();
      return;
    }
    const step = event.key === "ArrowDown" || event.key === "ArrowRight" ? 1 : event.key === "ArrowUp" || event.key === "ArrowLeft" ? -1 : 0;
    if (!step) return;
    event.preventDefault();
    const radios = [...(group?.querySelectorAll<HTMLElement>('[role="radio"]:not(:disabled)') ?? [])];
    const at = radios.indexOf(document.activeElement as HTMLElement);
    radios[(at + step + radios.length) % radios.length]?.focus();
  }
</script>

{#if current}
  <button type="button" class="view" bind:this={button} aria-haspopup="dialog" aria-expanded={open} aria-label={`View: ${choiceLabel(current)}. Change the view`} title={`View: ${choiceLabel(current)}`} onclick={show}>
    <svg viewBox="0 0 16 16" width="18" height="18" aria-hidden="true" focusable="false"><path d={GLYPHS[current]} fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
    <span class="short">{choiceShort(current)}</span>
  </button>
{/if}

{#if open && current}
  <!-- COI-3741: a picker is its own screen, with nothing of the seat behind it. -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="picker" role="dialog" aria-modal="true" aria-labelledby={`${id}-title`} tabindex="-1" onkeydown={key}>
    <header>
      <h2 id={`${id}-title`}>View</h2>
      <button type="button" class="close" onclick={hide}>Close</button>
    </header>
    <div class="choices" role="radiogroup" aria-labelledby={`${id}-title`} bind:this={group}>
      {#each CHOICES as each (each.choice)}
        {@const off = !viewChoice.levels && !each.choice.startsWith("raw")}
        <button type="button" role="radio" aria-checked={current === each.choice} disabled={off} tabindex={current === each.choice ? 0 : -1} onclick={() => choose(each.choice)}>
          <span class="name">{each.label}</span>
          <span class="detail">{off ? "This host sends no transcript, so only its screen shows." : each.detail}</span>
        </button>
      {/each}
    </div>
  </div>
{/if}

<style>
  /* The size of the phone bar's Send feedback square, everywhere, so a desktop header never wraps for a long name (COI-3757). */
  .view { flex: none; width: 44px; height: 44px; padding: 0; display: inline-flex; flex-direction: column; align-items: center; justify-content: center; gap: 1px; border: 1px solid var(--control-line); border-radius: 8px; background: transparent; color: var(--text-soft); white-space: nowrap; }
  .view:hover { border-color: var(--brand); }
  .short { font: 600 9.5px/1 var(--font-body); letter-spacing: -0.01em; }
  .view:focus-visible, .close:focus-visible, [role="radio"]:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .picker { position: fixed; inset: 0; z-index: 50; display: flex; flex-direction: column; background: var(--ground); color: var(--text); padding: env(safe-area-inset-top, 0px) env(safe-area-inset-right, 0px) var(--bottom-clear) env(safe-area-inset-left, 0px); overflow-y: auto; }
  header { flex: none; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 12px 4px 16px; border-bottom: 1px solid var(--line); }
  h2 { margin: 0; font: 600 18px var(--font-display); }
  .close { min-width: 44px; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .choices { display: flex; flex-direction: column; gap: 8px; width: 100%; max-width: 560px; margin: 0 auto; padding: 16px; }
  [role="radio"] { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; min-height: 56px; padding: 10px 14px; border: 1px solid var(--control-line); border-radius: 10px; background: transparent; color: var(--text); text-align: left; }
  [role="radio"][aria-checked="true"] { border-color: var(--brand); background: color-mix(in srgb, var(--brand) 22%, var(--surface)); }
  .name { font: 600 16px var(--font-body); }
  .detail { font-size: 13px; color: var(--muted); }
  [role="radio"]:disabled { opacity: 0.55; cursor: not-allowed; }
</style>
