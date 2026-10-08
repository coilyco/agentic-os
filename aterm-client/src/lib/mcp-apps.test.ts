import { describe, expect, it, vi } from "vitest";
import { InMemoryTransport, type JSONRPCMessage } from "@modelcontextprotocol/client";
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

const INITIALIZE = { appInfo: { name: "view", version: "1" }, appCapabilities: {}, protocolVersion: "2026-01-26" };

// The view's end of an in-memory pair: `posted` holds what it got, `send` speaks for it.
async function bridge(overrides: Partial<ViewHandlers> = {}) {
  const posted: Record<string, unknown>[] = [];
  const handlers: ViewHandlers = {
    callTool: vi.fn(async () => ({ content: [{ type: "text", text: "ok" }] })),
    readResource: vi.fn(async () => ({ contents: [] })),
    openLink: vi.fn(),
    resize: vi.fn(),
    displayMode: vi.fn((mode) => mode),
    started: vi.fn(),
    ...overrides,
  };
  const view = new ViewBridge(handlers, context);
  const [host, peer] = InMemoryTransport.createLinkedPair();
  peer.onmessage = (message) => posted.push(message as unknown as Record<string, unknown>);
  await view.connect(host);
  await peer.start();
  const send = async (message: Record<string, unknown>) => {
    await peer.send({ jsonrpc: "2.0", ...message } as JSONRPCMessage);
    await settle();
  };
  const initialize = async () => {
    await send({ id: 0, method: "ui/initialize", params: INITIALIZE });
    await send({ method: "ui/notifications/initialized" });
  };
  return { view, posted, handlers, send, initialize };
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
    const { view, posted, send } = await bridge();
    await send({ id: 1, method: "ui/initialize", params: INITIALIZE });
    expect(posted[0]).toMatchObject({ id: 1, result: { protocolVersion: "2026-01-26", hostContext: { theme: "dark" } } });
    await send({ method: "ui/notifications/initialized" });
    expect(view.started).toBe(true);
  });

  it("tells the host when the view has started", async () => {
    const { handlers, initialize } = await bridge();
    await initialize();
    expect(handlers.started).toHaveBeenCalledOnce();
  });

  it("holds input and result until initialized, then sends input first", async () => {
    const { view, posted, initialize } = await bridge();
    view.setResult({ content: [{ type: "text", text: "done" }] });
    view.setInput({ q: 1 });
    await settle();
    expect(posted).toHaveLength(0);
    await initialize();
    expect(methods(posted)).toEqual(["ui/notifications/tool-input", "ui/notifications/tool-result"]);
  });

  it("never sends a result before its input", async () => {
    const { view, posted, initialize } = await bridge();
    await initialize();
    view.setResult({ content: [] });
    await settle();
    expect(methods(posted)).toEqual([]);
    view.setInput({});
    await settle();
    expect(methods(posted)).toEqual(["ui/notifications/tool-input", "ui/notifications/tool-result"]);
  });

  it("sends a cancellation in place of a result", async () => {
    const { view, posted, initialize } = await bridge();
    await initialize();
    view.setInput({});
    view.cancel("the seat stopped");
    await settle();
    expect(posted.at(-1)).toMatchObject({ method: "ui/notifications/tool-cancelled", params: { reason: "the seat stopped" } });
  });

  it("proxies a view's tool call and returns its result", async () => {
    const { posted, handlers, initialize, send } = await bridge();
    await initialize();
    await send({ id: "a", method: "tools/call", params: { name: "refresh", arguments: { n: 2 } } });
    expect(handlers.callTool).toHaveBeenCalledWith("refresh", { n: 2 });
    expect(posted.at(-1)).toMatchObject({ id: "a", result: { content: [{ text: "ok" }] } });
  });

  it("proxies a resource read", async () => {
    const { posted, handlers, initialize, send } = await bridge({ readResource: vi.fn(async () => ({ contents: [{ uri: "ui://x", text: "hi" }] })) });
    await initialize();
    await send({ id: "r", method: "resources/read", params: { uri: "ui://x" } });
    expect(handlers.readResource).toHaveBeenCalledWith("ui://x");
    expect(posted.at(-1)).toMatchObject({ id: "r", result: { contents: [{ text: "hi" }] } });
  });

  it("turns a failed call into a JSON-RPC error", async () => {
    const { posted, initialize, send } = await bridge({ callTool: async () => Promise.reject(new Error("the daemon refused")) });
    await initialize();
    await send({ id: 2, method: "tools/call", params: { name: "x" } });
    expect(posted.at(-1)).toMatchObject({ id: 2, error: { message: "the daemon refused" } });
  });

  it("opens web links, and refuses links that are not web links", async () => {
    const { posted, handlers, initialize, send } = await bridge();
    await initialize();
    await send({ id: 3, method: "ui/open-link", params: { url: "https://example.com/a" } });
    await send({ id: 4, method: "ui/open-link", params: { url: "javascript:alert(1)" } });
    expect(handlers.openLink).toHaveBeenCalledExactlyOnceWith("https://example.com/a");
    expect(posted.find((m) => m.id === 3)).toMatchObject({ result: {} });
    expect(posted.find((m) => m.id === 4)).toMatchObject({ error: { code: -32602 } });
  });

  it("refuses methods it does not serve", async () => {
    const { posted, initialize, send } = await bridge();
    await initialize();
    await send({ id: 5, method: "ui/message", params: { role: "user", content: [{ type: "text", text: "hi" }] } });
    expect(posted.at(-1)).toMatchObject({ id: 5, error: { code: -32601 } });
  });

  it("answers a display-mode request with the mode shown, and tells the view when the host changes it", async () => {
    const { view, posted, handlers, initialize, send } = await bridge();
    await initialize();
    await send({ id: 6, method: "ui/request-display-mode", params: { mode: "fullscreen" } });
    expect(handlers.displayMode).toHaveBeenCalledWith("fullscreen");
    expect(posted.at(-1)).toMatchObject({ id: 6, result: { mode: "fullscreen" } });
    view.updateContext({ displayMode: "inline" });
    await settle();
    expect(posted.at(-1)).toMatchObject({ method: "ui/notifications/host-context-changed", params: { displayMode: "inline" } });
  });

  it("sends a context change made before the handshake once it finishes", async () => {
    const { view, posted, send } = await bridge();
    await send({ id: 1, method: "ui/initialize", params: INITIALIZE });
    view.updateContext({ displayMode: "fullscreen" });
    await settle();
    expect(methods(posted)).toEqual([]);
    await send({ method: "ui/notifications/initialized" });
    expect(posted.at(-1)).toMatchObject({ method: "ui/notifications/host-context-changed", params: { displayMode: "fullscreen" } });
  });

  it("passes size changes on", async () => {
    const { handlers, initialize, send } = await bridge();
    await initialize();
    await send({ method: "ui/notifications/size-changed", params: { width: 300, height: 240 } });
    expect(handlers.resize).toHaveBeenCalledWith(240);
  });

  it("resolves teardown on the view's reply, and does not wait on a view that never started", async () => {
    const idle = await bridge();
    await expect(idle.view.teardown(10_000)).resolves.toBeUndefined();
    const { view, posted, initialize, send } = await bridge();
    await initialize();
    const done = view.teardown(10_000);
    await settle();
    const request = posted.find((message) => message.method === "ui/resource-teardown")!;
    await send({ id: request.id as number, result: {} });
    await expect(done).resolves.toBeUndefined();
  });

  it("gives up on a view that ignores teardown", async () => {
    const { view, initialize } = await bridge();
    await initialize();
    await expect(view.teardown(20)).resolves.toBeUndefined();
  });
});
