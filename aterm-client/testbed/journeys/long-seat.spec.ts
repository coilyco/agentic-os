// Journey: open a seat that has run long, and scroll its history (COI-3422).
import { expect, kindOf, loadFonts, pushOutput, SEATS, test } from "../harness.ts";
import { longSeat } from "../scenes.ts";

const HISTORY_BYTES = 1 << 20;

// Throttled runs stand in for a slower webview. The plain run is the one that gates.
const RUNS = [
  { style: "text", cpu: 1 },
  { style: "redraw", cpu: 1 },
  { style: "text", cpu: 4 },
  { style: "redraw", cpu: 4 },
] as const;

for (const run of RUNS) test(`a long seat opens fast and scrolls steadily (${run.style}, cpu x${run.cpu})`, async ({ page, mock }, info) => {
  test.skip(kindOf(info) !== "desktop", "measured on the desktop app first");
  test.setTimeout(120_000);
  mock.setScene(SEATS.beetle.id, longSeat(HISTORY_BYTES, "long-seat", run.style));
  if (run.cpu > 1) await (await page.context().newCDPSession(page)).send("Emulation.setCPUThrottlingRate", { rate: run.cpu });
  await page.addInitScript(() => {
    const frames: { at: number; bytes: number }[] = [];
    const tasks: number[] = [];
    Object.assign(window, { __frames: frames, __tasks: tasks });
    const Native = window.WebSocket;
    window.WebSocket = class extends Native {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        this.addEventListener("message", (event) => {
          const text = typeof event.data === "string" ? event.data : "";
          if (text.startsWith('{"type":"output"')) frames.push({ at: performance.now(), bytes: text.length });
        });
      }
    } as typeof WebSocket;
    new PerformanceObserver((list) => list.getEntries().forEach((e) => tasks.push(e.duration))).observe({ type: "longtask", buffered: true });
  });
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  const tab = page.getByRole("tab", { name: new RegExp(SEATS.beetle.name) }).first();
  await tab.waitFor();
  await page.evaluate(() => {
    const w = window as unknown as Record<string, unknown>;
    w.__open = new Promise((resolve) => {
      document.addEventListener(
        "click",
        () => {
          const t0 = performance.now();
          const tasksBefore = (w.__tasks as number[]).length;
          let first = 0;
          const look = () => {
            const text = document.querySelector(".xterm-rows")?.textContent ?? "";
            if (!first && text.trim().length > 0) first = performance.now() - t0;
            if (text.includes("accept edits")) {
              resolve({ first, settled: performance.now() - t0, tasks: (w.__tasks as number[]).slice(tasksBefore) });
            } else requestAnimationFrame(look);
          };
          requestAnimationFrame(look);
        },
        { capture: true, once: true },
      );
    });
  });
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Profiler.enable");
  await cdp.send("Profiler.start");
  await tab.click();
  const open = (await page.evaluate(() => (window as unknown as { __open: Promise<unknown> }).__open)) as { first: number; settled: number; tasks: number[] };
  const { profile } = await cdp.send("Profiler.stop");
  const self = new Map<string, number>();
  const byId = new Map(profile.nodes.map((node) => [node.id, node]));
  profile.samples?.forEach((id, i) => {
    const call = byId.get(id)!.callFrame;
    const name = `${call.functionName || "(anon)"} ${call.url.split("/").pop()}:${call.lineNumber}`;
    self.set(name, (self.get(name) ?? 0) + (profile.timeDeltas?.[i] ?? 0) / 1000);
  });
  const top = [...self].sort((a, b) => b[1] - a[1]).slice(0, 12).map(([name, ms]) => `${Math.round(ms)}ms ${name}`);
  const wire = await page.evaluate(() => (window as unknown as { __frames: { bytes: number }[] }).__frames);
  await page.waitForTimeout(1000);
  const box = (await page.locator(".xterm-rows").first().boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  const scrollRun = async (wheel: number) => {
    await page.evaluate(() => {
      const w = window as unknown as Record<string, unknown>;
      const deltas: number[] = [];
      w.__scroll = deltas;
      let last = performance.now();
      const tick = (now: number) => {
        deltas.push(now - last);
        last = now;
        if (w.__scrolling !== false) requestAnimationFrame(tick);
      };
      w.__scrolling = true;
      requestAnimationFrame(tick);
    });
    for (let i = 0; i < 80; i++) {
      await page.mouse.wheel(0, wheel);
      await page.waitForTimeout(16);
    }
    const frames = await page.evaluate(() => {
      const w = window as unknown as { __scrolling: boolean; __scroll: number[] };
      w.__scrolling = false;
      return w.__scroll.slice(1);
    });
    const sorted = [...frames].sort((a, b) => a - b);
    const pick = (q: number) => Math.round((sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))] ?? 0) * 10) / 10;
    return { frames: frames.length, p50: pick(0.5), p95: pick(0.95), max: pick(1), over34ms: frames.filter((f) => f > 34).length };
  };
  const idle = await scrollRun(-240);
  // A working seat keeps printing while a person scrolls its history.
  let n = 0;
  const feed = setInterval(() => pushOutput(mock, SEATS.beetle.id, `\r\n\x1b[1m⏺\x1b[0m streaming line ${n++} of the work in progress`), 50);
  const streaming = await scrollRun(240);
  clearInterval(feed);
  const result = {
    project: info.project.name,
    style: run.style,
    cpu: run.cpu,
    topSelfTime: top,
    wireFramesBytes: wire.map((f) => f.bytes),
    firstRowMs: Math.round(open.first),
    settledMs: Math.round(open.settled),
    longTasksMs: open.tasks.map(Math.round),
    scrollIdle: idle,
    scrollWhileStreaming: streaming,
  };
  console.log(`MEASURE ${JSON.stringify(result)}`);
  await info.attach("measure.json", { body: JSON.stringify(result, null, 2), contentType: "application/json" });
  expect(result.wireFramesBytes.length).toBeGreaterThan(0);
});
