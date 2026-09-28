import { beforeEach, describe, expect, it, vi } from "vitest";

const init = vi.fn();
const failure = { next: undefined as Error | undefined };
vi.mock("@sentry/svelte", () => ({
  init: (options: unknown) => {
    if (failure.next) throw failure.next;
    init(options);
  },
}));

const crash = await import("./crash");

describe("crash reporting", () => {
  beforeEach(() => init.mockReset());

  it("stays off without a DSN", () => {
    expect(crash.initCrashReporting("")).toBe(false);
    expect(crash.initCrashReporting(undefined)).toBe(false);
    expect(init).not.toHaveBeenCalled();
  });

  it("keeps default integrations and adds no tracing, replay, or console capture", () => {
    expect(crash.initCrashReporting("https://public@example.invalid/1")).toBe(true);
    const options = init.mock.calls[0]?.[0];
    expect(options.sendDefaultPii).toBe(false);
    expect(options.tracesSampleRate).toBe(0);
    expect(options.integrations).toBeUndefined();
  });

  it("logs only the error name when init fails", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    failure.next = new TypeError("https://secret-key@o0.ingest.example/1");
    expect(crash.initCrashReporting("https://secret-key@o0.ingest.example/1")).toBe(false);
    expect(String(warn.mock.calls[0]?.[0])).toContain("TypeError");
    expect(String(warn.mock.calls[0]?.[0])).not.toContain("secret-key");
    failure.next = undefined;
  });

  it("scrubs terminal traffic and keeps the exception readable", () => {
    const secret = ["TERMINAL", "SECRET"].join("-");
    const event = crash.beforeSend({
      type: undefined,
      exception: { values: [{ type: "TypeError", value: "pane crashed" }] },
      extra: { session: { input: secret, seat: "eng-platform" } },
      breadcrumbs: [{ category: "ws", data: { payload: secret, kind: "frame" } }],
    });
    expect(event?.exception?.values?.[0]?.value).toBe("pane crashed");
    expect(JSON.stringify(event)).not.toContain(secret);
    expect(JSON.stringify(event)).toContain("eng-platform");
  });

  it("caps events per minute and recovers", () => {
    const later = Date.now() + 3_600_000;
    const allowed = Array.from({ length: 21 }, () => crash.withinBudget(later));
    expect(allowed.filter(Boolean)).toHaveLength(20);
    expect(crash.withinBudget(later + 61_000)).toBe(true);
  });
});
