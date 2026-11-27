<script lang="ts">
  // The host's kind (COI-3778): set on the attached host, shown on any other that named one.
  import { app, reconnecting, setMachineKind } from "../lib/app.svelte";
  import { MACHINE_KIND_LABEL, MACHINE_KIND_MEANING, MACHINE_KINDS, parseMachineKind, type MachineKind } from "../lib/machine-kind";
  import type { Host } from "../lib/protocol";

  // `headingId` names a heading that already labels the select, so no second label is drawn.
  let { host, headingId }: { host: Host; headingId?: string } = $props();

  const attached = $derived(host.id === app.attachedHostId);
  // Attached, the connection decides: no `host-kind`, no picker, whatever the row once said.
  const kind = $derived(attached ? app.machineKind : (host.machineKind ?? null));
  const settable = $derived(attached && app.machineKind !== null && app.connection?.machineKind !== undefined);
  const locked = $derived(!app.typing.allowed);
  // A set sent while the host is silent would never be answered.
  const silent = $derived(attached && reconnecting());

  let pending = $state<MachineKind | null>(null);
  let failure = $state("");

  async function choose(event: Event & { currentTarget: HTMLSelectElement }): Promise<void> {
    const chosen = parseMachineKind(event.currentTarget.value);
    if (!chosen || chosen === app.machineKind) return;
    pending = chosen;
    failure = "";
    const refused = await setMachineKind(chosen);
    pending = null;
    if (refused) {
      failure = refused;
      // The select shows the choice until told otherwise, so put back what the host holds.
      if (app.machineKind) event.currentTarget.value = app.machineKind;
    }
  }

  const hint = $derived(
    failure
      ? `Not changed. ${failure}`
      : pending
        ? `Setting ${MACHINE_KIND_LABEL[pending]}.`
        : silent
          ? "The host is not answering, so the kind cannot be changed until it is back."
          : locked
          ? `${kind ? MACHINE_KIND_MEANING[kind] : ""} This device is read only, so the kind can only be changed once it is unlocked.`.trim()
          : kind
            ? MACHINE_KIND_MEANING[kind]
            : "",
  );
</script>

{#if settable && kind}
  <div class="kind">
    {#if !headingId}<label for="machine-kind">Host kind</label>{/if}
    <select id="machine-kind" aria-labelledby={headingId} value={pending ?? kind} disabled={locked || silent || pending !== null} aria-describedby="machine-kind-hint" onchange={choose}>
      {#each MACHINE_KINDS as each (each)}
        <option value={each}>{MACHINE_KIND_LABEL[each]}</option>
      {/each}
    </select>
    <p id="machine-kind-hint" class="hint" class:error={failure} role={failure ? "alert" : undefined}>{hint}</p>
  </div>
{:else if kind}
  <p class="lede">{MACHINE_KIND_LABEL[kind]}. {MACHINE_KIND_MEANING[kind]}</p>
{/if}

<style>
  .kind { display: flex; flex-direction: column; gap: 8px; }
  label { font-weight: 600; }
  select { align-self: flex-start; min-width: min(220px, 100%); max-width: 100%; min-height: 44px; padding: 0 14px; border-radius: 8px; border: 1px solid var(--control-line); background: var(--terminal); color: var(--text); font: inherit; }
  select:disabled { opacity: 0.7; cursor: not-allowed; }
  .hint { margin: 0; font-size: 13px; color: var(--muted); }
  .hint.error { color: var(--danger-text); }
  .lede { margin: 0; color: var(--muted); font-size: 17px; }
</style>
