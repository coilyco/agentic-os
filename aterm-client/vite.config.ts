import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vitest/config";
import { appShell } from "./pwa-plugin.ts";

const env = (globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env ?? {};

export default defineConfig({
  // coilyco.dev serves the client under /aterm/, the daemon serves it at /.
  base: env.ATERM_CLIENT_BASE ?? "/",
  plugins: [svelte(), appShell()],
  server: { port: 5173, strictPort: true },
  test: { include: ["src/**/*.test.ts"] },
});
