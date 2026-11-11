<script lang="ts">
  import AttentionSwitches from "../components/AttentionSwitches.svelte";
  import InstallCard from "../components/InstallCard.svelte";
  import RoleSwitches from "../components/RoleSwitches.svelte";
  import TapTraceSwitch from "../components/TapTraceSwitch.svelte";
  import { app, closeSettings, removeHost } from "../lib/app.svelte";
  import { loadViewDefault, storeViewDefault, type ViewDefault } from "../lib/seat-view";
  import { loadWrapDefault, storeWrapDefault, type WrapDefault } from "../lib/wrap";
  import { DEFAULT_TEXT_SIZE, loadTextSize, stepTextSize, storeTextSize, TEXT_SIZES } from "../lib/text-size";

  const OPENS: { value: ViewDefault; label: string }[] = [
    { value: "auto", label: "Auto" },
    { value: "focus", label: "Focus" },
    { value: "default", label: "Default" },
    { value: "verbose", label: "Verbose" },
  ];
  let opens = $state(loadViewDefault());
  function setOpens(next: ViewDefault): void {
    opens = next;
    storeViewDefault(next);
  }
  const WRAPS: { value: WrapDefault; label: string }[] = [
    { value: "auto", label: "Auto" },
    { value: "on", label: "On" },
    { value: "off", label: "Off" },
  ];
  let wraps = $state(loadWrapDefault());
  function setWraps(next: WrapDefault): void {
    wraps = next;
    storeWrapDefault(next);
  }
  // Arrows move between the stops as a radio group does, and focus follows.
  function arrow<T extends string>(event: KeyboardEvent, list: { value: T }[], current: T, set: (next: T) => void): void {
    const step = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : event.key === "ArrowLeft" || event.key === "ArrowUp" ? -1 : 0;
    if (!step) return;
    event.preventDefault();
    const next = list[(list.findIndex((each) => each.value === current) + step + list.length) % list.length]!;
    set(next.value);
    queueMicrotask(() => (event.currentTarget as HTMLElement).querySelector<HTMLElement>('[aria-checked="true"]')?.focus());
  }

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

  <RoleSwitches />

  <section class="group" aria-labelledby="opens-label">
    <h2 id="opens-label">New seats open in</h2>
    <div class="stops" role="radiogroup" aria-labelledby="opens-label" tabindex="-1" onkeydown={(event) => arrow(event, OPENS, opens, setOpens)}>
      {#each OPENS as each (each.value)}
        <button type="button" role="radio" aria-checked={opens === each.value} tabindex={opens === each.value ? 0 : -1} onclick={() => setOpens(each.value)}>{each.label}</button>
      {/each}
    </div>
    <p class="hint">The view a seat shows the first time you open it here. Auto is Focus on a phone and the raw screen on a desktop. A seat keeps the view you pick for it.</p>
  </section>

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

  <section class="group" aria-labelledby="wrap-label">
    <h2 id="wrap-label">Wrap long lines</h2>
    <div class="stops" role="radiogroup" aria-labelledby="wrap-label" tabindex="-1" onkeydown={(event) => arrow(event, WRAPS, wraps, setWraps)}>
      {#each WRAPS as each (each.value)}
        <button type="button" role="radio" aria-checked={wraps === each.value} tabindex={wraps === each.value ? 0 : -1} onclick={() => setWraps(each.value)}>{each.label}</button>
      {/each}
    </div>
    <p class="hint">When a seat's screen is wider than yours, fold its long lines to fit. Auto does it on a phone or a narrow window and not on a desktop. The Wrap button in a seat's toolbar overrides this for that seat.</p>
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

  <InstallCard />

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
  .stops { display: flex; border: 1px solid var(--control-line); border-radius: 10px; overflow: hidden; }
  .stops button { flex: 1 1 0; min-width: 0; min-height: 44px; padding: 0 8px; border: 0; background: transparent; color: var(--muted); font: 600 14px var(--font-body); }
  .stops button + button { border-left: 1px solid var(--control-line); }
  .stops button[aria-checked="true"] { background: color-mix(in srgb, var(--brand) 22%, var(--surface)); color: var(--text); }
  .stops button:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); }
  .who { display: flex; flex-direction: column; min-width: 0; }
  .address { color: var(--muted); font: 12px var(--font-mono); overflow-wrap: anywhere; }
</style>
