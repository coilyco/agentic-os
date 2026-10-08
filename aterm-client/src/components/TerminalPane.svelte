<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import Terminal from "./Terminal.svelte";
  import { app, closeTerminal, colorOf, openTerminal } from "../lib/app.svelte";
  import type { Session } from "../lib/protocol";
  import { exitText, paneState, paneText, shouldOpen, terminalFor } from "../lib/terminals";

  let { session, accent }: { session: Session; accent: string } = $props();

  const view = $derived(
    paneState({
      support: app.terminalSupport,
      loaded: app.terminalsLoaded,
      typing: app.typing,
      entry: terminalFor(app.terminals, session.id),
      opening: app.terminalOpening[session.id] === true,
      refusal: app.terminalRefusals[session.id],
      closed: app.terminalClosed.includes(session.id),
    }),
  );
  const text = $derived(paneText(view, session.identity));
  const watchOnly = $derived(app.typing.allowed ? null : app.typing.reason);

  // Choosing the Terminal tab is asking for a shell, so it opens one unless Kai closed it.
  $effect(() => {
    if (shouldOpen(view)) untrack(() => openTerminal(session.id));
  });

  let pane: HTMLDivElement;
  let primary = $state<HTMLButtonElement>();
  // Focus follows a button Kai pressed, never a tab she only arrived on.
  let focusShell = false;

  $effect(() => {
    if (view.kind === "live" && focusShell) {
      focusShell = false;
      void tick().then(focusTerminal);
    }
  });

  function focusTerminal(): void {
    pane?.querySelector<HTMLTextAreaElement>("textarea")?.focus();
  }

  function reopen(): void {
    focusShell = true;
    openTerminal(session.id);
  }

  function dismiss(): void {
    closeTerminal(session.id);
    void tick().then(() => primary?.focus());
  }

  // The shell takes Tab and the arrows, so the keyboard needs a way out that it does not take.
  onMount(() => {
    const move = (event: KeyboardEvent) => {
      if (!event.ctrlKey || !event.altKey || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) return;
      event.preventDefault();
      event.stopPropagation();
      if (event.key === "ArrowLeft") document.getElementById("composer-input")?.focus();
      else focusTerminal();
    };
    window.addEventListener("keydown", move, true);
    return () => window.removeEventListener("keydown", move, true);
  });
</script>

<div class="pane" bind:this={pane}>
  {#if view.kind === "live" || view.kind === "exited"}
    <div class="bar">
      <span class="name">Shell beside {session.identity}</span>
      <button type="button" class="quiet" onclick={dismiss}>Close</button>
    </div>
    {#key view.entry.id}
      <Terminal connection={app.connection!} sessionId={view.entry.id} label={`shell beside ${session.identity}`} {accent} messages={[]} {colorOf} locked={!app.typing.allowed || view.kind === "exited"} />
    {/key}
    <p class="hint mono">Ctrl+Alt+Left returns to {session.identity}</p>
    <!-- Present from the first frame, since a status inserted at exit is not announced. -->
    <div role="status">
      {#if watchOnly && view.kind === "live"}
        <p class="note">This browser can watch the shell but not type into it ({watchOnly}).</p>
      {:else if view.kind === "exited"}
        <div class="note">
          <span>{exitText(view.code)}</span>
          <button type="button" bind:this={primary} onclick={reopen}>Open a new terminal</button>
        </div>
      {/if}
    </div>
  {:else if text}
    <div class="empty" role={view.kind === "refused" ? "alert" : "status"} aria-busy={view.kind === "opening" || view.kind === "waiting"}>
      <h2>{text.title}</h2>
      <p>{text.body}</p>
      {#if view.kind === "ready" || view.kind === "closed"}
        <button type="button" bind:this={primary} onclick={reopen}>Open terminal</button>
      {:else if view.kind === "refused"}
        <button type="button" bind:this={primary} onclick={reopen}>Try again</button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .pane { flex: 1; display: flex; flex-direction: column; min-height: 0; min-width: 0; }
  .bar { display: flex; align-items: center; justify-content: space-between; gap: 8px 12px; padding: 0 12px; border-bottom: 1px solid var(--line); }
  .name { font-family: var(--font-display); font-weight: 600; font-size: 14px; }
  .hint { margin: 0; padding: 4px 12px; color: var(--muted); font-size: 11px; border-top: 1px solid var(--line); }
  button { min-height: 44px; padding: 0 14px; border-radius: 6px; border: 1px solid var(--accent, var(--brand)); background: transparent; color: var(--text); font-family: var(--font-display); font-weight: 600; font-size: 14px; }
  button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .quiet { border-color: var(--line); color: var(--text-soft); }
  .note { margin: 0; padding: 6px 12px; display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; border-top: 1px solid var(--warn); background: var(--warn-fill); color: var(--warn-text); font-size: 13px; }
  .empty { padding: 20px 16px; display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
  .empty h2 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 17px; }
  .empty p { margin: 0 0 8px; color: var(--muted); font-size: 13px; max-width: 46ch; }
  .pane :global(.term-host) { min-height: 200px; }
</style>
