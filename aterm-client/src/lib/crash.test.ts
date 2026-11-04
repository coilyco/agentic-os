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

  it("drops the host from a fetch breadcrumb to a tailnet host and keeps path and method", () => {
    const crumb = crash.beforeBreadcrumb({
      category: "fetch",
      type: "http",
      data: { url: "https://tower.example.ts.net:7419/api/sessions?limit=5", method: "POST", status_code: 200 },
    });
    expect(crumb.data).toEqual({ url: "https://[host]/api/sessions?limit=5", method: "POST", status_code: 200 });
    expect(JSON.stringify(crumb)).not.toMatch(/tower|example\.ts\.net|7419/);
  });

  it("is wired into Sentry for breadcrumbs and events", () => {
    crash.initCrashReporting("https://public@example.invalid/1");
    const options = init.mock.calls[0]?.[0];
    const crumb = options.beforeBreadcrumb({ category: "fetch", data: { url: "http://tower.example.ts.net/x" } });
    expect(crumb.data.url).toBe("http://[host]/x");
    expect(options.beforeSend).toBe(crash.beforeSend);
  });

  it("drops every host from an event: request, headers, exception, frames, navigation and extra", () => {
    const names = /tower|ghost|example\.ts\.net|100\.64\.0\.7|fd00::7|operator|7419/;
    const event = crash.beforeSend({
      type: undefined,
      request: {
        url: "https://tower.example.ts.net/#/hosts",
        headers: { Referer: "https://tower.example.ts.net:7419/app/", "User-Agent": "test" },
      },
      exception: {
        values: [
          {
            type: "TypeError",
            value: "WebSocket to wss://operator@ghost.example.ts.net:7419/ws failed",
            stacktrace: {
              frames: [{ filename: "https://100.64.0.7:7419/assets/app.js", abs_path: "https://100.64.0.7:7419/assets/app.js" }],
            },
          },
        ],
      },
      breadcrumbs: [
        { category: "navigation", data: { from: "https://tower.example.ts.net/a", to: "https://tower.example.ts.net/b" } },
        { category: "fetch", data: { url: "wss://[fd00::7]:7419/ws", method: "GET" } },
      ],
      extra: { address: "https://ghost.example.ts.net:7419/health" },
    });
    expect(JSON.stringify(event)).not.toMatch(names);
    expect(event?.request?.url).toBe("https://[host]/#/hosts");
    expect(event?.exception?.values?.[0]?.stacktrace?.frames?.[0]?.abs_path).toBe("https://[host]/assets/app.js");
    expect(event?.breadcrumbs?.[1]?.data).toEqual({ url: "wss://[host]/ws", method: "GET" });
    expect(event?.breadcrumbs?.[0]?.data).toEqual({ from: "https://[host]/a", to: "https://[host]/b" });
  });

  it("leaves text that holds no url alone", () => {
    expect(crash.scrubUrl("pane crashed at /api/sessions: no host here")).toBe("pane crashed at /api/sessions: no host here");
  });

  it("drops a value nested past the depth cap instead of letting a host through", () => {
    let deep: unknown = { url: "https://tower.example.ts.net/x" };
    for (let i = 0; i < 40; i++) deep = { next: deep };
    expect(JSON.stringify(crash.scrubUrls(deep))).not.toContain("tower");
  });

  it("caps events per minute and recovers", () => {
    const later = Date.now() + 3_600_000;
    const allowed = Array.from({ length: 21 }, () => crash.withinBudget(later));
    expect(allowed.filter(Boolean)).toHaveLength(20);
    expect(crash.withinBudget(later + 61_000)).toBe(true);
  });
});
