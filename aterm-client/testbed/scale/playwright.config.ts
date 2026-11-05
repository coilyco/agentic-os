// Not the gate. The measurement of COI-3363, run by hand. Chromium only, one worker.
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "measure.spec.ts",
  workers: 1,
  reporter: "line",
  use: { ...devices["Desktop Chrome"] },
});
