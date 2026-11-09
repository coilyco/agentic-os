<script lang="ts">
  import { IN_APP } from "../lib/build";
  import { storeTrace, switchShown, traceStored } from "../lib/tap-trace";

  // The trace starts when the page loads, so the page reloads when this changes.
  let on = $state(traceStored());
  const shown = switchShown(IN_APP, traceStored(), location.search);

  function change(next: boolean): void {
    on = next;
    storeTrace(next);
    location.reload();
  }
</script>

{#if shown}
<section class="trace" aria-labelledby="trace-label">
  <label class="switch">
    <input type="checkbox" role="switch" checked={on} aria-describedby="trace-hint" onchange={(event) => change(event.currentTarget.checked)} />
    <span class="track" aria-hidden="true"></span>
    <span id="trace-label" class="name">Tap trace</span>
  </label>
  <p id="trace-hint" class="hint">Records what each tap does, for a bug report. The page reloads when you switch it.</p>
</section>
{/if}

<style>
  .trace { padding-top: 4px; }
  .switch { position: relative; min-height: 44px; display: flex; align-items: center; gap: 12px; padding: 0 10px; border-radius: 8px; cursor: pointer; }
  .switch:hover { background: #171a21; }
  input { position: absolute; opacity: 0; inset: 0; width: 100%; height: 100%; margin: 0; cursor: pointer; }
  .track { position: relative; flex: none; width: 36px; height: 20px; border-radius: 10px; border: 2px solid var(--control-line); background: transparent; }
  .track::after { content: ""; position: absolute; top: 2px; left: 2px; width: 12px; height: 12px; border-radius: 6px; background: var(--muted); }
  input:checked + .track { background: var(--brand); border-color: var(--brand); }
  input:checked + .track::after { transform: translateX(16px); background: var(--brand-ink); }
  input:focus-visible + .track { outline: 2px solid var(--brand); outline-offset: 3px; }
  .name { font-size: 14px; color: var(--text-soft); }
  .hint { margin: 0 10px 4px; font-size: 12px; color: var(--muted); }
</style>
