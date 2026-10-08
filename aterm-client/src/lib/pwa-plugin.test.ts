import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { appShell } from "../../pwa-plugin.ts";

type Hook = (...args: unknown[]) => void;

const cleanup: string[] = [];
afterEach(() => {
  for (const dir of cleanup.splice(0)) rmSync(dir, { recursive: true, force: true });
});

// A project root with a built tree at <root>/<built>, as vite leaves it at closeBundle.
function project(built: string): string {
  const root = mkdtempSync(join(tmpdir(), "aterm-app-shell-"));
  cleanup.push(root);
  mkdirSync(join(root, built, "assets"), { recursive: true });
  writeFileSync(join(root, built, "index.html"), "<html></html>");
  writeFileSync(join(root, built, "assets", "app.js"), "export {}");
  return root;
}

function runPlugin(root: string, outDir: string) {
  const plugin = appShell() as unknown as { configResolved: Hook; closeBundle: Hook };
  plugin.configResolved({ root, build: { outDir } });
  plugin.closeBundle();
}

describe("app shell plugin", () => {
  it("writes sw.js into a relative outDir under the project root", () => {
    const root = project("dist");
    runPlugin(root, "dist");
    expect(readFileSync(join(root, "dist", "sw.js"), "utf8")).toContain("./assets/app.js");
  });

  // vite build --outDir <absolute path>: the root must not be joined onto it (COI-2551).
  it("writes sw.js into an absolute outDir outside the project root", () => {
    const outDir = join(project("out"), "out");
    const elsewhere = mkdtempSync(join(tmpdir(), "aterm-app-shell-root-"));
    cleanup.push(elsewhere);
    runPlugin(elsewhere, outDir);
    const worker = readFileSync(join(outDir, "sw.js"), "utf8");
    expect(worker).not.toContain("__SHELL__");
    expect(worker).toContain("./assets/app.js");
  });
});
