<script lang="ts">
  import type { Choice } from "../lib/choices";

  let { choice, identity, onanswer, oncancel }: {
    choice: Choice;
    identity: string;
    /** Indexes of the picked options, and the typed text when "Type something." was one. */
    onanswer: (picks: number[], text?: string) => void;
    oncancel: () => void;
  } = $props();

  let writing = $state(-1);
  let text = $state("");
  let picked = $state<number[]>([]);
  const freeIndex = $derived(choice.options.findIndex((option) => option.freeText));

  function pick(index: number): void {
    if (choice.multi) {
      picked = picked.includes(index) ? picked.filter((each) => each !== index) : [...picked, index];
      if (choice.options[index]?.freeText) writing = picked.includes(index) ? index : -1;
      return;
    }
    if (choice.options[index]?.freeText) {
      writing = index;
      return;
    }
    onanswer([index]);
  }

  function submitText(event: SubmitEvent): void {
    event.preventDefault();
    if (text.trim()) onanswer([writing], text.trim());
  }

  function submitMulti(): void {
    const typed = freeIndex !== -1 && picked.includes(freeIndex) ? text.trim() : undefined;
    onanswer([...picked].sort((a, b) => a - b), typed);
  }

  function keys(event: KeyboardEvent): void {
    const card = (event.currentTarget as HTMLElement).closest(".card");
    const buttons = [...(card?.querySelectorAll<HTMLButtonElement>("button.option") ?? [])];
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const digit = Number(event.key);
    if (Number.isInteger(digit) && digit >= 1 && digit <= choice.options.length) {
      event.preventDefault();
      pick(digit - 1);
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const next = at === -1 ? 0 : (at + (event.key === "ArrowDown" ? 1 : -1) + buttons.length) % buttons.length;
      buttons[next]?.focus();
    } else if (event.key === "Escape" && choice.cancellable) {
      event.preventDefault();
      oncancel();
    }
  }
</script>

<section class="card" aria-labelledby="choice-question">
  <p class="asker">{identity} is asking{#if choice.header}<span class="chip">{choice.header}</span>{/if}</p>
  <h2 id="choice-question">{choice.question || "Pick one"}</h2>
  {#if choice.multi}<p class="hint">Pick any, then submit.</p>{/if}
  <ol>
    {#each choice.options as option, index (index)}
      <li>
        {#if writing === index && !choice.multi}
          <form class="write" onsubmit={submitText}>
            <label for="choice-text" class="visually-hidden">Your own answer</label>
            <!-- svelte-ignore a11y_autofocus -->
            <input id="choice-text" bind:value={text} placeholder="Type or dictate your own answer" autocomplete="off" autofocus />
            <button class="button primary" type="submit" disabled={!text.trim()}>Answer</button>
          </form>
        {:else}
          <button class="option" aria-pressed={choice.multi ? picked.includes(index) : undefined} onclick={() => pick(index)} onkeydown={keys}>
            <span class="number" aria-hidden="true">{choice.multi && picked.includes(index) ? "✓" : index + 1}</span>
            <span class="text">
              <span class="label">{option.freeText ? "Something else…" : option.label}</span>
              {#if option.description}<span class="description">{option.description}</span>{/if}
            </span>
          </button>
          {#if choice.multi && writing === index}
            <label for="choice-text" class="visually-hidden">Your own answer</label>
            <!-- svelte-ignore a11y_autofocus -->
            <input id="choice-text" class="multi-text" bind:value={text} placeholder="Type or dictate your own answer" autocomplete="off" autofocus />
          {/if}
        {/if}
      </li>
    {/each}
  </ol>
  <div class="actions">
    {#if choice.multi}
      <button class="button primary" onclick={submitMulti} disabled={picked.length === 0}>Submit {picked.length || ""}</button>
    {/if}
    {#if choice.cancellable}
      <button class="button" onclick={oncancel} onkeydown={keys}>Cancel</button>
    {/if}
  </div>
</section>

<style>
  .card { margin: 0 12px; padding: 14px 16px; border-radius: 12px; border: 1px solid var(--brand); background: #17131f; display: flex; flex-direction: column; gap: 10px; max-height: 50vh; overflow-y: auto; }
  .asker { margin: 0; font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--brand); }
  .chip { margin-left: 10px; padding: 2px 8px; border-radius: 999px; border: 1px solid #3a3350; color: var(--text-soft); text-transform: none; letter-spacing: 0; }
  h2 { margin: 0; font-size: 15px; font-weight: 500; line-height: 1.45; white-space: pre-line; overflow-wrap: anywhere; }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
  ol { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .option { width: 100%; min-height: 44px; display: flex; gap: 12px; align-items: flex-start; text-align: left; padding: 10px 12px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--surface); color: var(--text); }
  .option:hover, .option:focus-visible, .option[aria-pressed="true"] { border-color: var(--brand); background: #1d1729; }
  .number { flex: none; width: 22px; height: 22px; border-radius: 6px; display: inline-flex; align-items: center; justify-content: center; font: 12px var(--font-mono); color: var(--brand); border: 1px solid #3a3350; }
  [aria-pressed="true"] .number { background: var(--brand); color: var(--brand-ink); }
  .text { display: flex; flex-direction: column; gap: 2px; }
  .label { font-weight: 600; }
  .description { font-size: 13px; color: var(--muted); }
  .write { display: flex; gap: 8px; }
  .write input, .multi-text { flex: 1; min-height: 44px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--brand); background: var(--terminal); color: var(--text); font: 15px var(--font-body); }
  .multi-text { margin-top: 6px; width: 100%; box-sizing: border-box; }
  .actions { display: flex; gap: 8px; }
  button:disabled { opacity: 0.5; cursor: default; }
</style>
