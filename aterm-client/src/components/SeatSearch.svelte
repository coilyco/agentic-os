<script lang="ts">
  import { onMount } from "svelte";
  import Creature from "./Creature.svelte";
  import { app, openByPerson } from "../lib/app.svelte";
  import { jumpTo } from "../lib/search-jump.svelte";
  import { roleFor } from "../lib/sessions";
  import { refusalText, type SeatSearchResult, type TranscriptMatch } from "../lib/transcript";
  import { matchKindText, splitMarks } from "../lib/transcript-search";

  // One search over every live seat on this host, grouped by seat (COI-3797). A tap opens the seat at the match.
  const WAIT_MS = 300;
  let query = $state("");
  let results = $state<SeatSearchResult[]>([]);
  let phase = $state<"idle" | "searching" | "done" | "failed">("idle");
  let asked = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  // The welcome's features arrive after the Overview mounts, and `arrived` is what says so.
  const channel = $derived(app.feedbackHost.arrived ? app.connection?.transcripts : undefined);
  const found = $derived(results.filter((seat) => seat.matches.length > 0));
  const unread = $derived(results.filter((seat) => seat.reason));
  const count = $derived(found.reduce((sum, seat) => sum + seat.total, 0));
  onMount(() => () => clearTimeout(timer));

  function changed(): void {
    clearTimeout(timer);
    asked++;
    if (!query.trim()) {
      results = [];
      phase = "idle";
      return;
    }
    phase = "searching";
    timer = setTimeout(() => void run(), WAIT_MS);
  }

  async function run(): Promise<void> {
    if (!channel?.searchAll) return;
    const mine = ++asked;
    try {
      const reply = await channel.searchAll(query.trim());
      if (mine !== asked) return;
      results = reply;
      phase = "done";
    } catch {
      if (mine === asked) phase = "failed";
    }
  }

  function open(session: string, match: TranscriptMatch): void {
    jumpTo({ session, match, query: query.trim() });
    openByPerson(session);
  }

  const seatOf = (id: string) => app.sessions.find((one) => one.id === id);
  const status = $derived(
    !query.trim() ? "" : phase === "searching" ? "Searching" : phase === "failed" ? "Search failed" : phase === "done" ? (count ? `${count} ${count === 1 ? "match" : "matches"} in ${found.length} ${found.length === 1 ? "seat" : "seats"}` : "No matches") : "",
  );
</script>

{#if channel?.searchAll}
  <section class="seat-search" aria-labelledby="seat-search-title">
    <h2 id="seat-search-title" class="visually-hidden">Search every seat</h2>
    <div class="bar" role="search">
      <input bind:value={query} type="search" aria-label="Search every seat's transcript" placeholder="Search every seat" autocomplete="off" spellcheck="false" enterkeyhint="search" oninput={changed} onkeydown={(event) => event.key === "Enter" && (clearTimeout(timer), void run())} />
      <span class="count" role="status" aria-live="polite">{status}</span>
    </div>
    {#each found as seat (seat.session)}
      {@const session = seatOf(seat.session)}
      {@const role = session ? roleFor(session, app.roles) : undefined}
      <section class="seat" data-seat={seat.session} aria-label={`${session?.identity ?? seat.session}, ${seat.total} ${seat.total === 1 ? "match" : "matches"}`}>
        <h3>
          {#if role}<Creature role={role.slug} color={role.color} size={24} />{/if}
          <span class="name">{session?.identity ?? seat.session}</span>
          {#if role}<span class="role">{role.displayName}</span>{/if}
          <span class="n">{seat.total}</span>
        </h3>
        <ul>
          {#each [...seat.matches].reverse() as match (match.id)}
            <li>
              <button type="button" class="hit" onclick={() => open(seat.session, match)}>
                <span class="kind">{matchKindText(match.kind, match.field)}</span>
                <span class="snippet">{#each splitMarks(match.snippet, query) as part, at (at)}{#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}{/each}</span>
              </button>
            </li>
          {/each}
        </ul>
        {#if seat.total > seat.matches.length}<p class="more">The newest {seat.matches.length} of {seat.total} show here. Open the seat to step through all of them.</p>{/if}
      </section>
    {/each}
    {#if phase === "done" && unread.length}
      <p class="more">Not searched: {unread.map((seat) => `${seatOf(seat.session)?.identity ?? seat.session} (${refusalText(seat.reason)})`).join(" ")}</p>
    {/if}
  </section>
{/if}

<style>
  .seat-search { display: flex; flex-direction: column; gap: 10px; margin: 0 0 16px; }
  .bar { display: flex; align-items: center; gap: 8px; }
  input { flex: 1 1 auto; min-width: 0; min-height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font: 15px var(--font-body); }
  input:focus-visible, .hit:focus-visible { outline: 2px solid var(--brand); outline-offset: 1px; }
  .count { flex: none; font-size: 13px; color: var(--muted); white-space: nowrap; }
  .seat { border: 1px solid var(--line); border-radius: 10px; padding: 8px 10px; }
  h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 6px; font: 600 14px var(--font-body); color: var(--text); }
  .role { font-weight: 400; font-size: 12px; color: var(--muted); }
  .n { margin-left: auto; font-size: 12px; color: var(--muted); }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .hit { display: flex; gap: 10px; width: 100%; min-height: 44px; align-items: center; padding: 4px 6px; border: 0; border-radius: 8px; background: none; color: var(--text-soft); text-align: left; font: 14px/1.4 var(--font-body); }
  .hit:hover { background: var(--selected); }
  .kind { flex: none; min-width: 72px; font: 600 11px var(--font-body); letter-spacing: 0.04em; text-transform: uppercase; color: var(--muted); }
  .snippet { min-width: 0; overflow-wrap: anywhere; }
  mark { background: color-mix(in srgb, var(--treehouse) 30%, transparent); color: inherit; border-radius: 2px; }
  .more { margin: 4px 0 0; font-size: 12px; color: var(--muted); }
  .visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  @media (max-width: 360px) {
    .hit { flex-direction: column; align-items: flex-start; gap: 2px; }
  }
</style>
