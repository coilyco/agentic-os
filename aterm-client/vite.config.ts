import { execSync } from "node:child_process";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vitest/config";
import { appShell, osChrome } from "./pwa-plugin.ts";

const env = (globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env ?? {};

// The short sha a feedback record names, so a screenshot says which build it shows.
function buildStamp(): string {
  if (env.GITHUB_SHA) return env.GITHUB_SHA.slice(0, 7);
  try {
    return execSync("git rev-parse --short HEAD", { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim() || "dev";
  } catch {
    return "dev";
  }
}

export default defineConfig({
  define: { __ATERM_BUILD__: JSON.stringify(buildStamp()) },
  // coilyco.dev serves the client under /aterm/, the daemon serves it at /.
  base: env.ATERM_CLIENT_BASE ?? "/",
  plugins: [svelte(), osChrome(), appShell()],
  server: { port: 5173, strictPort: true },
  test: { include: ["src/**/*.test.ts"] },
});
