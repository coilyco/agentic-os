<script lang="ts">
  import { onMount } from "svelte";
  import Creature from "../components/Creature.svelte";
  import ChoiceCard from "../components/ChoiceCard.svelte";
  import CloseSeat from "../components/CloseSeat.svelte";
  import Composer from "../components/Composer.svelte";
  import RelayNotice from "../components/RelayNotice.svelte";
  import SidePanel from "../components/SidePanel.svelte";
  import SplitHandle from "../components/SplitHandle.svelte";
  import Terminal from "../components/Terminal.svelte";
  import { app, asksShownOn, colorOf, layout, messagesFor, needsKaiIds, placeNow, reconnecting, relayedTo } from "../lib/app.svelte";
  import { readTabs, tabKeys, type TabReading } from "../lib/dialog-tabs";
  import type { TabBarCells } from "../lib/screen";
  import { askScope, cardMemo, forgetCardMemos, memoKey, screenScope, tickedRows } from "../lib/card-memo.svelte";
  import { askAnswer, CANCEL, choiceFromAsk, choiceKey, detectChoice, keysFor, keysForPicks, type Choice } from "../lib/choices";
  import { contextPercent, contextText } from "../lib/context";
  import { isBlank } from "../lib/screen";
  import type { Session } from "../lib/protocol";
  import type { Role } from "../lib/roster";
  import { sessionCode } from "../lib/sessions";
  import { needsPasskey } from "../lib/typing";
  import { loadOverrides, sideTabFor, storeOverride, type SideTab } from "../lib/side-tab";
  import { loadSide, loadSideOpen, storeSide, storeSideOpen } from "../lib/split";
  import { mark } from "../lib/tap-trace";
  import { wantsFind } from "../lib/terminal-extras";

  let { role, session }: { role: Role; session: Session } = $props();
  // On a phone the switcher bar names the seat, so the header keeps only a heading for screen readers.
  let narrow = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px)").matches);
  // A phone on its side folds the header too: the rail names the seat, and the height goes to the terminal.
  const phone = $derived(narrow || layout.reading);
  // Above 1000px the side panel sits beside the seat, and a person opens it. Below, it is a sheet that starts put away.
  let split = $state(typeof matchMedia === "function" && matchMedia("(min-width: 1001px)").matches);
  let sideOpen = $state(loadSideOpen());
  function toggleSide(): void {
    sideOpen = !sideOpen;
    storeSideOpen(sideOpen);
  }
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const wide = matchMedia("(min-width: 1001px)");
    const read = () => {
      narrow = query.matches;
      split = wide.matches;
    };
    read();
    query.addEventListener("change", read);
    wide.addEventListener("change", read);
    return () => {
      query.removeEventListener("change", read);
      wide.removeEventListener("change", read);
    };
  });
  const messages = $derived(messagesFor(session));
  let seatTerminal = $state<{ openFind: () => void }>();
  const findChord = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform) ? "Cmd+F" : "Ctrl+Shift+F";
  // Cmd+F from the composer or the card finds in the seat's output, as it does from the terminal.
  function findKey(event: KeyboardEvent): void {
    if (event.defaultPrevented || !wantsFind(event)) return;
    if (!(event.target instanceof Element) || !event.target.closest(".work")) return;
    event.preventDefault();
    seatTerminal?.openFind();
  }
  // A structured ask beats a menu read off the screen: it is the seat's own words.
  const ask = $derived(asksShownOn(session.id)[0]);
  // A relayed card is the worker's question, so the card names the worker and not this seat.
  const asker = $derived((ask && app.sessions.find((each) => each.id === ask.session)?.identity) || session.identity);
  const relayed = $derived(relayedTo(session.id));
  // Opens on what the role looks at beside its seat, unless Kai picked another tab for it before.
  // The panel is keyed by session, so the role it starts with is the role it keeps.
  // svelte-ignore state_referenced_locally
  // A browser nobody has opened a page in is an empty state, so it is not what a seat opens on.
  let side = $state<SideTab>(sideTabFor(role.slug, loadOverrides(), { browser: !!app.connection?.browser && (app.browsers[session.id]?.state ?? "none") !== "none" }));
  // Null is the panel's own width, which the browser widens to half.
  let sideWidth = $state<number | null>(loadSide());
  function resize(width: number | null): void {
    sideWidth = width;
    storeSide(width);
  }
  // Below 1000px the side panel is a bottom sheet over the seat's terminal, and starts put away.
  let raised = $state(false);
  // A question waiting sits at the foot of the terminal, which a raised sheet would cover.
  $effect(() => {
    if (dialogOpen) raised = false;
  });
  // A click on a page card puts this seat's browser pane on screen, and the pane loads the page (COI-3258).
  $effect(() => {
    if (app.pageToShow?.sessionId !== session.id) return;
    side = "browser";
    sideOpen = true;
    raised = true;
  });
  const page = $derived(app.browsers[session.id]);
  // The browser needs room to be read, so a page takes half the width. Its empty states do not.
  const wide = $derived(side === "browser" && page !== undefined && page.state !== "none");
  let choice = $state<Choice | null>(null);
  // The composer holds its send, and focus no longer hides the strip: leaving the field would bring it back and move the card under a tap.
  const dialogOpen = $derived(!!(ask || choice));
  let shownKey = $state("");
  // A seat that is still starting has printed nothing, so the pane says so instead of sitting empty.
  let blank = $state(true);
  // Hidden once answered, until the screen shows a different menu.
  let answered = "";
  // A redraw arrives in pieces, so one frame without the menu is not it closing.
  let hideTimer: ReturnType<typeof setTimeout> | undefined;
  // Whether the last screen read held a menu, so a trace notes only the change.
  let menuSeen = false;
  const HIDE_AFTER_MS = 400;
  const FORGET_AFTER_MS = 5000;
  let forgetTimer: ReturnType<typeof setTimeout> | undefined;

  const keyOf = choiceKey;

  // The tabs of a multi-question dialog, and which is current, as the screen shows them.
  let dialogTabs = $state<TabReading | null>(null);

  function goToTab(index: number): void {
    if (!dialogTabs) return;
    const chunks = tabKeys(dialogTabs.current, index, dialogTabs.tabs.length);
    chunks.forEach((chunk, at) => setTimeout(() => app.connection?.input(session.id, chunk), at * KEY_GAP_MS));
  }

  function readScreen(rows: string[], tabBar: TabBarCells | null): void {
    blank = isBlank(rows);
    const found = detectChoice(rows);
    if ((found !== null) !== menuSeen) mark("screen", found ? "menu" : "no-menu");
    menuSeen = found !== null;
    dialogTabs = found && tabBar ? readTabs(tabBar.text, new Set(tabBar.highlighted)) : null;
    if (!found) {
      hideTimer ??= setTimeout(() => {
        choice = null;
        shownKey = "";
        answered = "";
        hideTimer = undefined;
        // A dialog that flickers away keeps its draft for a while. One that stays gone does not.
        forgetTimer = setTimeout(() => forgetCardMemos(screenScope(session.id)), FORGET_AFTER_MS);
      }, HIDE_AFTER_MS);
      return;
    }
    clearTimeout(forgetTimer);
    clearTimeout(hideTimer);
    hideTimer = undefined;
    const key = keyOf(found);
    if (key === answered) return;
    // Our own keys redraw the same question while they land: a checkbox ticks, a typed row fills. That is not a new menu.
    if (performance.now() < holdUntil && found.question === heldQuestion) return;
    if (key !== shownKey) {
      shownKey = key;
      choice = found;
    } else if (choice) {
      if (choice.cursor !== found.cursor) choice.cursor = found.cursor;
      // A counter in the question ticks in place, without making a new card.
      if (choice.question !== found.question) choice.question = found.question;
    }
  }

  const KEY_GAP_MS = 150;

  const HOLD_AFTER_KEYS_MS = 700;
  let holdUntil = 0;
  let heldQuestion = "";

  function answer(chunks: string[]): void {
    if (!choice) return;
    mark("answer", "screen-card");
    answered = shownKey;
    forgetCardMemos(screenScope(session.id));
    heldQuestion = choice.question;
    holdUntil = performance.now() + chunks.length * KEY_GAP_MS + HOLD_AFTER_KEYS_MS;
    chunks.forEach((chunk, index) => setTimeout(() => app.connection?.input(session.id, chunk), index * KEY_GAP_MS));
    choice = null;
    shownKey = "";
  }
