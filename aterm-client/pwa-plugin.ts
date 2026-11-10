import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import type { Plugin } from "vite";

const WORKER_SOURCE = "pwa/sw.js";
const MANIFEST_SOURCE = "pwa/manifest.webmanifest";
const TOKENS = "src/app.css";
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
      // resolve, not join: an absolute --outDir must stay as it is.
      outDir = resolve(config.root, config.build.outDir);
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

/** A #rrggbb token from the token block: the one source the OS chrome reads. */
export function tokenHex(css: string, name: string): string {
  const found = new RegExp(`${name}:\\s*(#[0-9a-fA-F]{6})\\s*;`).exec(css);
  const hex = found?.[1];
  if (!hex) throw new Error(`${TOKENS} has no ${name} as #rrggbb, which the OS chrome reads`);
  return hex;
}

/** Fills the theme-color meta and the manifest from the tokens, before any CSS runs. */
export function osChrome(): Plugin {
  let root = "";
  let base = "/";
  const fill = (text: string) => {
    const css = readFileSync(join(root, TOKENS), "utf8");
    return text.replaceAll("__GROUND__", tokenHex(css, "--ground")).replaceAll("__PLATE__", tokenHex(css, "--plate"));
  };
  const manifest = () => fill(readFileSync(join(root, MANIFEST_SOURCE), "utf8"));
  return {
    name: "aterm-os-chrome",
    configResolved(config) {
      root = config.root;
      base = config.base;
    },
    transformIndexHtml: (html) => fill(html),
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.url?.split("?")[0] !== `${base}manifest.webmanifest`) return next();
        res.setHeader("content-type", "application/manifest+json");
        res.end(manifest());
      });
    },
    generateBundle() {
      this.emitFile({ type: "asset", fileName: "manifest.webmanifest", source: manifest() });
    },
  };
}
