// How the client behaves as seats grow (COI-3363): the mock daemon with N seats,
// 90 percent idle, and a real Chromium. `measure.spec.ts` runs it.
import type { Browser, CDPSession, Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { startMock, type SessionSpec } from "../mock-daemon.ts";

const ROLES = (JSON.parse(readFileSync(new URL("../../src/lib/fixtures/roster.json", import.meta.url), "utf8")) as { roles: { slug: string }[] }).roles.map((role) => role.slug);
const ANIMALS = ["Imp-Dragonfly", "Beetle-Ox", "Griffin-Goose", "Turtle-Ox", "Frog-Ox", "Wyvern-Goose", "Gnome-Dragonfly", "Panda-Goose"];

/** One seat in ten works. The rest sit at a prompt, as most of Kai's do. */
export function seats(count: number): SessionSpec[] {
  return Array.from({ length: count }, (_, at) => {
    const role = ROLES[at % ROLES.length] as string;
    const animal = ANIMALS[at % ANIMALS.length] as string;
    const busy = at % 10 === 0;
    return {
      name: `${role}-${animal.toLowerCase()}-${at}`,
      role,
      identity: animal,
      seat: "claude",
      ready: true,
      bracketed_paste: true,
      kai_drafting: false,
      pending: 0,
      context: { tokens: 40_000 + at * 700, window: 200_000, source: "claude" },
      last_reply: `Seat ${at} finished its last turn and is waiting.`,
      quiet_seconds: busy ? 0 : 600 + at,
      scene: busy ? "claude-working" : "claude-idle",
    };
  });
}

const INSTRUMENT = `
  window.__m = { long: [], events: [], frames: [] };
  new PerformanceObserver((list) => list.getEntries().forEach((e) => __m.long.push([e.startTime, e.duration]))).observe({ type: "longtask", buffered: true });
  new PerformanceObserver((list) => list.getEntries().forEach((e) => __m.events.push([e.name, e.duration]))).observe({ type: "event", durationThreshold: 16, buffered: true });
  const tick = (t) => { __m.frames.push(t); requestAnimationFrame(tick); };
  requestAnimationFrame(tick);
`;

interface Metrics {
  at: number;
  task: number;
  script: number;
  layout: number;
  style: number;
  heap: number;
  nodes: number;
  listeners: number;
}

async function metrics(cdp: CDPSession): Promise<Metrics> {
  const { metrics: list } = await cdp.send("Performance.getMetrics");
  const get = (name: string) => list.find((entry) => entry.name === name)?.value ?? 0;
  return { at: get("Timestamp"), task: get("TaskDuration"), script: get("ScriptDuration"), layout: get("LayoutDuration"), style: get("RecalcStyleDuration"), heap: get("JSHeapUsedSize"), nodes: get("Nodes"), listeners: get("JSEventListeners") };
}

/** Garbage collected first, so the heap is what the page holds. */
async function settled(cdp: CDPSession): Promise<Metrics> {
  await cdp.send("HeapProfiler.collectGarbage");
  return metrics(cdp);
}

async function window_(page: Page, run: () => Promise<void>, cdp: CDPSession) {
  await page.evaluate(() => {
    const m = (window as unknown as { __m: { long: unknown[]; events: unknown[]; frames: number[] } }).__m;
    m.long.length = 0;
    m.events.length = 0;
    m.frames.length = 0;
  });
  const before = await metrics(cdp);
  await run();
  const after = await metrics(cdp);
  const seen = await page.evaluate(() => (window as unknown as { __m: { long: [number, number][]; events: [string, number][]; frames: number[] } }).__m);
  const span = after.at - before.at;
  const gaps = seen.frames.slice(1).map((frame, at) => frame - (seen.frames[at] ?? frame));
  return {
    seconds: +span.toFixed(2),
    cpuPct: +(((after.task - before.task) / span) * 100).toFixed(1),
    scriptPct: +(((after.script - before.script) / span) * 100).toFixed(1),
    layoutPct: +(((after.layout - before.layout) / span) * 100).toFixed(1),
    stylePct: +(((after.style - before.style) / span) * 100).toFixed(1),
    longTasks: seen.long.length,
    longestMs: Math.round(Math.max(0, ...seen.long.map((entry) => entry[1]))),
    over100: seen.long.filter((entry) => entry[1] > 100).length,
    fps: gaps.length ? +(1000 / (gaps.reduce((sum, gap) => sum + gap, 0) / gaps.length)).toFixed(1) : 0,
    worstFrameMs: Math.round(Math.max(0, ...gaps)),
    slowestEventMs: Math.round(Math.max(0, ...seen.events.map((entry) => entry[1]))),
  };
}

export async function runOnce(count: number, browser: Browser): Promise<Record<string, unknown>> {
  const specs = seats(count);
  const mock = await startMock({ sessions: specs });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, serviceWorkers: "block", reducedMotion: "reduce" });
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.addInitScript(INSTRUMENT);
  const out: Record<string, unknown> = { seats: count };
  try {
    await page.goto(mock.url);
    await page.getByRole("tab", { name: /this Mac/ }).click();
    const t0 = Date.now();
    await page.locator("button.seat").nth(count - 1).waitFor({ timeout: 120_000 });
    out.listMs = Date.now() - t0;
    out.rows = await page.locator("button.seat").count();
    await page.waitForTimeout(2000);
    const loaded = await settled(cdp);
    out.heapMB = +(loaded.heap / 1e6).toFixed(1);
    out.nodes = loaded.nodes;
    out.listeners = loaded.listeners;
    out.quiet = await window_(page, () => page.waitForTimeout(6000), cdp);
    // The daemon pushes the full roster on every change. A working seat changes often.
    const frame = (k: number) => ({ type: "sessions", terminals: [], sessions: specs.map((spec, at) => (at % 10 === 0 ? { ...spec, context: { ...spec.context!, tokens: spec.context!.tokens + k * 90 }, last_reply: `turn ${k}` } : spec)).map(({ scene: _scene, ...rest }) => rest) });
    const spinner = (k: number) => `\r\x1b[2K\x1b[38;5;173m✻\x1b[0m Churning… (${k}s · ↑ ${k * 40} tokens · esc to interrupt)`;
    const profiling = Boolean(process.env.SCALE_PROFILE);
    // One roster frame's cost: 30 of them, 200 ms apart, working seats' fields changing.
    if (profiling) await cdp.send("Profiler.enable").then(() => cdp.send("Profiler.setSamplingInterval", { interval: 200 })).then(() => cdp.send("Profiler.start"));
    out.roster = await window_(page, async () => {
      for (let k = 0; k < 30; k++) {
        mock.push(frame(k));
        await page.waitForTimeout(200);
      }
    }, cdp);
    if (profiling) {
      const { profile } = await cdp.send("Profiler.stop");
      const self = new Map<string, number>();
      const nodes = new Map(profile.nodes.map((node) => [node.id, node]));
      profile.samples?.forEach((id, at) => {
        const call = nodes.get(id)!.callFrame;
        const key = `${call.functionName || "(anonymous)"} ${call.url.split("/").pop()}:${call.lineNumber + 1}`;
        self.set(key, (self.get(key) ?? 0) + (profile.timeDeltas?.[at] ?? 0));
      });
      const total = [...self.values()].reduce((sum, value) => sum + value, 0);
      out.profile = [...self.entries()].sort((x, y) => y[1] - x[1]).slice(0, 14).map(([key, us]) => `${((us / total) * 100).toFixed(1)}% ${key}`);
    }
    // The real daemon's frames arrive as little as 1 ms apart. Ten bursts of five.
    out.burst = await window_(page, async () => {
      for (let round = 0; round < 10; round++) {
        for (let k = 0; k < 5; k++) mock.push(frame(round * 5 + k));
        await page.waitForTimeout(1000);
      }
    }, cdp);
    // One seat in ten prints a spinner ten times a second. No roster frames.
    out.stream = await window_(page, async () => {
      for (let k = 0; k < 80; k++) {
        specs.forEach((spec, at) => at % 10 === 0 && mock.push({ type: "output", session: spec.name, data: Buffer.from(spinner(k), "utf8").toString("base64") }));
        await page.waitForTimeout(100);
      }
    }, cdp);
    // Open one seat, then move through ten more the way a person does.
    await page.locator("button.seat").first().click();
    await page.locator(".xterm-rows").waitFor();
    await page.waitForTimeout(800);
    const clicks: number[] = [];
    out.switching = await window_(page, async () => {
      for (let k = 1; k <= 10; k++) {
        const ms = await page.evaluate(async (at) => {
          const rows = [...document.querySelectorAll<HTMLElement>("button.seat")];
          const started = performance.now();
          rows[at]?.click();
          await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)));
          return performance.now() - started;
        }, Math.min(k * Math.max(1, Math.floor(count / 11)), count - 1));
        clicks.push(Math.round(ms));
        await page.waitForTimeout(350);
      }
    }, cdp);
    out.clickToPaintMs = clicks;
    const composer = page.locator("#composer-input");
    await composer.click();
    out.typing = await window_(page, () => composer.pressSequentially("typing while a hundred seats sit open", { delay: 25 }), cdp);
    const end = await settled(cdp);
    out.heapAfterMB = +(end.heap / 1e6).toFixed(1);
    out.nodesAfter = end.nodes;
  } finally {
    await context.close();
    await mock.stop();
  }
  return out;
}

