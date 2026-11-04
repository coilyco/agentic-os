import { afterEach, describe, expect, it, vi } from "vitest";
import { Terminal } from "@xterm/xterm";
import { afterQueued, safely, watchDelay } from "./pane-watch";

afterEach(() => vi.restoreAllMocks());

// A harness block redrawn often: erase three rows, print three. It wraps at 30.
function frames(count: number): string {
  let out = "";
  for (let index = 0; index < count; index++) {
    if (index > 0) out += "\x1b[3A\x1b[J";
    for (let line = 0; line < 3; line++) out += `frame ${String(index).padStart(3, "0")} line ${line} ${"x".repeat(21)}\r\n`;
  }
  return out;
}

function screen(term: Terminal): string[] {
  const buffer = term.buffer.active;
  return Array.from({ length: buffer.length }, (_, row) => buffer.getLine(row)?.translateToString(true) ?? "");
}

function drawn(resize: (term: Terminal) => void): Promise<string[]> {
  const term = new Terminal({ cols: 40, rows: 12, scrollback: 100, allowProposedApi: true });
  const data = frames(400);
  for (let at = 0; at < data.length; at += 200) term.write(data.slice(at, at + 200));
  resize(term);
  return new Promise((done) => term.write("", () => done(screen(term))));
}

describe("afterQueued", () => {
  it("draws the queued bytes at the old size, as a window that had drained would", async () => {
    const drained = await new Promise<string[]>((done) => {
      const term = new Terminal({ cols: 40, rows: 12, scrollback: 100, allowProposedApi: true });
      term.write(frames(400), () => {
        term.resize(30, 12);
        done(screen(term));
      });
    });
    const ordered = await drawn((term) => afterQueued(term, "resizing", () => term.resize(30, 12)));
    const bare = await drawn((term) => term.resize(30, 12));
    expect(bare).not.toEqual(drained);
    expect(ordered).toEqual(drained);
  });

  it("reports a throwing step and leaves the terminal taking writes", async () => {
    const report = vi.spyOn(console, "error").mockImplementation(() => {});
    const term = new Terminal({ cols: 10, rows: 3, allowProposedApi: true });
    afterQueued(term, "resizing", () => {
      throw new Error("boom");
    });
    await new Promise<void>((done) => term.write("ok", done));
    expect(String(report.mock.calls[0]?.[0])).toContain("resizing");
    expect(screen(term)[0]).toBe("ok");
  });
});

describe("watchDelay", () => {
  it("waits longer before each ask and stops after three", () => {
    expect([0, 1, 2].map(watchDelay)).toEqual([2_500, 5_000, 10_000]);
    expect(watchDelay(3)).toBeNull();
  });
});

describe("safely", () => {
  it("runs the step", () => {
    const step = vi.fn();
    safely("a step", step);
    expect(step).toHaveBeenCalledOnce();
  });

  it("reports a throw and returns, so the steps after it still run", () => {
    const report = vi.spyOn(console, "error").mockImplementation(() => {});
    const after = vi.fn();
    safely("marking envelopes", () => {
      throw new Error("boom");
    });
    after();
    expect(report).toHaveBeenCalledOnce();
    expect(String(report.mock.calls[0]?.[0])).toContain("marking envelopes");
    expect(after).toHaveBeenCalledOnce();
  });
});
