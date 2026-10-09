<script lang="ts">
  import { onMount } from "svelte";
  import { app } from "../lib/app.svelte";
  import { DeviceKeyError, keyOffer, type KeyStatus } from "../lib/device-key";
  import { needsPasskey } from "../lib/typing";

  // `always` is the host page, where enrollment stays reachable when nothing is locked.
  let { always = false }: { always?: boolean } = $props();

  const channel = $derived(app.connection?.deviceKey);
  const hostName = $derived(app.hosts.find((host) => host.id === app.attachedHostId)?.label ?? "the host");
  const locked = $derived(needsPasskey(app.typing));

  let status = $state<KeyStatus | null>(null);
  // The host did not know this phone's key, so a prompt cannot help and the steps open.
  let unknown = $state(false);
  let code = $state("");
  let busy = $state(false);
  let failure = $state("");

  const offer = $derived(locked || always ? keyOffer(status, app.typing.deviceKey ?? null, unknown) : null);

  async function refresh(): Promise<void> {
    try {
      status = (await channel?.status()) ?? null;
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    }
  }
  onMount(() => void refresh());

  async function run(task: () => Promise<void>): Promise<void> {
    busy = true;
    failure = "";
    try {
      await task();
      code = "";
      unknown = false;
    } catch (error) {
      if (error instanceof DeviceKeyError && error.reason === "device_key_unknown") unknown = true;
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
      await refresh();
    }
  }

  function enroll(event: SubmitEvent): void {
    event.preventDefault();
    if (code.trim()) void run(() => channel!.enroll(code.trim()));
  }
</script>

{#if offer}
  <div class="unlock">
    {#if offer.blocked}<p class="steps">{offer.blocked}</p>{/if}
    {#if offer.unlock}
      <button class="button primary" type="button" disabled={busy} onclick={() => run(() => channel!.assert())}>
        {busy ? "Waiting for your fingerprint" : "Unlock with fingerprint"}
      </button>
      <p class="hint">Your phone asks for a fingerprint or its screen lock.</p>
    {/if}
    {#if offer.enroll}
      <details open={offer.enrollOpen}>
        <summary>{offer.unlock ? "No key yet? Set one up with a code" : "Set up a key with a code"}</summary>
        <ol class="steps">
          <li>On {hostName}, open a terminal that no agent session started.</li>
          <li>Run <code>aterm passkey enroll</code>.</li>
          <li>Enter the code it prints.</li>
        </ol>
        <form class="row" onsubmit={enroll}>
          <label for="key-code" class="visually-hidden">Enrollment code</label>
          <input id="key-code" bind:value={code} placeholder="Enrollment code" autocomplete="off" autocapitalize="characters" spellcheck="false" disabled={busy} />
          <button class="button primary" type="submit" disabled={busy || !code.trim()}>{busy ? "Waiting for your phone" : "Set up key"}</button>
        </form>
      </details>
    {/if}
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
