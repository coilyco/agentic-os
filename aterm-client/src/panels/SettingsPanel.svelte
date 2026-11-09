<script lang="ts">
  import AttentionSwitches from "../components/AttentionSwitches.svelte";
  import TapTraceSwitch from "../components/TapTraceSwitch.svelte";
  import { app, closeSettings, removeHost } from "../lib/app.svelte";
  import { DEFAULT_TEXT_SIZE, loadTextSize, stepTextSize, storeTextSize, TEXT_SIZES } from "../lib/text-size";

  let size = $state(loadTextSize());
  // Hosts typed or saved here. The demo host and ones a daemon lists are not stored.
  const saved = $derived(app.hosts.filter((host) => host.kind === "daemon" && !host.found));

  function setSize(next: number): void {
    size = next;
    storeTextSize(next);
  }
</script>

<section class="settings" aria-labelledby="settings-title">
  <header>
    <button type="button" class="back" onclick={closeSettings}>Back</button>
    <h1 id="settings-title">Settings</h1>
  </header>
  <p class="lede">These apply to this device only. Each takes effect as soon as you change it.</p>

  <AttentionSwitches />

  <section class="group" aria-labelledby="text-label">
    <h2 id="text-label">Terminal text</h2>
    <div class="row">
      <button type="button" aria-label="Smaller text" disabled={size === TEXT_SIZES[0]} onclick={() => setSize(stepTextSize(size, -1))}>A−</button>
      <output aria-live="polite">{size}px</output>
      <button type="button" aria-label="Larger text" disabled={size === TEXT_SIZES[TEXT_SIZES.length - 1]} onclick={() => setSize(stepTextSize(size, 1))}>A+</button>
      <button type="button" disabled={size === DEFAULT_TEXT_SIZE} onclick={() => setSize(DEFAULT_TEXT_SIZE)}>Reset to {DEFAULT_TEXT_SIZE}px</button>
    </div>
    <p class="hint">The size of the text in every seat's terminal. A smaller size fits more characters on a narrow screen.</p>
  </section>

  <section class="group" aria-labelledby="hosts-label">
    <h2 id="hosts-label">Hosts on this device</h2>
    {#if saved.length === 0}
      <p class="hint">No host is saved on this device.</p>
    {:else}
      <ul>
        {#each saved as host (host.id)}
          <li>
            <span class="who"><strong>{host.label}</strong><span class="address">{host.address}</span></span>
            <button type="button" aria-label={`Remove ${host.label} from this device`} onclick={() => removeHost(host.id)}>Remove</button>
          </li>
        {/each}
      </ul>
      <p class="hint">Removing a host forgets it here. It does not stop anything running on that host.</p>
    {/if}
  </section>

  <TapTraceSwitch />
</section>

<style>
  .settings { max-width: 640px; padding: 24px clamp(16px, 4vw, 48px) 40px; display: flex; flex-direction: column; gap: 22px; }
  header { display: flex; align-items: center; gap: 14px; }
  h1 { margin: 0; font-family: var(--font-display); font-weight: 600; font-size: clamp(26px, 6vw, 36px); line-height: 1.1; }
  h2 { margin: 0 0 6px; font: 600 12px var(--font-body); letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); }
  .lede { margin: 0; color: var(--muted); font-size: 15px; }
  .back, .row button, li button { min-height: 44px; min-width: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: transparent; color: var(--text); font: 600 14px var(--font-body); }
  .back:focus-visible, .row button:focus-visible, li button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  button:disabled { opacity: 0.5; cursor: default; }
  .group { display: flex; flex-direction: column; gap: 8px; }
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
  output { min-width: 56px; text-align: center; font: 600 15px var(--font-mono); }
  .hint { margin: 0; color: var(--muted); font-size: 13px; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .who { display: flex; flex-direction: column; min-width: 0; }
  .address { color: var(--muted); font: 12px var(--font-mono); overflow-wrap: anywhere; }
</style>
