<script lang="ts">
  import { MENU_KEYS, type ScreenMenu } from "../lib/screen-menu";

  let { menu, identity, onkey, locked = false, lockedLabel = "Read only" }: {
    menu: ScreenMenu;
    identity: string;
    /** One key for the seat's menu. The menu moves, and the next screen read redraws the card. */
    onkey: (key: string) => void;
    locked?: boolean;
    lockedLabel?: string;
  } = $props();

  const resume = $derived(menu.kind === "resume_picker");
  const heading = $derived(resume ? "Resume a conversation" : "MCP servers");
  const empty = $derived(resume ? "No conversation is highlighted on the screen. Up and Down move the highlight, or the picker is empty." : "The highlighted server is off the screen. Up and Down bring it back.");
</script>

<section class="card" aria-labelledby="menu-heading" data-menu={menu.kind}>
  <div class="body">
    <p class="asker"><span>{identity}{#if locked}<span class="chip">{lockedLabel}</span>{/if}</span></p>
    <h2 id="menu-heading">{heading}</h2>
    {#if menu.selected}
      <p class="selected" aria-live="polite"><span class="label">Highlighted</span> {menu.selected}</p>
      {#if menu.detail}<p class="detail">{menu.detail}</p>{/if}
    {:else}
      <p class="detail" role="status">{empty}</p>
    {/if}
  </div>
  <div class="actions">
    <button class="button" type="button" disabled={locked} aria-label="Highlight the row above" onclick={() => onkey(MENU_KEYS.up)}>Up</button>
    <button class="button" type="button" disabled={locked} aria-label="Highlight the row below" onclick={() => onkey(MENU_KEYS.down)}>Down</button>
    <button class="button primary" type="button" disabled={locked || !menu.selected} onclick={() => onkey(MENU_KEYS.open)}>{resume ? "Resume" : "Open"}</button>
    <button class="button" type="button" disabled={locked} onclick={() => onkey(MENU_KEYS.close)}>Close</button>
  </div>
</section>

<style>
  .card { margin: 0 12px; border-radius: 12px; border: 1px solid var(--brand); background: var(--raised); display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
  .body { flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 14px 16px 10px; display: flex; flex-direction: column; gap: 8px; }
  .asker { margin: 0; font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--brand); }
  .chip { margin-left: 10px; padding: 2px 8px; border-radius: 999px; border: 1px solid var(--chip-line); color: var(--text-soft); text-transform: none; letter-spacing: 0; }
  h2 { margin: 0; font-size: 15px; font-weight: 500; line-height: 1.45; }
  .selected { margin: 0; font: 14px var(--font-mono); overflow-wrap: anywhere; }
  .label { font: 12px var(--font-body); color: var(--text-soft); margin-right: 6px; }
  .detail { margin: 0; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  .actions { flex: none; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; padding: 0 16px 14px; }
  .button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  button:disabled { opacity: 0.5; cursor: default; }
  @media (max-width: 480px) {
    .card { margin: 0 8px; }
    .body { padding: 10px 12px 8px; }
    .actions { padding: 0 12px 10px; gap: 6px; }
  }
</style>
