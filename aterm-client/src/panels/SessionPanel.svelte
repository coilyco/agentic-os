<script lang="ts">
  import Creature from "../components/Creature.svelte";
  import ChoiceCard from "../components/ChoiceCard.svelte";
  import Composer from "../components/Composer.svelte";
  import SidePanel, { type SideTab } from "../components/SidePanel.svelte";
  import Terminal from "../components/Terminal.svelte";
  import { app, asksFor, colorOf, jumpToWaiting, messagesFor } from "../lib/app.svelte";
  import { askAnswer, CANCEL, choiceFromAsk, detectChoice, keysFor, type Choice } from "../lib/choices";
  import { contextPercent, contextText } from "../lib/context";
  import type { Session } from "../lib/protocol";
  import type { Role } from "../lib/roster";
  import { sessionCode } from "../lib/sessions";

  let { role, session }: { role: Role; session: Session } = $props();
  const messages = $derived(messagesFor(session));
  // A structured ask beats a menu read off the screen: it is the seat's own words.
  const ask = $derived(asksFor(session.id)[0]);
  let side = $state<SideTab>("messages");
  const page = $derived(app.browsers[session.id]);
  // The browser needs room to be read, so a page takes half the width. Its empty states do not.
  const wide = $derived(side === "browser" && page !== undefined && page.state !== "none");
  let choice = $state<Choice | null>(null);
  let shownKey = $state("");
  // Hidden once answered, until the screen shows a different menu.
  let answered = "";
  // A redraw arrives in pieces, so one frame without the menu is not it closing.
  let hideTimer: ReturnType<typeof setTimeout> | undefined;
  const HIDE_AFTER_MS = 400;

  function keyOf(found: Choice): string {
    return JSON.stringify([found.header, found.question, found.options]);
  }

  function readScreen(rows: string[]): void {
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
    nextIfTriaging();
  }

  // Alt-tabbed in to clear the queue, so an answer moves straight to the next seat.
  function nextIfTriaging(): void {
    if (app.triage) setTimeout(jumpToWaiting, 250);
  }
</script>

<section class="session" style:--accent={role.color}>
  <header>
    <Creature role={role.slug} color={role.color} size={40} />
    <h1>{session.identity}</h1>
    <span class="role">{role.displayName} // {session.seat}{sessionCode(session) ? ` // ${sessionCode(session)}` : ""}</span>
    {#if session.context}
      {@const percent = contextPercent(session.context)}
      <span class="context mono" data-source={session.context.source} title={`context read from ${session.context.source}`}>
        {contextText(session.context)}
        {#if percent !== null}<span class="bar" aria-hidden="true" style:--fill={`${Math.min(percent, 100)}%`}></span>{/if}
      </span>
    {/if}
    <span class="state mono">{session.state}</span>
    {#if session.degraded.length}
      <p class="degraded">Started without {session.degraded.join(", ")}. agent-compose skipped these steps at launch, so this seat may be missing what they set up.</p>
    {/if}
  </header>
  <div class="body" class:wide>
    <div class="work">
      {#if app.connection}
        {#key session.id}
          <!-- The card overlays the terminal so it never resizes it, which would make the harness redraw its menu. -->
          <div class="screen">
            <Terminal connection={app.connection} sessionId={session.id} label={session.identity} accent={role.color} {messages} {colorOf} onscreen={readScreen} />
            {#if ask}
              {#key ask.id}
                <div class="overlay">
                  <ChoiceCard
                    choice={choiceFromAsk(ask)}
                    identity={session.identity}
                    focusToken={app.focusCard}
                    onanswer={(picks, text) => {
                      const reply = askAnswer(ask, picks, text);
                      app.connection?.answer(ask.id, reply.picks, reply.text);
                      nextIfTriaging();
                    }}
                    oncancel={() => {
                      app.connection?.cancelAsk(ask.id);
                      nextIfTriaging();
                    }}
                  />
                </div>
              {/key}
            {:else if choice}
              {@const current = choice}
              {#key shownKey}
                <div class="overlay">
                  <ChoiceCard choice={current} identity={session.identity} onanswer={(picks, text) => answer(keysFor(current, picks[0] ?? 0, text))} oncancel={() => answer(CANCEL)} />
                </div>
              {/key}
            {/if}
          </div>
          <Composer connection={app.connection} {session} sessions={app.sessions} />
        {/key}
      {/if}
    </div>
    <SidePanel {session} {messages} selfRole={role.slug} {colorOf} bind:tab={side} />
  </div>
</section>

<style>
  .session { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
  header { min-height: 64px; padding: 8px 24px; display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; border-bottom: 3px solid var(--accent); background: color-mix(in srgb, var(--accent) 6%, var(--ground)); }
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
  .overlay { position: absolute; left: 0; right: 0; bottom: 0; max-height: 92%; display: flex; flex-direction: column; justify-content: flex-end; padding-bottom: 10px; background: linear-gradient(to top, var(--terminal) 70%, transparent); }
  .body { flex: 1; display: grid; grid-template-columns: minmax(0, 1fr) 360px; min-height: 0; }
  .body.wide { grid-template-columns: minmax(0, 1fr) minmax(360px, 50%); }
  @media (max-width: 1000px) {
    .body, .body.wide { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(320px, 1fr) auto; }
    .body :global(.panel) { border-left: none; border-top: 1px solid var(--line); max-height: 40vh; }
    .body.wide :global(.panel) { max-height: none; }
  }
</style>
