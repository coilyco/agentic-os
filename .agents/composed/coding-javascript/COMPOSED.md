---
name: coding-javascript
description: JavaScript and TypeScript umbrella skill. Default to TypeScript over plain JavaScript. Node 20+. ESM modules. Frameworks live as subskills (e.g. coding-javascript-react).
seed:
  kind: language
  language: javascript
  extensions: [".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx"]
---

# coding-javascript

Umbrella for anything in the JS/TS world. Subskills (frameworks, runtimes, build tools) sit under this when they accumulate enough form to deserve their own surface.

## Defaults

- **Language**: TypeScript over plain JavaScript. If a project is plain JS, work in JS but flag the migration option once.
- **Runtime**: Node 20+ unless target pins lower. Bun and Deno on a case-by-case basis.
- **Modules**: ESM. CJS only when the surrounding code requires it.
- **Package manager**: project's own. If greenfield, `pnpm`.
- **Build**: prefer the project's chosen bundler. For new projects, `vite`.

## Style

- Strict TS config (`strict: true`). Don't reach for `any`, prefer `unknown` and narrow.
- `import type` for type-only imports.
- Async/await over raw promise chains.
- No default exports unless the framework demands them. Named exports only.

## Frameworks (subskills)

- React → `coding-javascript-react`. Read before touching any React code, the persona has strong feelings.

## When this skill is active

Editing or writing JS/TS. Inherit Kai's defaults before falling back to training-data conventions.

## Triggers

javascript, typescript, js, ts, .js, .ts, .mjs, .tsx, node, nodejs, npm, pnpm, yarn, bun, deno, esm, cjs, vite, webpack, esbuild.
