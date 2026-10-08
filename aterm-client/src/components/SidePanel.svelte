<script lang="ts" module>
  export type { SideTab } from "../lib/side-tab";
</script>

<script lang="ts">
  import BrowserPane from "./BrowserPane.svelte";
  import MessagesPanel from "./MessagesPanel.svelte";
  import TerminalPane from "./TerminalPane.svelte";
  import ViewsPanel from "./ViewsPanel.svelte";
  import { app, viewsFor } from "../lib/app.svelte";
  import type { PeerMessage, Session } from "../lib/protocol";
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
    onpick,
  }: {
    session: Session;
    messages: PeerMessage[];
    selfRole: string;
    colorOf: (role: string) => string;
    accent: string;
    tab?: SideTab;
    /** Fires for a tab a person picked, never for the one the role opened on. */
    onpick?: (tab: SideTab) => void;
  } = $props();

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

<aside class="panel" aria-label={`${session.identity}'s messages, views, browser, and terminal`}>
  <div class="tabs" role="tablist" aria-orientation="horizontal" tabindex="-1" onkeydown={tablistKeys}>
    {#each tabs as each (each.id)}
      <button
        role="tab"
        type="button"
        id={`side-tab-${each.id}`}
        aria-selected={tab === each.id}
        aria-controls="side-panel"
        tabindex={tab === each.id ? 0 : -1}
        onclick={() => {
          tab = each.id;
          onpick?.(each.id);
        }}
      >
        {each.label}
        {#if each.badge}<span class="badge" data-live={each.badge === "live"}>{each.badge}</span>{/if}
      </button>
    {/each}
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
  .tabs { display: flex; gap: 0; padding: 8px 8px 0; border-bottom: 1px solid var(--line); overflow-x: auto; }
  [role="tab"] { min-height: 44px; padding: 0 9px; border: none; border-bottom: 2px solid transparent; background: transparent; color: var(--muted); font-family: var(--font-display); font-weight: 600; font-size: 15px; display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
  [role="tab"][aria-selected="true"] { color: var(--text); border-bottom-color: var(--accent, var(--brand)); }
  .badge { font-family: var(--font-body); font-weight: 600; font-size: 11px; padding: 1px 7px; border-radius: 999px; background: var(--line); color: var(--text-soft); }
  .badge[data-live="true"] { background: color-mix(in srgb, var(--ok) 20%, transparent); color: var(--ok); }
  @media (max-width: 480px) {
    .tabs { gap: 0; padding: 4px 4px 0; }
    [role="tab"] { padding: 0 7px; font-size: 14px; }
  }
  .content { flex: 1; min-height: 0; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; }
  .content[data-tab="terminal"] { padding: 0; overflow: hidden; }
</style>
