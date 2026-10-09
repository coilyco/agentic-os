<script lang="ts">
  import { onMount } from "svelte";
  import Creature from "../components/Creature.svelte";
  import ChoiceCard from "../components/ChoiceCard.svelte";
  import Composer from "../components/Composer.svelte";
  import SidePanel from "../components/SidePanel.svelte";
  import SplitHandle from "../components/SplitHandle.svelte";
  import Terminal from "../components/Terminal.svelte";
  import { app, asksFor, colorOf, messagesFor, placeNow, reconnecting } from "../lib/app.svelte";
  import { askAnswer, CANCEL, choiceFromAsk, detectChoice, keysFor, type Choice } from "../lib/choices";
  import { contextPercent, contextText } from "../lib/context";
  import { isBlank } from "../lib/screen";
  import type { Session } from "../lib/protocol";
  import type { Role } from "../lib/roster";
  import { sessionCode } from "../lib/sessions";
  import { needsPasskey } from "../lib/typing";
  import { loadOverrides, sideTabFor, storeOverride, type SideTab } from "../lib/side-tab";
  import { loadSide, storeSide } from "../lib/split";

  let { role, session }: { role: Role; session: Session } = $props();
  // On a phone the switcher bar names the seat, so the header keeps only a heading for screen readers.
  let phone = $state(typeof matchMedia === "function" && matchMedia("(max-width: 720px)").matches);
  onMount(() => {
    const query = matchMedia("(max-width: 720px)");
    const read = () => (phone = query.matches);
    read();
    query.addEventListener("change", read);
    return () => query.removeEventListener("change", read);
  });
  const messages = $derived(messagesFor(session));
  // A structured ask beats a menu read off the screen: it is the seat's own words.
  const ask = $derived(asksFor(session.id)[0]);
  // Opens on what the role looks at beside its seat, unless Kai picked another tab for it before.
  // The panel is keyed by session, so the role it starts with is the role it keeps.
  // svelte-ignore state_referenced_locally
  let side = $state<SideTab>(sideTabFor(role.slug, loadOverrides()));
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
    if (ask || choice) raised = false;
  });
  const page = $derived(app.browsers[session.id]);
  // The browser needs room to be read, so a page takes half the width. Its empty states do not.
  const wide = $derived(side === "browser" && page !== undefined && page.state !== "none");
  let choice = $state<Choice | null>(null);
  let shownKey = $state("");
  // A seat that is still starting has printed nothing, so the pane says so instead of sitting empty.
  let blank = $state(true);
  // Hidden once answered, until the screen shows a different menu.
  let answered = "";
  // A redraw arrives in pieces, so one frame without the menu is not it closing.
  let hideTimer: ReturnType<typeof setTimeout> | undefined;
  const HIDE_AFTER_MS = 400;

  function keyOf(found: Choice): string {
    return JSON.stringify([found.header, found.question, found.options]);
  }

  function readScreen(rows: string[]): void {
    blank = isBlank(rows);
    const found = detectChoice(rows);
    if (!found) {
      hideTimer ??= setTimeout(() => {
        choice = null;
        shownKey = "";
        answered = "";
        hideTimer = undefined;
      }, HIDE_AFTER_MS);
      return;
    }
    clearTimeout(hideTimer);
    hideTimer = undefined;
    const key = keyOf(found);
    if (key === answered) return;
    if (key !== shownKey) {
      shownKey = key;
      choice = found;
    } else if (choice && choice.cursor !== found.cursor) {
      choice.cursor = found.cursor;
    }
  }

  const KEY_GAP_MS = 150;

  function answer(chunks: string[]): void {
    if (!choice) return;
    answered = shownKey;
    chunks.forEach((chunk, index) => setTimeout(() => app.connection?.input(session.id, chunk), index * KEY_GAP_MS));
    choice = null;
    shownKey = "";
  }
</script>

