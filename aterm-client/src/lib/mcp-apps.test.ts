import { describe, expect, it, vi } from "vitest";
import { cspFor, resultText, ViewBridge, viewDocument, type HostContext, type ViewHandlers } from "./mcp-apps";

const context: HostContext = {
  theme: "dark",
  displayMode: "inline",
  availableDisplayModes: ["inline", "fullscreen"],
  platform: "web",
  locale: "en-US",
  timeZone: "UTC",
  containerDimensions: { maxHeight: 600, width: 360 },
};

function bridge(overrides: Partial<ViewHandlers> = {}) {
  const posted: Record<string, unknown>[] = [];
  const handlers: ViewHandlers = {
    callTool: vi.fn(async () => ({ content: [{ type: "text", text: "ok" }] })),
    readResource: vi.fn(async () => ({ contents: [] })),
    openLink: vi.fn(),
    resize: vi.fn(),
    displayMode: vi.fn((mode) => mode),
    ...overrides,
  };
  const view = new ViewBridge((message) => posted.push(message as Record<string, unknown>), handlers, context);
  return { view, posted, handlers };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const methods = (posted: Record<string, unknown>[]) => posted.map((message) => message.method).filter(Boolean);

describe("viewDocument", () => {
  it("pins the policy first inside an existing head", () => {
    const html = viewDocument('<html><head><script src="x.js"></script></head></html>');
    expect(html.indexOf("Content-Security-Policy")).toBeLessThan(html.indexOf("x.js"));
  });

  it("wraps a fragment with no head", () => {
    expect(viewDocument("<p>hi</p>")).toMatch(/^<!doctype html><html><head><meta http-equiv="Content-Security-Policy"/);
  });
});

describe("cspFor", () => {
  it("blocks the network unless a domain is declared", () => {
    expect(cspFor()).toContain("connect-src 'none'");
    expect(cspFor({ connectDomains: ["https://api.example.com"] })).toContain("connect-src https://api.example.com");
  });
});

describe("resultText", () => {
  it("joins text blocks and says so when there are none", () => {
    expect(resultText({ content: [{ type: "text", text: "a" }, { type: "image" }, { type: "text", text: "b" }] })).toBe("a\n\nb");
    expect(resultText(undefined)).toBe("The tool returned no text.");
  });
});

describe("ViewBridge", () => {
  it("answers initialize with the host context", async () => {
    const { view, posted } = bridge();
    view.receive({ jsonrpc: "2.0", id: 1, method: "ui/initialize", params: {} });
    await settle();
    expect(posted[0]).toMatchObject({ id: 1, result: { hostContext: { theme: "dark" } } });
    expect(view.started).toBe(true);
  });

  it("holds input and result until initialized, then sends input first", async () => {
    const { view, posted } = bridge();
    view.setResult({ content: [{ type: "text", text: "done" }] });
    view.setInput({ q: 1 });
    expect(posted).toHaveLength(0);
    view.receive({ jsonrpc: "2.0", method: "ui/notifications/initialized", params: {} });
    expect(methods(posted)).toEqual(["ui/notifications/tool-input", "ui/notifications/tool-result"]);
  });

  it("never sends a result before its input", () => {
    const { view, posted } = bridge();
    view.receive({ jsonrpc: "2.0", method: "ui/notifications/initialized" });
    view.setResult({ content: [] });
    expect(methods(posted)).toEqual([]);
    view.setInput({});
    expect(methods(posted)).toEqual(["ui/notifications/tool-input", "ui/notifications/tool-result"]);
  });

  it("sends a cancellation in place of a result", () => {
    const { view, posted } = bridge();
    view.receive({ jsonrpc: "2.0", method: "ui/notifications/initialized" });
    view.setInput({});
    view.cancel("the seat stopped");
    expect(posted.at(-1)).toMatchObject({ method: "ui/notifications/tool-cancelled", params: { reason: "the seat stopped" } });
  });

  it("proxies a view's tool call and returns its result", async () => {
    const { view, posted, handlers } = bridge();
    view.receive({ jsonrpc: "2.0", id: "a", method: "tools/call", params: { name: "refresh", arguments: { n: 2 } } });
    await settle();
    expect(handlers.callTool).toHaveBeenCalledWith("refresh", { n: 2 });
    expect(posted[0]).toMatchObject({ id: "a", result: { content: [{ text: "ok" }] } });
  });

  it("turns a failed call into a JSON-RPC error", async () => {
    const { view, posted } = bridge({ callTool: async () => Promise.reject(new Error("the daemon refused")) });
    view.receive({ jsonrpc: "2.0", id: 2, method: "tools/call", params: { name: "x" } });
    await settle();
    expect(posted[0]).toMatchObject({ id: 2, error: { message: "the daemon refused" } });
  });

  it("refuses methods it does not serve, and links that are not web links", async () => {
    const { view, posted, handlers } = bridge();
    view.receive({ jsonrpc: "2.0", id: 3, method: "ui/message", params: {} });
    view.receive({ jsonrpc: "2.0", id: 4, method: "ui/open-link", params: { url: "javascript:alert(1)" } });
    await settle();
    expect(posted[0]).toMatchObject({ id: 3, error: { code: -32601 } });
    expect(posted[1]).toMatchObject({ id: 4, error: { code: -32602 } });
    expect(handlers.openLink).not.toHaveBeenCalled();
  });

  it("passes size changes and ignores anything that is not JSON-RPC", () => {
    const { view, posted, handlers } = bridge();
    view.receive({ jsonrpc: "2.0", method: "ui/notifications/size-changed", params: { width: 300, height: 240 } });
    view.receive("not rpc");
    view.receive({ method: "tools/call" });
    expect(handlers.resize).toHaveBeenCalledWith(240);
    expect(posted).toHaveLength(0);
  });

  it("resolves teardown on the view's reply", async () => {
    const { view, posted } = bridge();
    view.receive({ jsonrpc: "2.0", id: 1, method: "ui/initialize" });
    await settle();
    const done = view.teardown("closed", 10_000);
    const request = posted.find((message) => message.method === "ui/resource-teardown")!;
    view.receive({ jsonrpc: "2.0", id: request.id, result: {} });
    await expect(done).resolves.toBeUndefined();
  });
});
