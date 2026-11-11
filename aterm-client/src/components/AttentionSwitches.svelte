<script lang="ts">
  import { app, setAlert } from "../lib/app.svelte";
  import { push, pushBlocked, setPush } from "../lib/push.svelte";

  // The browser, not the daemon, decides this: no worker, no push manager, an insecure page.
  const blocked = pushBlocked();

  async function flip(event: Event & { currentTarget: HTMLInputElement }): Promise<void> {
    const box = event.currentTarget;
    await setPush(box.checked);
    // A refusal leaves the switch where it was, and the box has already moved.
    box.checked = push.on;
  }
</script>

<section class="alerts" aria-labelledby="alerts-label">
  <h2 id="alerts-label">Alerts</h2>
  <div class="switches">
    <label class="switch">
      <input type="checkbox" role="switch" checked={app.alerts.sound} aria-describedby="alerts-sound-hint" onchange={(event) => setAlert("sound", event.currentTarget.checked)} />
      <span class="track" aria-hidden="true"></span>
      <span class="name">Sound</span>
    </label>
    <label class="switch">
      <input type="checkbox" role="switch" checked={app.alerts.visual} aria-describedby="alerts-visual-hint" onchange={(event) => setAlert("visual", event.currentTarget.checked)} />
      <span class="track" aria-hidden="true"></span>
      <span class="name">Highlight</span>
    </label>
    {#if app.webPush}
      <label class="switch">
        <input type="checkbox" role="switch" checked={push.on} disabled={blocked !== null} aria-busy={push.busy} aria-describedby="alerts-push-hint alerts-push-note" onchange={flip} />
        <span class="track" aria-hidden="true"></span>
        <span class="name">Notify when closed</span>
      </label>
    {/if}
  </div>
  <p id="alerts-sound-hint" class="hint">One short tone when the coordinator seat pings you. A seat that is waiting on you stays silent.</p>
  <p id="alerts-visual-hint" class="hint">A soft tint on the seat's tab, until you open it.</p>
  {#if app.webPush}
    <p id="alerts-push-hint" class="hint">A notification on this device when a seat starts waiting on you, even with the app closed.</p>
    <p id="alerts-push-note" class={push.failed || blocked ? "blocked" : "hint"} role={push.failed ? "alert" : "status"}>
      {#if push.busy}{push.on ? "Turning off" : "Turning on"}...{:else if push.message}{push.message}{:else if blocked}{blocked}{/if}
    </p>
  {/if}
  {#if app.alerts.soundBlocked}
    <p class="blocked" role="status">Sound is on, but the browser is holding audio back. Click anywhere on the page, or switch Sound off and on again.</p>
  {/if}
</section>

<style>
  .alerts { margin-top: auto; padding-top: 12px; border-top: 1px solid var(--line); }
  h2 { margin: 6px 10px 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  .switches { display: flex; flex-direction: column; }
  .switch { position: relative; min-height: 44px; display: flex; align-items: center; gap: 12px; padding: 0 10px; border-radius: 8px; cursor: pointer; }
  .switch:hover { background: var(--hover); }
  input { position: absolute; opacity: 0; inset: 0; width: 100%; height: 100%; margin: 0; cursor: pointer; }
  .track { position: relative; flex: none; width: 36px; height: 20px; border-radius: 10px; border: 2px solid var(--control-line); background: transparent; }
  .track::after { content: ""; position: absolute; top: 2px; left: 2px; width: 12px; height: 12px; border-radius: 6px; background: var(--muted); transition: transform 120ms; }
  input:checked + .track { background: var(--brand); border-color: var(--brand); }
  input:checked + .track::after { transform: translateX(16px); background: var(--brand-ink); }
  input:focus-visible + .track { outline: 2px solid var(--brand); outline-offset: 3px; }
  .name { font-size: 14px; color: var(--text-soft); }
  .hint, .blocked { margin: 0 10px 4px; font-size: 12px; color: var(--muted); }
  .blocked { color: var(--warn-text); }
  .blocked::before { content: "! "; font-weight: 700; color: var(--warn); }
  /* A live region is kept in the page so a message is announced when it arrives, and takes no room while empty. */
  #alerts-push-note:empty { margin: 0; }
  #alerts-push-note:empty::before { content: none; }

</style>
