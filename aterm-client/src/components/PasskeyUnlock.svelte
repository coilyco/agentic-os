<script lang="ts">
  import { app, HOSTED } from "../lib/app.svelte";
  import { needsPasskey } from "../lib/typing";

  const channel = $derived(app.connection?.passkey);
  const standing = $derived(app.typing.allowed ? undefined : app.typing.passkey);
  // Only the hosted page can run the ceremony. Sending a code from any other page would spend it for nothing.
  const offered = $derived(HOSTED && needsPasskey(app.typing) && channel !== undefined && standing !== undefined);
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

{#if offered}
  <div class="unlock">
    {#if standing === "enrolled"}
      <button class="button primary" type="button" disabled={busy} onclick={() => run(() => channel!.assert())}>
        {busy ? "Waiting for your device" : "Unlock with passkey"}
      </button>
      <p class="hint">Your device asks you to verify, with a fingerprint, a face, or a PIN.</p>
    {:else}
      <p class="steps"><strong>No passkey is set up for this device yet.</strong></p>
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
    {/if}
    {#if failure}
      <p class="failure" role="alert">{failure}</p>
    {/if}
  </div>
{/if}

<style>
  .unlock { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; margin-top: 12px; }
  .steps { margin: 0; color: var(--text-soft); }
  ol.steps { padding-left: 22px; display: flex; flex-direction: column; gap: 4px; }
  code { font-family: var(--font-mono); font-size: 0.92em; color: var(--text); }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
  .row { display: flex; gap: 10px; flex-wrap: wrap; align-self: stretch; }
  .row input { flex: 1 1 200px; min-width: 0; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font-family: var(--font-mono); }
  .failure { margin: 0; color: var(--danger-text); }
  button:disabled { opacity: 0.6; cursor: progress; }
</style>
