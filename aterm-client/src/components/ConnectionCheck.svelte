<script lang="ts">
  import { onMount, tick } from "svelte";
  import { checkHops, type HopReport, type HopStatus, LIMITS, reportText, TITLES } from "../lib/hops";
  import { openLink } from "../lib/link-notice.svelte";
  import { inApp } from "../lib/open-external";
  import type { Host } from "../lib/protocol";

  let { host, auto = false }: { host: Host; auto?: boolean } = $props();

  let report = $state<HopReport | null>(null);
  let running = $state(false);
  let copy = $state<"" | "done" | "failed">("");
  let fallback = $state<HTMLTextAreaElement | undefined>();
  let latest = 0;

  const WORDS: Record<HopStatus, string> = { pass: "Passed", fail: "Failed", maybe: "Can't tell from here", skipped: "Not tried" };
  const waiting = Object.values(TITLES);
  // While a check is out the last one is stale, so its rows give way to waiting ones.
  const shown = $derived(running ? [] : (report?.hops ?? []));
  const text = $derived(report ? reportText(report, context()) : "");
  const broken = $derived(report ? report.hops.some((hop) => hop.status === "fail" || hop.status === "maybe") : false);
  // Chrome's error page only helps when the request never got an answer. Past that the daemon prints its own reason.
  const unanswered = $derived(report ? report.hops.slice(0, 3).some((hop) => hop.status !== "pass") : false);

  function context() {
    const link = (navigator as Navigator & { connection?: { type?: string; effectiveType?: string } }).connection;
    return { label: host.label, page: location.href, agent: navigator.userAgent, online: navigator.onLine ?? null, link: link?.type ?? link?.effectiveType ?? "" };
  }

  async function check(): Promise<void> {
    const mine = ++latest;
    running = true;
    copy = "";
    const found = await checkHops(host.address);
    // A newer check was asked for while this one was out.
    if (mine !== latest) return;
    report = found;
    running = false;
  }

  async function send(): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      copy = "done";
    } catch {
      // Only a secure page may write the clipboard, so the report is shown selected for a manual copy.
      copy = "failed";
      await tick();
      fallback?.focus();
      fallback?.select();
    }
  }

  onMount(() => {
    if (auto) void check();
  });
</script>

<section class="check" aria-labelledby="check-title">
  <h2 id="check-title">Connection check</h2>
  <p class="lede">Tests each hop between this page and {host.label}, in order, and says which broke.</p>
  <div class="actions">
    <button class="button" type="button" onclick={check} disabled={running}>{running ? "Checking" : report ? "Check again" : "Check connection"}</button>
  </div>
  <!-- The status is on the page before it has words, since a status inserted with its text is not announced. -->
  <p class="verdict" role="status" class:bad={broken}>{running ? "Checking each hop. This takes up to 8 seconds." : (report?.verdict ?? "")}</p>

  {#if running || report}
    <ol class="hops">
      {#each shown as hop, index (hop.id)}
        <li data-hop={hop.id} data-status={hop.status}>
          <div class="head">
            <span class="num mono" aria-hidden="true">{index + 1}</span>
            <strong class="name">{hop.title}</strong>
            <span class="chip">{WORDS[hop.status]}</span>
            {#if hop.status !== "skipped"}<span class="basis">{hop.basis === "seen" ? "Seen by this page" : hop.status === "maybe" ? "Not visible to a page" : "Inferred, not seen"}</span>{/if}
          </div>
          <p class="observed mono">{hop.observed}</p>
          {#if hop.cause}<p class="cause">{hop.cause}</p>{/if}
        </li>
      {:else}
        {#each waiting as title, index (title)}
          <li data-status="waiting">
            <div class="head">
              <span class="num mono" aria-hidden="true">{index + 1}</span>
              <strong class="name">{title}</strong>
              <span class="chip">Waiting</span>
            </div>
          </li>
        {/each}
      {/each}
    </ol>
  {/if}

  {#if report && !running}
    <div class="actions">
      <button class="button primary" type="button" onclick={send}>Copy report for prod-director</button>
      {#if broken && report.page}
        <a class="button" href={report.page} target="_blank" rel="noopener" onclick={(event) => { if (inApp() && report?.page) { event.preventDefault(); void openLink(report.page); } }}>Open {host.label} in a tab</a>
      {/if}
    </div>
    <p class="note" role="status">{copy === "done" ? "Copied. Paste it to prod-director." : copy === "failed" ? "This page cannot write the clipboard. The report is selected below, so copy it by hand." : ""}</p>
    {#if copy === "failed"}
      <textarea class="fallback mono" readonly rows="10" aria-label="Report for prod-director" value={text} bind:this={fallback}></textarea>
    {/if}
    {#if broken && report.page}
      <p class="hint">{unanswered ? "Chrome's own error page, in that tab, names the exact failure, such as ERR_CONNECTION_RESET." : "The daemon prints its own reason on that page."}</p>
    {/if}
    <details>
      <summary>What this page cannot see</summary>
      <ul>
        {#each LIMITS as line (line)}<li>{line}</li>{/each}
      </ul>
    </details>
  {/if}
</section>

<style>
  .check { display: flex; flex-direction: column; gap: 12px; padding-top: 8px; border-top: 1px solid var(--line); }
  h2 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: 22px; }
  .lede, .hint, .note { margin: 0; color: var(--muted); }
  .lede { font-size: 15px; }
  .hint, .note { font-size: 13px; }
  .note { min-height: 1.4em; }
  .actions { display: flex; gap: 10px; flex-wrap: wrap; }
  .button:disabled { opacity: 0.5; cursor: default; }
  .verdict { margin: 0; min-height: 1.5em; font-size: 17px; font-weight: 600; }
  .verdict.bad { color: var(--danger-text); }
  .hops { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  li[data-hop], li[data-status="waiting"] { padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); overflow-wrap: anywhere; }
  li[data-status="fail"] { border-color: var(--danger); background: var(--danger-fill); }
  li[data-status="maybe"] { border-color: var(--warn); background: var(--warn-fill); }
  li[data-status="skipped"] { opacity: 0.7; }
  .head { display: flex; align-items: baseline; gap: 6px 10px; flex-wrap: wrap; }
  .num { color: var(--muted); }
  .name { flex: 1 1 auto; min-width: 0; }
  .chip { font-size: 13px; font-weight: 600; }
  li[data-status="pass"] .chip { color: var(--ok); }
  li[data-status="fail"] .chip { color: var(--danger-text); }
  li[data-status="maybe"] .chip { color: var(--warn-text); }
  li[data-status="skipped"] .chip, li[data-status="waiting"] .chip { color: var(--muted); }
  .basis { font-size: 12px; color: var(--muted); }
  .observed { margin: 6px 0 0; font-size: 12.5px; color: var(--text-soft); }
  .cause { margin: 6px 0 0; font-size: 15px; }
  details { font-size: 14px; color: var(--muted); }
  summary { cursor: pointer; min-height: 32px; display: flex; align-items: center; }
  details ul { margin: 6px 0 0; padding-left: 20px; display: flex; flex-direction: column; gap: 6px; }
  .fallback { width: 100%; padding: 10px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font-size: 12px; }
</style>