<section class="session" style:--accent={role.color}>
  <header>
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
    {#if session.degraded.length}
      <p class="degraded">Started without {session.degraded.join(", ")}. agent-compose skipped these steps at launch, so this seat may be missing what they set up.</p>
    {/if}
  </header>
  <div class="body" class:wide data-composing={app.composing} style:--side={sideWidth ? `${sideWidth}px` : undefined}>
    <div class="work">
      {#if app.connection}
        {#key session.id}
          <!-- The card overlays the terminal so it never resizes it, which would make the harness redraw its menu. -->
          <div class="screen" data-link={app.link.state}>
            <Terminal connection={app.connection} sessionId={session.id} label={session.identity} accent={role.color} {messages} {colorOf} onscreen={readScreen} cropHarness locked={!app.typing.allowed || reconnecting()} />
            {#if session.starting && blank}
              <p class="starting" role="status">Starting {session.identity} on {session.seat}. Its first screen shows here as soon as {session.seat} has opened.</p>
            {/if}
            {#if ask}
              {#key ask.id}
                <div class="overlay">
                  <ChoiceCard
                    choice={choiceFromAsk(ask)}
                    identity={session.identity}
                    focusToken={app.focusAsk === session.id ? app.focusCard : 0}
                    onfocused={() => (app.focusAsk = null)}
                    locked={!app.typing.allowed || reconnecting()}
                    lockedLabel={reconnecting() ? "Reconnecting" : "Read only"}
                    cancelLocked={needsPasskey(app.typing) || reconnecting()}
                    onanswer={(picks, text) => {
                      const reply = askAnswer(ask, picks, text);
                      app.connection?.answer(ask.id, reply.picks, reply.text);
                    }}
                    oncancel={() => {
                      app.connection?.cancelAsk(ask.id);
                    }}
                  />
                </div>
              {/key}
            {:else if choice}
              {@const current = choice}
              {#key shownKey}
                <div class="overlay">
                  <ChoiceCard choice={current} identity={session.identity} locked={!app.typing.allowed || reconnecting()} lockedLabel={reconnecting() ? "Reconnecting" : "Read only"} onanswer={(picks, text) => answer(keysFor(current, picks[0] ?? 0, text))} oncancel={() => answer(CANCEL)} />
                </div>
              {/key}
            {/if}
          </div>
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
    <SplitHandle onchange={resize} onreset={() => resize(null)} />
    <SidePanel {session} {messages} selfRole={role.slug} {colorOf} accent={role.color} bind:tab={side} bind:open={raised} onpick={(tab) => storeOverride(role.slug, tab)} />
  </div>
</section>

<style>
  .session { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
  header { flex: none; min-height: 64px; padding: 8px 24px; display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; border-bottom: 3px solid var(--accent); background: color-mix(in srgb, var(--accent) 6%, var(--ground)); }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 22px; }
  .role { font-size: 14px; color: color-mix(in srgb, var(--accent) 55%, white); }
  .degraded { flex-basis: 100%; margin: 0; padding: 6px 10px; border-radius: 6px; border: 1px solid var(--warn); background: var(--warn-fill); color: var(--warn-text); font-size: 13px; }
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
  .body :global(.panel) { border-left: none; }
  /* A phone: the seat's name and state on one row, its role and context on the next, so the header does not cost the terminal a third of the screen. */
  @media (max-width: 720px) {
    /* The header gives its two rows back. A degraded start still says so. */
    header { min-height: 0; padding: 0; border-bottom-width: 2px; }
    .degraded { margin: 4px 8px; font-size: 12px; }
    .overlay { max-height: 100%; padding-bottom: 6px; }
    /* The tab strip gives way to the keyboard, so typing has the whole screen but the seat's text. */
    .body[data-composing="true"] :global(.panel) { display: none; }
    .body[data-composing="true"] { --strip: 0px; }
  }
  @media (max-width: 1000px) {
    /* The sheet floats over this row, so the strip's height stays clear of the composer and the terminal never resizes as it rises. */
    .body, .body.wide { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(320px, 1fr); padding-bottom: var(--strip); min-height: calc(320px + var(--strip)); }
  }
</style>