/** The real client on a real daemon, read only: the list and idle. It never opens a seat,
 * since attaching with a size would resize live terminals. */
export async function observeLive(url: string, browser: Browser): Promise<Record<string, unknown>> {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, serviceWorkers: "block", reducedMotion: "reduce" });
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.addInitScript(INSTRUMENT);
  const out: Record<string, unknown> = { url };
  try {
    await page.goto(url);
    await page.getByRole("tab", { name: /this Mac/ }).click();
    await page.locator("button.seat").first().waitFor({ timeout: 60_000 });
    await page.waitForTimeout(3000);
    out.rows = await page.locator("button.seat").count();
    const loaded = await settled(cdp);
    out.heapMB = +(loaded.heap / 1e6).toFixed(1);
    out.nodes = loaded.nodes;
    // Every websocket frame received in the window, by type, to divide cost by.
    const heard = new Map<string, { n: number; bytes: number }>();
    await cdp.send("Network.enable");
    cdp.on("Network.webSocketFrameReceived", (event) => {
      const kind = /"type":"(\w+)"/.exec(event.response.payloadData.slice(0, 80))?.[1] ?? "?";
      const got = heard.get(kind) ?? { n: 0, bytes: 0 };
      got.n += 1;
      got.bytes += event.response.payloadData.length;
      heard.set(kind, got);
    });
    out.idle = await window_(page, () => page.waitForTimeout(20_000), cdp);
    out.frames = Object.fromEntries([...heard].map(([kind, got]) => [kind, `${got.n} frames, ${Math.round(got.bytes / 1000)} KB`]));
  } finally {
    await context.close();
  }
  return out;
}

