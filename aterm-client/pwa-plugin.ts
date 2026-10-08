import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import type { Plugin } from "vite";

const WORKER_SOURCE = "pwa/sw.js";
// index.html is reached as "./". The daemon redirects /index.html there, and a cached
// redirect cannot answer a navigation.
const LEFT_OUT = new Set(["index.html", "sw.js"]);
// Every browser that installs an app reads woff2, so woff twins only double the download.
const isFallbackFont = (file: string) => file.endsWith(".woff");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? walk(path) : [path];
  });
}

/** Writes dist/sw.js: the worker, every built file listed, a version from their bytes. */
export function appShell(): Plugin {
  let outDir = "";
  return {
    name: "aterm-app-shell",
    apply: "build",
    configResolved(config) {
      outDir = join(config.root, config.build.outDir);
    },
    closeBundle() {
      const built = walk(outDir)
        .map((path) => relative(outDir, path).split("\\").join("/"))
        .filter((file) => !LEFT_OUT.has(file) && !isFallbackFont(file))
        .sort();
      const hash = createHash("sha256");
      for (const file of ["index.html", ...built]) hash.update(file).update(readFileSync(join(outDir, file)));
      const shell = { version: hash.digest("hex").slice(0, 12), files: ["./", ...built.map((file) => `./${file}`)] };
      const worker = readFileSync(WORKER_SOURCE, "utf8").replace('"__SHELL__"', () => JSON.stringify(shell));
      writeFileSync(join(outDir, "sw.js"), worker);
    },
  };
}
