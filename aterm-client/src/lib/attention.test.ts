import { describe, expect, it } from "vitest";
import { ATTENTION_OFF, detectCue, parseAttention } from "./attention";

describe("detectCue", () => {
  it("fires when a seat enters waiting", () => {
    expect(detectCue(new Set(), ["a"], null)).toEqual({ known: new Set(["a"]), fire: true });
  });

  it("records a seat already waiting at connect without firing, then stays quiet", () => {
    const baseline = detectCue(null, ["a"], null);
    expect(baseline.fire).toBe(false);
    expect(detectCue(baseline.known, ["a"], null).fire).toBe(false);
  });

  it("fires once for two seats entering together", () => {
    expect(detectCue(new Set(), ["a", "b"], null).fire).toBe(true);
  });

  it("does not fire for the open seat", () => {
    expect(detectCue(new Set(), ["a"], "a").fire).toBe(false);
  });

  it("still fires for a second seat when the open one is also waiting", () => {
    expect(detectCue(new Set(), ["a", "b"], "a").fire).toBe(true);
  });

  it("fires again when a seat leaves waiting and returns", () => {
    const left = detectCue(new Set(["a"]), [], null);
    expect(detectCue(left.known, ["a"], null).fire).toBe(true);
  });

  it("does not fire when a seat leaves waiting", () => {
    expect(detectCue(new Set(["a"]), [], null).fire).toBe(false);
  });
});

describe("parseAttention", () => {
  it("reads both switches off when nothing is stored", () => {
    expect(parseAttention(null)).toEqual(ATTENTION_OFF);
  });

  it("round-trips each switch on its own", () => {
    expect(parseAttention('{"sound":true,"visual":false}')).toEqual({ sound: true, visual: false });
    expect(parseAttention('{"sound":false,"visual":true}')).toEqual({ sound: false, visual: true });
  });

  it("reads anything malformed as off", () => {
    expect(parseAttention("not json")).toEqual(ATTENTION_OFF);
    expect(parseAttention('{"sound":"yes"}')).toEqual(ATTENTION_OFF);
    expect(parseAttention("[]")).toEqual(ATTENTION_OFF);
  });
});
