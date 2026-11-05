<script lang="ts">
  import { onMount, tick } from "svelte";
  import type { CardMemo } from "../lib/card-memo.svelte";
  import { progressText, type TabReading } from "../lib/dialog-tabs";
  import { advanceIndex, nextRowIndex, type Choice } from "../lib/choices";

  let { choice, identity, memo, tabs = null, ontab, onanswer, oncancel, focusToken = 0, onfocused, locked = false, cancelLocked = locked, lockedLabel = "Read only" }: {
    choice: Choice;
    identity: string;
    /** The card's working state, held by its parent so a rebuilt card keeps its text box and draft. */
    memo: CardMemo;
    /** A multi-question dialog's tabs and the current one, read off its screen. */
    tabs?: TabReading | null;
    /** Asked to move to a tab. The pane presses the arrows. */
    ontab?: (index: number) => void;
    /** Each change asks the card to take the keyboard, as when you alt-tab in. */
    focusToken?: number;
    /** Said once the card has taken the keyboard, so the request is spent and a later card does not repeat it. */
    onfocused?: () => void;
    /** Indexes of the picked options, and the typed text when "Type something." was one. */
    onanswer: (picks: number[], text?: string) => void;
    oncancel: () => void;
    /** The daemon refuses this connection's answers. The composer below says why. */
    locked?: boolean;
    /** An ask is cancelled by `cancel_ask`, which the guard leaves open. A menu's Esc is typed. */
    cancelLocked?: boolean;
    /** What the chip says while locked. A link that is down is not a read-only connection. */
    lockedLabel?: string;
  } = $props();

  let card: HTMLElement;
  // On a phone the card can fold to a bar, so the output behind it can be read.
  let narrow = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px)").matches);
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const read = () => (narrow = query.matches);
    query.addEventListener("change", read);
    return () => query.removeEventListener("change", read);
  });
  const folded = $derived(memo.collapsed && narrow);
  const progress = $derived(tabs ? progressText(tabs) : "");
  let lastFocus = 0;
  $effect(() => {
    if (focusToken && focusToken !== lastFocus) {
      lastFocus = focusToken;
      card?.querySelector<HTMLButtonElement>("button.option, button.advance")?.focus();
      onfocused?.();
    }
  });
  // The field replaces the button that was just clicked, or opens under it, and the page's focus does not follow by itself.
  $effect(() => {
    if (memo.writing === -1 || !memo.focused) return;
    void tick().then(() => {
      const field = card?.querySelector<HTMLInputElement>("#choice-text");
      field?.focus();
      // The card scrolls inside itself, and the box it just opened must not sit below the fold.
      field?.scrollIntoView({ block: "nearest" });
    });
  });
  const freeIndex = $derived(choice.options.findIndex((option) => option.freeText));
  // A harness's own Next row rides in the pinned footer, so a long list never scrolls it away.
  const advance = $derived(advanceIndex(choice));
  // A multi-select's Next applies the picks first, so it is the card's one submit and the list has no row for it.
  const next = $derived(nextRowIndex(choice));
  const nextLabel = $derived(next === -1 ? "" : (choice.options[next]?.label ?? ""));
  // Claude Code draws that row without a number, so the rows after it count one fewer than their place in the list.
  const shown = (index: number): number => index + 1 - (next !== -1 && next < index ? 1 : 0);
  const rows = $derived(choice.options.flatMap((_, index) => (index === next ? [] : [index])));

  function pick(index: number): void {
    if (choice.multi) {
      memo.picked = memo.picked.includes(index) ? memo.picked.filter((each) => each !== index) : [...memo.picked, index];
      memo.writing = choice.options[index]?.freeText && memo.picked.includes(index) ? index : choice.options[index]?.freeText ? -1 : memo.writing;
      if (memo.writing !== -1) memo.focused = true;
      return;
    }
    if (choice.options[index]?.freeText) {
      memo.writing = index;
      memo.focused = true;
      return;
    }
    onanswer([index]);
  }

  // The box beside Answer is about 150px on a phone, so its hint has to be short there.
  const placeholder = $derived(narrow ? "Your own answer" : "Type or dictate your own answer");

  // A field taken out of the page fires no blur worth believing, so only a field still there was left on purpose.
  function leave(event: FocusEvent): void {
    if ((event.currentTarget as HTMLElement).isConnected) memo.focused = false;
  }

  function submitText(event: SubmitEvent): void {
    event.preventDefault();
    if (memo.text.trim()) onanswer([memo.writing], memo.text.trim());
  }

  function submitMulti(): void {
    const typed = freeIndex !== -1 && memo.picked.includes(freeIndex) ? memo.text.trim() : undefined;
    onanswer([...memo.picked].sort((a, b) => a - b), typed);
  }

  function keys(event: KeyboardEvent): void {
    if (locked && !(event.key === "Escape" && !cancelLocked)) return;
    const card = (event.currentTarget as HTMLElement).closest(".card");
    const buttons = [...(card?.querySelectorAll<HTMLButtonElement>("button.option, button.advance") ?? [])];
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const digit = Number(event.key);
    if (Number.isInteger(digit) && digit >= 1 && digit <= rows.length) {
      event.preventDefault();
      pick(rows[digit - 1]!);
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const next = at === -1 ? 0 : (at + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) % buttons.length;
      buttons[next]?.focus();
    } else if (choice.multi && event.key === "Enter" && (memo.picked.length || next !== -1)) {
      // Space toggles an option, Enter submits, as checkboxes do.
      event.preventDefault();
      submitMulti();
    } else if (event.key === "Escape" && choice.cancellable) {
      event.preventDefault();
      oncancel();
    }
  }