async function opened(count: number, browser: Browser) {
  const mock = await startMock({ sessions: seats(count) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, serviceWorkers: "block", reducedMotion: "reduce" });
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.addInitScript(INSTRUMENT);
  await page.goto(mock.url);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await page.locator("button.seat").nth(count - 1).waitFor({ timeout: 120_000 });
  await page.waitForTimeout(1500);
  return { mock, page, cdp, close: async () => { await context.close(); await mock.stop(); } };
}

/** Long tasks over 100 ms while moving through seats and typing, at this many seats. */
export async function longTasksWhileUsed(count: number, browser: Browser): Promise<{ rows: number; over100: number; longestMs: number }> {
  const { page, close } = await opened(count, browser);
  try {
    const rows = await page.locator("button.seat").count();
    await page.evaluate(() => ((window as unknown as { __m: { long: unknown[] } }).__m.long.length = 0));
    await page.locator("button.seat").first().click();
    await page.locator(".xterm-rows").waitFor();
    for (let k = 1; k <= 8; k++) {
      await page.locator("button.seat").nth(Math.floor((k * count) / 9)).click();
      await page.waitForTimeout(250);
    }
    await page.locator("#composer-input").pressSequentially("typing across a hundred seats", { delay: 20 });
    const long = await page.evaluate(() => (window as unknown as { __m: { long: [number, number][] } }).__m.long);
    return { rows, over100: long.filter((entry) => entry[1] > 100).length, longestMs: Math.round(Math.max(0, ...long.map((entry) => entry[1]))) };
  } finally {
    await close();
  }
}

