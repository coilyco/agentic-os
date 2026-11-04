// The aterm client test bed: journeys against a mock daemon, desktop and phone.
// Walkthrough: the aterm-client skill's testbed reference.
import { defineConfig, devices } from "@playwright/test";

const landscape = /landscape\.spec\.ts/;
const common = { serviceWorkers: "block" as const, colorScheme: "dark" as const, reducedMotion: "reduce" as const };

export default defineConfig({
  testDir: "testbed/journeys",
  outputDir: "testbed/out/results",
  // Baselines are per platform: a font rasterises differently on macOS and Linux.
  snapshotPathTemplate: "testbed/baselines/{platform}/{projectName}/{testFileName}/{testName}/{arg}{ext}",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: [["list"], ["html", { outputFolder: "testbed/out/report", open: "never" }]],
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.002, animations: "disabled", caret: "hide" } },
  // Locally the first run writes baselines. In CI a missing baseline fails.
  updateSnapshots: process.env.CI ? "none" : "missing",
  use: { ...common, trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [
    { name: "desktop-1440", metadata: { kind: "desktop" }, testIgnore: landscape, use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "desktop-1920", metadata: { kind: "desktop" }, testIgnore: landscape, use: { ...devices["Desktop Chrome"], viewport: { width: 1920, height: 1080 } } },
    // iPhone-class runs WebKit, which is Safari's engine. Android-class runs Chromium.
    { name: "iphone", metadata: { kind: "phone" }, testIgnore: landscape, use: { ...devices["iPhone 15"] } },
    { name: "android", metadata: { kind: "phone" }, testIgnore: landscape, use: { ...devices["Pixel 7"] } },
    { name: "phone-320", metadata: { kind: "phone" }, testIgnore: landscape, use: { ...devices["Pixel 7"], viewport: { width: 320, height: 640 } } },
    // Landscape runs only landscape.spec.ts. Height is the scarce dimension there.
    { name: "iphone-landscape", metadata: { kind: "landscape" }, testMatch: landscape, use: { ...devices["iPhone 15"], viewport: { width: 844, height: 390 } } },
    { name: "android-landscape", metadata: { kind: "landscape" }, testMatch: landscape, use: { ...devices["Pixel 7"], viewport: { width: 740, height: 360 } } },
  ],
});
