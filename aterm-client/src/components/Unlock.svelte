<script lang="ts">
  import { app } from "../lib/app.svelte";
  import DeviceKeyUnlock from "./DeviceKeyUnlock.svelte";
  import PasskeyUnlock from "./PasskeyUnlock.svelte";

  // The app unlocks with the phone's key, and the hosted page with a passkey. Neither offers the other's.
  let { always = false }: { always?: boolean } = $props();
</script>

{#if app.deviceKeyAvailable}
  <DeviceKeyUnlock {always} />
{:else}
  <PasskeyUnlock {always} />
{/if}
