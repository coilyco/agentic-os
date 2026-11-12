<script lang="ts">
  import { app, cancelSignIn, refreshMachines, signIn, signInThroughBrowser, signOut, signUp } from "../lib/app.svelte";
  import { passkeySupport } from "../lib/passkey";

  // The account this window is signed in to. Machines registered to it are listed with the other hosts, so this panel holds the sign-in, the empty state and what went wrong.
  // A hop signs in on accounts' own page in the browser, so this page needs no passkey support of its own.
  const hop = $derived(app.accounts.hop);
  const unsupported = $derived(hop ? null : passkeySupport());
  const busy = $derived(app.accounts.status === "signing-in");
  const machines = $derived(app.hosts.filter((host) => host.brokered));

  let name = $state("");

  function create(event: SubmitEvent): void {
    event.preventDefault();
    if (name.trim()) void signUp(name.trim());
  }
</script>

{#if app.accounts.configured}
  <section class="account" aria-labelledby="account-title">
    {#if app.accounts.status === "signed-in"}
      <h2 id="account-title">Your machines</h2>
      <p class="who" role="status">Signed in as {app.accounts.name}.</p>
      {#if app.accounts.listError}
        <div class="alert" role="alert">
          <p>{app.accounts.listError}{app.accounts.listed ? " The list below is the last one this window got." : ""}</p>
          <button class="button" type="button" onclick={() => refreshMachines()}>Check again</button>
        </div>
      {/if}
      {#if app.accounts.listed && machines.length === 0 && !app.accounts.listError}
        <p>No machine is registered to this account yet. One shows up in the host list once it is registered and connected to the broker.</p>
      {:else if machines.length > 0}
        <p>{machines.length === 1 ? "One machine is" : `${machines.length} machines are`} registered to this account. Pick one from the host list. An offline machine is marked offline and opens once it connects.</p>
      {/if}
      <button class="button" type="button" onclick={() => signOut()}>Sign out</button>
    {:else}
      <h2 id="account-title">Sign in to see your machines</h2>
      {#if hop}
        <p>Machines register to your account, so signing in lists them here. This opens the accounts page in your browser, where you verify with a fingerprint, a face, or a PIN. Come back here when it finishes.</p>
      {:else}
        <p>Machines register to your account, so signing in lists them here. This device asks you to verify with a fingerprint, a face, or a PIN.</p>
      {/if}
      {#if unsupported}
        <p class="alert" role="status">{unsupported}</p>
      {/if}
      <button class="button primary" type="button" disabled={busy || unsupported !== null} onclick={() => (hop ? signInThroughBrowser() : signIn())}>
        {busy ? (hop ? "Waiting for your browser" : "Waiting for your device") : "Sign in with a passkey"}
      </button>
      {#if busy && hop}
        <p role="status">Finish signing in in your browser, then return here.</p>
        <button class="button" type="button" onclick={() => cancelSignIn()}>Cancel</button>
      {/if}
      {#if app.accounts.error}
        <p class="failure" role="alert">{app.accounts.error}</p>
      {/if}
      {#if !hop}
      <details>
        <summary>No account yet? Create one</summary>
        <form class="row" onsubmit={create}>
          <label for="account-name" class="visually-hidden">Account name</label>
          <input id="account-name" bind:value={name} placeholder="Account name" autocomplete="username webauthn" spellcheck="false" disabled={busy || unsupported !== null} />
          <button class="button" type="submit" disabled={busy || unsupported !== null || !name.trim()}>Create account with a passkey</button>
        </form>
      </details>
      {/if}
    {/if}
  </section>
{/if}

<style>
  .account { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; padding: 16px; border: 1px solid var(--control-line); border-radius: 10px; }
  h2 { margin: 0; font-family: var(--font-display); font-size: 18px; font-weight: 600; }
  p { margin: 0; color: var(--text-soft); font-size: 15px; }
  .who { color: var(--text); }
  .alert { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; align-self: stretch; padding: 12px 16px; border: 1px solid var(--danger); border-radius: 10px; background: var(--danger-fill); }
  .alert p { flex: 1 1 220px; }
  .failure { color: var(--danger-text); }
  details { align-self: stretch; }
  summary { min-height: 44px; display: flex; align-items: center; cursor: pointer; color: var(--text-soft); }
  .row { display: flex; gap: 10px; flex-wrap: wrap; }
  .row input { flex: 1 1 200px; min-width: 0; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); }
  button:disabled { opacity: 0.6; cursor: progress; }
</style>
