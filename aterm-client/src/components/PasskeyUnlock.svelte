<script lang="ts">
  import { app, HOSTED } from "../lib/app.svelte";
  import { passkeyOffer } from "../lib/typing";

  // `always` is the host page, where enrollment stays reachable when nothing is locked.
  let { always = false }: { always?: boolean } = $props();

  // The flag is reactive and the connection's feature list is not, so this reads the flag first and the channel only once it is set.
  const channel = $derived(app.passkeyAvailable ? app.connection?.passkey : undefined);
  const standing = $derived(app.typing.passkey);
  // Only the hosted page can run the ceremony. Sending a code from any other page would spend it for nothing.
  const offer = $derived(passkeyOffer(app.typing, { hosted: HOSTED, channel: channel !== undefined, always }));
  const hostName = $derived(app.hosts.find((host) => host.id === app.attachedHostId)?.label ?? "the host");

  let code = $state("");
  let busy = $state(false);
  let failure = $state("");

  async function run(task: () => Promise<void>): Promise<void> {
    busy = true;
    failure = "";
    try {
      await task();
      code = "";
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  function enroll(event: SubmitEvent): void {
    event.preventDefault();
    if (code.trim()) void run(() => channel!.enroll(code.trim()));
  }
</script>

{#snippet steps()}
  <ol class="steps">
    <li>On {hostName}, open a terminal that no agent session started.</li>
    <li>Run <code>aterm passkey enroll</code>.</li>
    <li>Enter the code it prints.</li>
  </ol>
  <form class="row" onsubmit={enroll}>
    <label for="enroll-code" class="visually-hidden">Enrollment code</label>
    <input id="enroll-code" bind:value={code} placeholder="Enrollment code" autocomplete="off" autocapitalize="characters" spellcheck="false" disabled={busy} />
    <button class="button primary" type="submit" disabled={busy || !code.trim()}>{busy ? "Waiting for your device" : "Set up passkey"}</button>
  </form>
{/snippet}

{#if offer}
  <div class="unlock">
    {#if offer.unlock}
      <button class="button primary" type="button" disabled={busy} onclick={() => run(() => channel!.assert())}>
        {busy ? "Waiting for your device" : "Unlock with passkey"}
      </button>
      <p class="hint">Your device asks you to verify, with a fingerprint, a face, or a PIN.</p>
    {/if}
    {#if standing === "unenrolled"}
      <p class="steps"><strong>No passkey is set up for this device yet.</strong></p>
    {/if}
    <details open={offer.enrollOpen}>
      <summary>{standing === "unenrolled" ? "Set up a passkey with a code" : offer.unlock ? "No passkey yet? Set one up with a code" : "Set up a passkey with a code"}</summary>
      {@render steps()}
    </details>
    {#if failure}
      <p class="failure" role="alert">{failure}</p>
    {/if}
  </div>
{/if}

<style>
  .unlock { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; margin-top: 12px; }
  .steps { margin: 0; color: var(--text-soft); }
  ol.steps { padding-left: 22px; display: flex; flex-direction: column; gap: 4px; margin: 8px 0; }
  code { font-family: var(--font-mono); font-size: 0.92em; color: var(--text); }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
  details { align-self: stretch; }
  summary { min-height: 44px; display: flex; align-items: center; gap: 8px; cursor: pointer; color: var(--text-soft); }
  summary::before { content: "+"; width: 1em; font-family: var(--font-mono); color: var(--brand); }
  details[open] > summary::before { content: "\2212"; }
  .row { display: flex; gap: 10px; flex-wrap: wrap; align-self: stretch; }
  .row input { flex: 1 1 200px; min-width: 0; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font-family: var(--font-mono); }
  .failure { margin: 0; color: var(--danger-text); }
  button:disabled { opacity: 0.6; cursor: progress; }
</style>
