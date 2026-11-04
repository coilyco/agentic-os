import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RETRY_SPAN_MS } from "./backoff";
import { Recheck } from "./recheck";

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(Math, "random").mockReturnValue(0.5);
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("Recheck", () => {
  it("asks on the backoff until the host answers, then stops", async () => {
    let up = false;
    const ask = vi.fn(async () => up);
    const recheck = new Recheck(ask);
    recheck.start();
    await vi.advanceTimersByTimeAsync(1000);
    await vi.advanceTimersByTimeAsync(2000);
    expect(ask).toHaveBeenCalledTimes(2);
    up = true;
    await vi.advanceTimersByTimeAsync(4000);
    expect(ask).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(RETRY_SPAN_MS);
    expect(ask).toHaveBeenCalledTimes(3);
  });

  it("makes its last automatic ask an hour after the first miss, then goes quiet", async () => {
    const ask = vi.fn(async () => false);
    const recheck = new Recheck(ask);
    recheck.start();
    await vi.advanceTimersByTimeAsync(RETRY_SPAN_MS - 1);
    const before = ask.mock.calls.length;
    await vi.advanceTimersByTimeAsync(1);
    expect(ask.mock.calls.length).toBe(before + 1);
    await vi.advanceTimersByTimeAsync(RETRY_SPAN_MS * 2);
    expect(ask.mock.calls.length).toBe(before + 1);
  });

  it("asks at once on a wake, even after the hour, and starts the hour over", async () => {
    let up = false;
    const ask = vi.fn(async () => up);
    const recheck = new Recheck(ask);
    recheck.start();
    await vi.advanceTimersByTimeAsync(RETRY_SPAN_MS + 1);
    const spent = ask.mock.calls.length;
    recheck.wake();
    await vi.advanceTimersByTimeAsync(0);
    expect(ask.mock.calls.length).toBe(spent + 1);
    await vi.advanceTimersByTimeAsync(1000);
    expect(ask.mock.calls.length).toBe(spent + 2);
    up = true;
    await vi.advanceTimersByTimeAsync(2000);
    expect(ask.mock.calls.length).toBe(spent + 3);
  });

  it("does not stack asks when a wake lands while one is out", async () => {
    let finish!: (answered: boolean) => void;
    const ask = vi.fn(() => new Promise<boolean>((resolve) => (finish = resolve)));
    const recheck = new Recheck(ask);
    recheck.wake();
    recheck.wake();
    recheck.start();
    expect(ask).toHaveBeenCalledTimes(1);
    finish(true);
    await vi.advanceTimersByTimeAsync(RETRY_SPAN_MS);
    expect(ask).toHaveBeenCalledTimes(1);
  });

  it("treats a thrown ask as a miss", async () => {
    const ask = vi.fn(async () => {
      throw new Error("no route");
    });
    const recheck = new Recheck(ask);
    recheck.start();
    await vi.advanceTimersByTimeAsync(3000);
    expect(ask).toHaveBeenCalledTimes(2);
  });
});
