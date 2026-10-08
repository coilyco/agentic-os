<script lang="ts" module>
  export type { SideTab } from "../lib/side-tab";
</script>

<script lang="ts">
  import BrowserPane from "./BrowserPane.svelte";
  import MessagesPanel from "./MessagesPanel.svelte";
  import TerminalPane from "./TerminalPane.svelte";
  import ViewsPanel from "./ViewsPanel.svelte";
  import { onMount } from "svelte";
  import { app, viewsFor } from "../lib/app.svelte";
  import type { PeerMessage, Session } from "../lib/protocol";
  import { escapeHides, openAfterPick } from "../lib/sheet";
  import type { SideTab } from "../lib/side-tab";
  import { terminalFor } from "../lib/terminals";
  import { tablistKeys } from "../lib/tabs";

  let {
    session,
    messages,
    selfRole,
    colorOf,
    accent,
    tab = $bindable("messages"),
    open = $bindable(false),
    onpick,
  }: {
    session: Session;
    messages: PeerMessage[];
    selfRole: string;
    colorOf: (role: string) => string;
    accent: string;
    tab?: SideTab;
    /** Below the split breakpoint the panel is a bottom sheet, and this is whether it is raised. */
    open?: boolean;
    /** Fires for a tab a person picked, never for the one the role opened on. */
    onpick?: (tab: SideTab) => void;
  } = $props();

  // Tracks the stylesheet's own breakpoint, so the sheet's controls exist exactly when the sheet does.
  let narrow = $state(false);
  onMount(() => {
    const query = matchMedia("(max-width: 1000px)");
    const read = () => (narrow = query.matches);
    read();
    query.addEventListener("change", read);
    return () => query.removeEventListener("change", read);
  });

  function pick(id: SideTab): void {
    open = openAfterPick(narrow, open, tab, id);
    tab = id;
    onpick?.(id);
  }

  function hide(): void {
    open = false;
    document.getElementById(`side-tab-${tab}`)?.focus();
  }

  function escape(event: KeyboardEvent): void {
    if (event.key !== "Escape" || event.defaultPrevented) return;
    if (!escapeHides(narrow, open, tab, (event.target as HTMLElement).closest(".head") !== null)) return;
    event.preventDefault();
    hide();
  }

  const viewCount = $derived(viewsFor(session.id).length);
  const browserLive = $derived(app.browsers[session.id]?.state === "live");
  const shellLive = $derived(terminalFor(app.terminals, session.id)?.exit === null);
  const tabs = $derived([
    { id: "messages" as const, label: "Messages", badge: messages.length ? String(messages.length) : "" },
    { id: "views" as const, label: "Views", badge: viewCount ? String(viewCount) : "" },
    { id: "browser" as const, label: "Browser", badge: browserLive ? "live" : "" },
    { id: "terminal" as const, label: "Terminal", badge: shellLive ? "live" : "" },
  ]);
</script>

<!-- Escape is heard here only because the keys it answers come from controls inside this panel. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<aside
  class="panel"
  data-open={open}
  aria-label={`${session.identity}'s messages, views, browser, and terminal`}
  onkeydown={escape}
>
  <div class="head">
    <div class="tabs" role="tablist" aria-orientation="horizontal" tabindex="-1" onkeydown={tablistKeys}>
      {#each tabs as each (each.id)}
        <button
          role="tab"
          type="button"
          id={`side-tab-${each.id}`}
          aria-selected={tab === each.id}
          aria-controls="side-panel"
          tabindex={tab === each.id ? 0 : -1}
          onclick={() => pick(each.id)}
        >
          {each.label}
          {#if each.badge}<span class="badge" data-live={each.badge === "live"}><span class="word">{each.badge}</span></span>{/if}
        </button>
      {/each}
    </div>
    {#if narrow && open}
      <button type="button" class="hide" aria-label="Hide the side panel" aria-controls="side-panel" onclick={hide}>
        <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M3 6l5 5 5-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" /></svg>
      </button>
    {/if}
  </div>
  <div id="side-panel" class="content" data-tab={tab} role="tabpanel" aria-labelledby={`side-tab-${tab}`} tabindex="-1">
    {#if tab === "messages"}
      <MessagesPanel {messages} {selfRole} {colorOf} />
    {:else if tab === "views"}
      <ViewsPanel {session} />
    {:else if tab === "browser"}
      <BrowserPane {session} />
    {:else}
      <TerminalPane {session} {accent} />
    {/if}
  </div>
</aside>

<style>
  .panel { display: flex; flex-direction: column; min-height: 0; min-width: 0; border-left: 1px solid var(--line); }
  .head { display: flex; align-items: flex-end; border-bottom: 1px solid var(--line); }
  .tabs { display: flex; flex: 1; min-width: 0; gap: 0; padding: 8px 8px 0; overflow-x: auto; }
  [role="tab"] { min-height: 44px; padding: 0 9px; border: none; border-bottom: 2px solid transparent; background: transparent; color: var(--muted); font-family: var(--font-display); font-weight: 600; font-size: 15px; display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
  [role="tab"][aria-selected="true"] { color: var(--text); border-bottom-color: var(--accent, var(--brand)); }
  .badge { font-family: var(--font-body); font-weight: 600; font-size: 11px; padding: 1px 7px; border-radius: 999px; background: var(--line); color: var(--text-soft); }
  .badge[data-live="true"] { background: color-mix(in srgb, var(--ok) 20%, transparent); color: var(--ok); }
  .hide { flex: none; width: 44px; min-height: 44px; display: grid; place-items: center; border: none; background: transparent; color: var(--text-soft); }
  .content { flex: 1; min-height: 0; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; }
  .content[data-tab="terminal"] { padding: 0; overflow: hidden; }
  @media (max-width: 480px) {
    .tabs { gap: 0; padding: 4px 4px 0; }
    [role="tab"] { padding: 0 6px; font-size: 14px; }
    /* Four tabs and the hide button share 320px, so a badge sits on its tab's corner instead of widening the row. "live" is a dot, still in the accessible name. */
    [role="tab"] { position: relative; }
    .badge { position: absolute; top: 1px; right: -2px; padding: 0 5px; font-size: 10px; line-height: 14px; }
    .badge[data-live="true"] { top: 5px; right: 1px; width: 8px; height: 8px; padding: 0; background: var(--ok); }
    .badge[data-live="true"] .word { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  }
  @media (max-width: 360px) {
    [role="tab"] { padding: 0 5px; font-size: 13px; }
  }
  /* A bottom sheet over the seat's terminal. Its tab strip always shows, and SessionPanel keeps that strip's height clear of the composer.
     Fixed to the window, since the shell's own sidebar and header can leave the page body too short to read a sheet in. */
  @media (max-width: 1000px) {
    .panel { position: fixed; left: 0; right: 0; bottom: 0; z-index: 5; height: var(--strip); border-left: none; border-top: 1px solid var(--line); background: var(--ground); }
    .head { height: calc(var(--strip) - env(safe-area-inset-bottom)); flex: none; }
    .panel[data-open="false"] .content { display: none; }
    .panel[data-open="true"] { height: min(80dvh, calc(100dvh - 72px)); box-shadow: 0 -8px 16px rgb(0 0 0 / 0.35); }
  }
</style>