</script>

<svelte:window onkeydown={findKey} />

<section class="session" style:--accent={role.color}>
  <header class:folded={phone}>
    {#if !phone}<Creature role={role.slug} color={role.color} size={40} />{/if}
    <h1 class:visually-hidden={phone}>{session.identity}</h1>
    {#if !phone}
      <span class="role">{role.displayName} // {session.seat}{sessionCode(session) ? ` // ${sessionCode(session)}` : ""}</span>
      {#if session.context}
        {@const percent = contextPercent(session.context)}
        <span class="context mono" data-source={session.context.source} title={`context read from ${session.context.source}`}>
          {contextText(session.context)}
          {#if percent !== null}<span class="bar" aria-hidden="true" style:--fill={`${Math.min(percent, 100)}%`}></span>{/if}
        </span>
      {/if}
      <span class="state mono">{session.state}</span>
    {/if}
    {#if !phone}
      <button type="button" class="head-button" aria-label="Find in output" title={`Find in output (${findChord})`} onclick={() => seatTerminal?.openFind()}>Find</button>
    {/if}
    <CloseSeat {session} needsKai={needsKaiIds().has(session.id)} where="view" />
    {#if split}
      <button type="button" class="head-button" aria-expanded={sideOpen} aria-controls="side-panel" onclick={toggleSide}>
        Side panel{#if messages.length}<span class="count">{messages.length}</span>{/if}
      </button>
    {/if}
    {#if session.degraded.length}
      <p class="degraded">Started without {session.degraded.join(", ")}. agent-compose skipped these steps at launch, so this seat may be missing what they set up.</p>
    {/if}
  </header>
  <div class="body" class:wide class:away={split && !sideOpen} data-composing={app.composing && !dialogOpen} data-reading={layout.reading && !split} style:--side={sideWidth ? `${sideWidth}px` : undefined}>
    <div class="work">
      {#if app.connection}
        {#key session.id}
          <!-- The card overlays the terminal so it never resizes it, which would make the harness redraw its menu. -->
          <div class="screen" data-link={app.link.state}>
            <Terminal bind:this={seatTerminal} connection={app.connection} sessionId={session.id} label={session.identity} accent={role.color} {messages} {colorOf} onscreen={readScreen} cropHarness covered={!!(ask || choice)} locked={!app.typing.allowed || reconnecting()} />
            {#if session.starting && blank}
              <p class="starting" role="status">Starting {session.identity} on {session.seat}. Its first screen shows here as soon as {session.seat} has opened.</p>
            {/if}
            {#if ask}
              {@const asked = choiceFromAsk(ask)}
              {#key ask.id}
                <div class="overlay" data-card="daemon">
                  <ChoiceCard
                    choice={asked}
                    memo={cardMemo(memoKey(askScope(ask.id), asked.options), [])}
                    identity={asker}
                    focusToken={app.focusAsk === session.id ? app.focusCard : 0}
                    onfocused={() => (app.focusAsk = null)}
                    locked={!app.typing.allowed || reconnecting()}
                    lockedLabel={reconnecting() ? "Reconnecting" : "Read only"}
                    cancelLocked={needsPasskey(app.typing) || reconnecting()}
                    onanswer={(picks, text) => {
                      mark("answer", "daemon-card");
                      const reply = askAnswer(ask, picks, text);
                      forgetCardMemos(askScope(ask.id));
                      app.connection?.answer(ask.id, reply.picks, reply.text);
                    }}
                    oncancel={() => {
                      forgetCardMemos(askScope(ask.id));
                      app.connection?.cancelAsk(ask.id);
                    }}
                  />
                </div>
              {/key}
            {:else if choice}
              {@const current = choice}
              {#key shownKey}
                <div class="overlay" data-card="screen">
                  <ChoiceCard choice={current} tabs={dialogTabs} ontab={goToTab} onmove={() => app.connection?.input(session.id, "\x1b[B")} memo={cardMemo(memoKey(screenScope(session.id), current.options), tickedRows(current.options))} identity={session.identity} locked={!app.typing.allowed || reconnecting()} lockedLabel={reconnecting() ? "Reconnecting" : "Read only"} onanswer={(picks, text) => answer(current.multi ? keysForPicks(current, picks, text) : keysFor(current, picks[0] ?? 0, text))} oncancel={() => answer(CANCEL)} />
                </div>
              {/key}
            {/if}
          </div>
          {#if relayed}<RelayNotice seat={relayed} />{/if}
          {#if app.inputNotice}
            <p class="input-notice" role="alert">
              <span>{app.inputNotice}</span>
              <button type="button" onclick={() => (app.inputNotice = "")}>Dismiss</button>
            </p>
          {/if}
          <Composer
            connection={app.connection}
            {session}
            sessions={app.sessions}
            typing={app.typing}
            offline={reconnecting()}
            {dialogOpen}
            draft={app.drafts[session.id] ?? ""}
            ondraft={(text) => (text ? (app.drafts[session.id] = text) : delete app.drafts[session.id])}
            onsent={() => (app.inputNotice = "")}
            onfocuschange={(focused) => (app.composing = focused)}
            place={placeNow()}
            working={session.state === "working" && !blank}
          />
        {/key}
      {/if}
    </div>
    {#if !split || sideOpen}
      <SplitHandle onchange={resize} onreset={() => resize(null)} />
      <SidePanel {session} {messages} selfRole={role.slug} {colorOf} accent={role.color} bind:tab={side} bind:open={raised} onpick={(tab) => storeOverride(role.slug, tab)} />
    {/if}
  </div>
</section>

<style>
  .session { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
  header { flex: none; min-height: 64px; padding: 8px 24px; display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; border-bottom: 3px solid var(--accent); background: color-mix(in srgb, var(--accent) 6%, var(--ground)); }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 22px; }
  .role { font-size: 14px; color: color-mix(in srgb, var(--accent) 55%, white); }
  .degraded { flex-basis: 100%; margin: 0; padding: 6px 10px; border-radius: 6px; border: 1px solid var(--warn); background: var(--warn-fill); color: var(--warn-text); font-size: 13px; }
  .head-button { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 0 14px; border-radius: 999px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); }
  .head-button[aria-expanded="true"] { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 16%, var(--ground)); }
  .head-button:hover { border-color: var(--accent); }
  .head-button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .count { min-width: 20px; height: 20px; padding: 0 6px; box-sizing: border-box; border-radius: 10px; background: var(--accent); color: var(--ground); font: 700 12px/20px var(--font-body); text-align: center; }
  .state { margin-left: auto; font-size: 12px; padding: 4px 10px; border-radius: 999px; border: 1px solid var(--accent); }
  .context { margin-left: auto; font-size: 12px; padding: 4px 10px; border-radius: 999px; border: 1px solid var(--line); white-space: nowrap; }
  .context + .state { margin-left: 0; }
  .bar { display: inline-block; position: relative; width: 48px; height: 6px; margin-left: 8px; border-radius: 3px; overflow: hidden; vertical-align: middle; background: var(--line); }
  .bar::after { content: ""; position: absolute; inset: 0 auto 0 0; width: var(--fill); background: var(--accent); }
  .work { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
  .screen { position: relative; flex: 1 1 auto; display: flex; flex-direction: column; min-height: 0; }
  .screen[data-link="reconnecting"] :global(.xterm) { filter: grayscale(1); opacity: 0.7; }
  .input-notice { margin: 0; padding: 8px 12px; display: flex; align-items: center; justify-content: space-between; gap: 8px 12px; flex-wrap: wrap; border-top: 1px solid var(--danger); background: var(--danger-fill); color: var(--danger-text); font-size: 14px; }
  .input-notice button { min-height: 44px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--danger); background: transparent; color: var(--danger-text); }
  .starting { position: absolute; inset: 0; margin: 0; padding: 24px; display: grid; place-content: center; text-align: center; color: var(--muted); pointer-events: none; }
  .overlay { position: absolute; left: 0; right: 0; bottom: 0; max-height: 92%; display: flex; flex-direction: column; justify-content: flex-end; padding-bottom: 10px; background: linear-gradient(to top, var(--terminal) 70%, transparent); }
  .body { --side: 360px; --strip: calc(44px + env(safe-area-inset-bottom)); flex: 1; display: grid; grid-template-columns: minmax(0, 1fr) 12px var(--side); min-height: 0; }
  .body.wide { --side: minmax(360px, 50%); }
  .body.away { grid-template-columns: minmax(0, 1fr); }
  .body :global(.panel) { border-left: none; }
  header.folded { min-height: 0; padding: 0; border-bottom-width: 2px; }
  /* A phone: the seat's name and state on one row, its role and context on the next, so the header does not cost the terminal a third of the screen. */
  @media (max-width: 720px) {
    /* The header gives its two rows back. A degraded start still says so. */
    header { min-height: 0; padding: 0; border-bottom-width: 2px; }
    .degraded { margin: 4px 8px; font-size: 12px; }
    /* The card leaves the seat's output a readable slice. While a text box is open the keyboard takes the rest, so it may use all of it. */
    .overlay { max-height: 58%; padding-bottom: 6px; }
    .overlay:has(:global([data-writing="true"])) { max-height: 100%; }
    /* The tab strip gives way to the keyboard, so typing has the whole screen but the seat's text. */
    .body[data-composing="true"] :global(.panel) { display: none; }
    .body[data-composing="true"] { --strip: 0px; }
  }
  @media (max-width: 1000px) {
    /* The sheet floats over this row, so the strip's height stays clear of the composer and the terminal never resizes as it rises. */
    .body, .body.wide { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(320px, 1fr); padding-bottom: var(--strip); min-height: calc(320px + var(--strip)); }
    /* A phone on its side has no strip to keep clear, and its height is too scarce to reserve any. */
    .body[data-reading="true"] { --strip: 0px; }
  }
</style>