</script>

<section class="card" class:folded aria-labelledby="choice-question" data-writing={memo.writing !== -1} bind:this={card}>
  <div class="body">
    <p class="asker">
      <span>{identity} is asking{#if choice.header}<span class="chip">{choice.header}</span>{/if}{#if locked}<span class="chip">{lockedLabel}</span>{/if}</span>
      {#if narrow}<button type="button" class="fold" aria-expanded={!folded} onclick={() => (memo.collapsed = !memo.collapsed)}>{folded ? "Show question" : "Hide question"}</button>{/if}
    </p>
  <h2 id="choice-question" class:peek={folded}>{choice.question || "Pick one"}</h2>
  {#if !folded}
  {#if tabs}
    <div class="tabs">
      {#if progress}<p class="progress">{progress}</p>{/if}
      <div class="tabrow" role="group" aria-label="Questions">
        {#each tabs.tabs as tab, index (index)}
          <button type="button" class="tab" class:current={index === tabs.current} aria-current={index === tabs.current ? "step" : undefined} aria-label={tab.answered ? `${tab.label}, answered` : tab.label} disabled={locked} onclick={() => ontab?.(index)}>
            {#if tab.answered}<span class="done" aria-hidden="true">✓</span>{/if}{tab.label}
          </button>
        {/each}
      </div>
    </div>
  {/if}
  {#if choice.multi}<p class="hint">Pick any, then {next === -1 ? "submit" : nextLabel.toLowerCase()}.</p>{/if}
  <ol>
    {#each choice.options as option, index (index)}
      {#if index !== advance && index !== next}
      <li>
        {#if memo.writing === index && !choice.multi}
          <form class="write" onsubmit={submitText}>
            <label for="choice-text" class="visually-hidden">Your own answer</label>
            <!-- svelte-ignore a11y_autofocus -->
            <input id="choice-text" bind:value={memo.text} placeholder={placeholder} autocomplete="off" onfocus={() => (memo.focused = true)} onblur={leave} autofocus />
            <button class="button primary" type="submit" disabled={!memo.text.trim()}>Answer</button>
          </form>
        {:else}
          <button class="option" disabled={locked} aria-pressed={choice.multi ? memo.picked.includes(index) : undefined} onclick={() => pick(index)} onkeydown={keys}>
            <span class="number" aria-hidden="true">{choice.multi && memo.picked.includes(index) ? "✓" : shown(index)}</span>
            <span class="text">
              <span class="label">{option.freeText ? "Something else…" : option.label}</span>
              {#if option.description}<span class="description">{option.description}</span>{/if}
            </span>
          </button>
          {#if choice.multi && memo.writing === index}
            <label for="choice-text" class="visually-hidden">Your own answer</label>
            <!-- svelte-ignore a11y_autofocus -->
            <input id="choice-text" class="multi-text" bind:value={memo.text} placeholder={placeholder} autocomplete="off" onfocus={() => (memo.focused = true)} onblur={leave} autofocus />
          {/if}
        {/if}
      </li>
      {/if}
    {/each}
  </ol>
  <p class="keys" aria-hidden="true">
    {choice.multi ? `1 to ${rows.length} to pick // Enter to ${next === -1 ? "submit" : nextLabel.toLowerCase()}` : `1 to ${choice.options.length} to answer`}{choice.cancellable ? " // Esc to cancel" : ""}
  </p>
  {/if}
  </div>
  {#if !folded}
  <div class="actions">
    {#if advance !== -1}
      <button class="button primary advance" disabled={locked} onclick={() => pick(advance)} onkeydown={keys}>{choice.options[advance]?.label}</button>
    {/if}
    {#if choice.multi && next !== -1}
      <button class="button primary advance" onclick={submitMulti} disabled={locked}>{nextLabel}</button>
    {:else if choice.multi}
      <button class="button primary" onclick={submitMulti} disabled={locked || memo.picked.length === 0}>Submit {memo.picked.length || ""}</button>
    {/if}
    {#if choice.cancellable}
      <button class="button" disabled={cancelLocked} onclick={oncancel} onkeydown={keys}>Cancel</button>
    {/if}
  </div>
  {/if}
</section>

<style>
  /* The question and options scroll inside the card, the buttons never do: the overlay bounds the height. */
  .card { margin: 0 12px; border-radius: 12px; border: 1px solid var(--brand); background: #17131f; display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
  .body {
    flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 14px 16px; display: flex; flex-direction: column; gap: 10px;
    /* Shadows that show only on the side with more to scroll to. */
    background:
      linear-gradient(#17131f 30%, transparent) top / 100% 24px local no-repeat,
      linear-gradient(transparent, #17131f 70%) bottom / 100% 24px local no-repeat,
      radial-gradient(farthest-side at 50% 0, rgb(0 0 0 / 0.55), transparent) top / 100% 10px scroll no-repeat,
      radial-gradient(farthest-side at 50% 100%, rgb(0 0 0 / 0.55), transparent) bottom / 100% 10px scroll no-repeat;
  }
  .asker { margin: 0; display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--brand); }
  .fold { flex: none; min-height: 44px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); letter-spacing: 0; text-transform: none; }
  .fold:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .tabs { display: flex; flex-direction: column; gap: 6px; }
  .progress { margin: 0; font-size: 12px; color: var(--muted); }
  .tabrow { display: flex; gap: 6px; overflow-x: auto; scrollbar-width: none; }
  .tab { flex: none; min-height: 36px; padding: 0 12px; display: inline-flex; align-items: center; gap: 6px; border-radius: 999px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); }
  .tab.current { border-color: var(--brand); background: #1d1729; }
  .tab:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .done { color: var(--brand); }
  @media (pointer: coarse) { .tab { min-height: 44px; } }
  .card.folded .body { padding: 6px 12px 10px; gap: 2px; overflow: hidden; background: none; }
  .peek { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 14px; }
  .chip { margin-left: 10px; padding: 2px 8px; border-radius: 999px; border: 1px solid #3a3350; color: var(--text-soft); text-transform: none; letter-spacing: 0; }
  h2 { margin: 0; font-size: 15px; font-weight: 500; line-height: 1.45; white-space: pre-line; overflow-wrap: anywhere; }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
  ol { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .option { width: 100%; min-height: 44px; display: flex; gap: 12px; align-items: flex-start; text-align: left; padding: 10px 12px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--surface); color: var(--text); }
  .option:hover, .option[aria-pressed="true"] { border-color: var(--brand); background: #1d1729; }
  /* Plain :focus too, since focus set by script may not count as visible. */
  .option:focus { outline: 2px solid var(--brand); outline-offset: 2px; border-color: var(--brand); background: #1d1729; }
  .number { flex: none; width: 22px; height: 22px; border-radius: 6px; display: inline-flex; align-items: center; justify-content: center; font: 12px var(--font-mono); color: var(--brand); border: 1px solid #3a3350; }
  [aria-pressed="true"] .number { background: var(--brand); color: var(--brand-ink); }
  .text { display: flex; flex-direction: column; gap: 2px; }
  .label { font-weight: 600; }
  .description { font-size: 13px; color: var(--muted); }
  .write { display: flex; gap: 8px; }
  .write input, .multi-text { flex: 1; min-height: 44px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--brand); background: var(--terminal); color: var(--text); font: 15px var(--font-body); }
  .multi-text { margin-top: 6px; width: 100%; box-sizing: border-box; }
  .actions { flex: none; display: flex; flex-wrap: wrap; gap: 8px; padding: 0 16px 14px; }
  .actions:empty { display: none; }
  .body:has(+ .actions:not(:empty)) { padding-bottom: 10px; }
  .keys { margin: 0; font: 12px var(--font-mono); color: var(--muted); }
  button:disabled { opacity: 0.5; cursor: default; }
  /* A phone's terminal is a few hundred pixels, so every row gives back what it can and stays a 44px target. */
  @media (max-width: 480px) {
    .card { margin: 0 8px; }
    .body { padding: 10px 12px; gap: 8px; }
    .keys { display: none; }
    ol { gap: 4px; }
    .option { padding: 6px 10px; gap: 10px; }
    .actions { padding: 0 12px 10px; }
    .body:has(+ .actions:not(:empty)) { padding-bottom: 8px; }
  }
</style>