const longItem = (seat: number, n: number) => {
  const at = new Date(Date.UTC(2026, 10, 11, 0, 0, 0) + n * 20_000).toISOString();
  if (n % 3 === 0) return { id: `s${seat}-p${n}`, kind: "prompt", text: `Seat ${seat} question ${n}: why does the wrapped row hide the stripe in the folded view?`, at };
  if (n % 3 === 1) return { id: `s${seat}-t${n}`, kind: "tool", tool: { name: "Bash", class: "shell", summary: `Ran step ${n}`, output: Array.from({ length: 40 }, (_, line) => `line ${line} of the output of step ${n}`).join("\n"), ok: true } };
  return { id: `s${seat}-r${n}`, kind: "reply", text: `Answer ${n}. ${"It names the cause and the file, and says what it changed. ".repeat(6)}`, complete: true, at };
};

/** The open seat after a long run: `earlier` pages read back, then windows of arrivals.
 * Each seat has `history` items. Heap is read after a collection: what is held. */
export async function longHistory(browser: Browser, options: { seats: number; history: number; earlier: number; windows: number }): Promise<Record<string, unknown>> {
  const specs = seats(options.seats);
  const mock = await startMock({ sessions: specs, transcript: true });
  specs.forEach((spec, seat) => mock.setTranscript(spec.name, Array.from({ length: options.history }, (_, n) => longItem(seat, n))));
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, serviceWorkers: "block", reducedMotion: "reduce" });
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.addInitScript(INSTRUMENT);
  const out: Record<string, unknown> = { ...options };
  try {
    await page.goto(mock.url);
    await page.getByRole("tab", { name: /this Mac/ }).click();
    await page.locator("button.seat").nth(options.seats - 1).waitFor({ timeout: 120_000 });
    await page.locator("button.seat").first().click();
    await page.getByRole("radio", { name: "Focus" }).click();
    await page.getByText(/From the transcript/).waitFor({ timeout: 30_000 });
    for (let at = 0; at < options.earlier; at++) {
      const button = page.locator(".earlier");
      if ((await button.count()) === 0) break;
      await button.click();
      await page.waitForTimeout(60);
      await page.waitForFunction(() => !document.querySelector<HTMLButtonElement>(".earlier")?.disabled);
    }
    const latest = page.getByRole("button", { name: /Jump to latest/ });
    if (await latest.count()) await latest.click();
    await page.waitForTimeout(500);
    out.rowsInDom = await page.locator(".transcript .feed > :is(.turn, .step, .note)").count();
    const samples: Record<string, unknown>[] = [];
    let next = options.history;
    for (let w = 0; w < options.windows; w++) {
      const sample = await window_(page, async () => {
        for (let beat = 0; beat < 5; beat++) {
          mock.appendTranscript(specs[0]!.name, [longItem(0, next++), longItem(0, next++), longItem(0, next++)]);
          await page.waitForTimeout(2000);
        }
      }, cdp);
      const held = await settled(cdp);
      samples.push({ w, heapMB: +(held.heap / 1e6).toFixed(1), nodes: held.nodes, ...sample });
    }
    out.samples = samples;
  } finally {
    await context.close();
    await mock.stop();
  }
  return out;
}
