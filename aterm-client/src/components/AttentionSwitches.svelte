<script lang="ts">
  import { app, setAlert } from "../lib/app.svelte";
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
      <span class="name">Strong visual</span>
    </label>
  </div>
  <p id="alerts-sound-hint" class="hint">One short tone when a seat starts waiting on you.</p>
  <p id="alerts-visual-hint" class="hint">A glow at the page edge and on the seat's tab, until you open it.</p>
  {#if app.alerts.soundBlocked}
    <p class="blocked" role="status">Sound is on, but the browser is holding audio back. Click anywhere on the page, or switch Sound off and on again.</p>
  {/if}
</section>

<style>
  .alerts { margin-top: auto; padding-top: 12px; border-top: 1px solid var(--line); }
  h2 { margin: 6px 10px 4px; font-family: var(--font-display); font-size: 13px; font-weight: 600; letter-spacing: 0.12em; text-transform: uppercase; color: var(--muted); }
  .switches { display: flex; flex-direction: column; }
  .switch { position: relative; min-height: 44px; display: flex; align-items: center; gap: 12px; padding: 0 10px; border-radius: 8px; cursor: pointer; }
  .switch:hover { background: #171a21; }
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

  @media (max-width: 720px) {
    .alerts { margin-top: 0; padding: 0 16px; border-top: none; display: flex; flex-wrap: wrap; align-items: center; column-gap: 8px; }
    h2, .hint { display: none; }
    .switches { flex-direction: row; flex-wrap: wrap; }
    .blocked { flex-basis: 100%; margin: 0 0 4px; }
  }
</style>
