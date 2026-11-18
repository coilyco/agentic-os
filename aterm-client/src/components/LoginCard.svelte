<script lang="ts">
  import type { Login } from "../lib/login";

  let { login, identity, folded = false, onfold, onsignin, onopen, onsubmit, onretry, oncancel, note = "", locked = false, lockedLabel = "Read only" }: {
    login: Login;
    identity: string;
    /** Folded to one line, so the terminal's own text can be read. */
    folded?: boolean;
    onfold: () => void;
    /** The method menu's button. The pane presses the keys and opens the link once it shows. */
    onsignin: () => void;
    /** Opens the seat's authorize link in the device's own browser. */
    onopen: (url: string) => void;
    /** The pasted code. The pane types it and Enter. */
    onsubmit: (code: string) => void;
    onretry: () => void;
    oncancel: () => void;
    /** Why the browser did not open, in words. */
    note?: string;
    locked?: boolean;
    lockedLabel?: string;
  } = $props();

  let code = $state("");
  const heading = $derived(login.kind === "error" ? "That code did not work" : login.kind === "code" ? "Paste the code from Claude" : "Sign in to Claude");
</script>

<section class="card" class:folded aria-labelledby="login-heading">
  <div class="body">
    <p class="asker">
      <span>{identity}: not signed in{#if locked}<span class="chip">{lockedLabel}</span>{/if}</span>
      <!-- The visible word is short so the heading keeps one line on a phone. The name says what it does. -->
      <button type="button" class="fold" aria-expanded={!folded} aria-label={folded ? "Show sign-in" : "Show terminal"} onclick={onfold}>{folded ? "Sign-in" : "Terminal"}</button>
    </p>
    <h2 id="login-heading" class:peek={folded}>{heading}</h2>
    {#if !folded}
      {#if login.kind === "error"}
        <p class="said" role="alert">{login.message}</p>
      {:else if login.kind === "code"}
        <p class="say">Open the sign-in page, approve there, then copy the code it shows and paste it below.</p>
        {#if login.url !== null}
          {@const url = login.url}
          <button class="button open" type="button" disabled={locked} onclick={() => onopen(url)}>Open sign-in page</button>
        {:else}
          <p class="said" role="status">The sign-in link on the terminal is not one this card opens. Use the terminal's own link.</p>
        {/if}
        {#if note}<p class="said" role="alert">{note}</p>{/if}
        <form
          onsubmit={(event) => {
            event.preventDefault();
            if (code.trim()) onsubmit(code);
          }}
        >
          <label for="login-code">Code from Claude</label>
          <input id="login-code" bind:value={code} placeholder="Paste the code" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" enterkeyhint="go" disabled={locked} />
          <button class="button primary" type="submit" disabled={locked || !code.trim()}>Sign in</button>
        </form>
      {:else}
        <p class="say">The sign-in page opens in this device's own browser. Console and cloud logins stay on the terminal.</p>
      {/if}
    {/if}
  </div>
  {#if !folded && login.kind !== "code"}
    <div class="actions">
      {#if login.kind === "method"}
        <button class="button primary" disabled={locked} onclick={onsignin}>Sign in with Claude</button>
      {:else}
        <button class="button primary" disabled={locked} onclick={onretry}>Try again</button>
        <button class="button" disabled={locked} onclick={oncancel}>Continue without signing in</button>
      {/if}
    </div>
  {/if}
</section>

<style>
  .card { margin: 0 12px; border-radius: 12px; border: 1px solid var(--brand); background: var(--raised); display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
  .body { flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 14px 16px; display: flex; flex-direction: column; gap: 10px; }
  .asker { margin: 0; display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--brand); }
  .fold { flex: none; min-height: 44px; padding: 0 12px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 13px var(--font-body); letter-spacing: 0; text-transform: none; }
  .fold:focus-visible, .button:focus-visible, input:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  .chip { margin-left: 10px; padding: 2px 8px; border-radius: 999px; border: 1px solid var(--chip-line); color: var(--text-soft); text-transform: none; letter-spacing: 0; }
  h2 { margin: 0; font-size: 15px; font-weight: 500; line-height: 1.45; overflow-wrap: anywhere; }
  .peek { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 14px; }
  .say { margin: 0; font-size: 13px; color: var(--muted); }
  /* A bar, not a box: the seat's own words are the one thing here that must not be clipped. */
  .said { margin: 0; padding: 2px 0 2px 10px; border-left: 3px solid var(--warn); color: var(--warn-text); font-size: 13px; overflow-wrap: anywhere; }
  .open { align-self: flex-start; }
  form { display: flex; flex-wrap: wrap; gap: 6px 8px; align-items: center; }
  label { flex: 1 0 100%; font-size: 12px; color: var(--text-soft); }
  input { flex: 1 1 140px; min-width: 0; min-height: 44px; padding: 0 12px; box-sizing: border-box; border-radius: 8px; border: 1px solid var(--brand); background: var(--terminal); color: var(--text); font: 15px var(--font-mono); }
  .actions { flex: none; display: flex; flex-wrap: wrap; gap: 8px; padding: 0 16px 14px; }
  .body:has(+ .actions) { padding-bottom: 10px; }
  .card.folded .body { padding: 6px 12px 10px; gap: 2px; overflow: hidden; }
  button:disabled { opacity: 0.5; cursor: default; }
  @media (max-width: 480px) {
    .card { margin: 0 8px; }
    .body { padding: 10px 12px; gap: 8px; }
    .actions { padding: 0 12px 10px; }
  }
</style>
