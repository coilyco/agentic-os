import { afterEach, describe, expect, it, vi } from "vitest";
import { probe } from "./daemon-host";

// undici raises `error` from inside close() on a socket that is still connecting.
class ClosingRaisesError {
  static closes = 0;
  private handlers: Record<string, Array<() => void>> = {};
  addEventListener(type: string, handler: () => void): void {
    (this.handlers[type] ??= []).push(handler);
  }
  close(): void {
    ClosingRaisesError.closes++;
    if (ClosingRaisesError.closes > 50) throw new RangeError("Maximum call stack size exceeded");
    for (const handler of this.handlers.error ?? []) handler();
  }
}

afterEach(() => {
  vi.unstubAllGlobals();
  ClosingRaisesError.closes = 0;
});

describe("probe", () => {
  it("closes a socket that never connected once, though closing raises an error", async () => {
    vi.stubGlobal("WebSocket", ClosingRaisesError);
    await expect(probe("ws://127.0.0.1:1", 20)).rejects.toThrow("no answer");
    expect(ClosingRaisesError.closes).toBe(1);
  });
});
