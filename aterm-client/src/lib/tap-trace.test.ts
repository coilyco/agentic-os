import { afterEach, describe, expect, it, vi } from "vitest";
import { clear, describe as describeShape, format, mark, records, traceWanted, TRACE_TEXT_MAX, type TraceEvent } from "./tap-trace";

describe("the flag", () => {
  it("is off with no flag and on with one", () => {
    expect(traceWanted("")).toBe(false);
    expect(traceWanted("?other=1")).toBe(false);
    expect(traceWanted("?taptrace")).toBe(true);
    expect(traceWanted("?taptrace=1")).toBe(true);
  });
  it("can be said off", () => {
    expect(traceWanted("?taptrace=0")).toBe(false);
    expect(traceWanted("?taptrace=off")).toBe(false);
  });
});

describe("a target", () => {
  it("is told by tag, id, class words, place and state, and never by its text", () => {
    const option = { tagName: "BUTTON", className: "option svelte-1wg5bl", index: 2, disabled: false, textContent: "Behind a button", value: "secret" };
    expect(describeShape(option)).toBe("button.option[2]");
    expect(describeShape({ tagName: "BUTTON", className: "button primary", disabled: true })).toBe("button.button.primary:disabled");
    expect(describeShape(option)).not.toContain("Behind");
  });
  it("drops a class or id that looks like content", () => {
    expect(describeShape({ tagName: "DIV", id: "Seat Imp-Dragonfly is asking", className: "Which Accent Color" })).toBe("div");
  });
  it("says none for nothing", () => {
    expect(describeShape(null)).toBe("none");
  });
});

const event = (t: number): TraceEvent => ({ t, type: "click", target: "button.option[1]", x: 10, y: 20, renderer: "daemon-card" });

describe("the text handed back", () => {
  it("is ordered lines under a header", () => {
    const text = format([event(0), { t: 12.3, type: "card-gone", target: "app", renderer: "terminal" }], "taptrace v1");
    expect(text.split("\n")).toEqual(["taptrace v1", "+0.0 click button.option[1] @10,20 [daemon-card]", "+12.3 card-gone app [terminal]"]);
  });
  it("drops the oldest events to fit, and says so", () => {
    const many = Array.from({ length: 400 }, (_, i) => event(i));
    const text = format(many, "taptrace v1");
    expect(text.length).toBeLessThanOrEqual(TRACE_TEXT_MAX);
    expect(text).toMatch(/\(\d+ earlier events dropped\)/);
    expect(text.trimEnd().endsWith("[daemon-card]")).toBe(true);
    expect(text).toContain("+399.0 click");
  });
});

describe("when off", () => {
  afterEach(() => vi.restoreAllMocks());
  it("holds nothing and says nothing to the console", () => {
    const spies = (["log", "info", "warn", "error", "debug"] as const).map((name) => vi.spyOn(console, name));
    clear();
    mark("answer", "daemon-card");
    expect(records()).toEqual([]);
    for (const spy of spies) expect(spy).not.toHaveBeenCalled();
  });
});
