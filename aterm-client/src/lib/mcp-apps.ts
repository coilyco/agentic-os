// The host side of an MCP Apps view: the JSON-RPC a view speaks over
// postMessage. Spec: modelcontextprotocol/ext-apps, specification/2026-01-26.
// Why one sandboxed iframe, not the spec's double iframe: see the architecture notes.

export const PROTOCOL_VERSION = "2026-01-26";

export interface ViewCsp {
  connectDomains?: string[];
  resourceDomains?: string[];
  frameDomains?: string[];
  baseUriDomains?: string[];
}

/** One tool result that carried a `ui://` view, as the client holds it. */
export interface View {
  id: string;
  session: string;
  server: string;
  tool: string;
  resourceUri: string;
  html: string;
  csp?: ViewCsp;
  prefersBorder?: boolean;
  toolInput: Record<string, unknown>;
  toolResult?: ToolResult;
  cancelled?: string;
}

export interface ToolResult {
  content?: { type: string; text?: string }[];
  structuredContent?: unknown;
  isError?: boolean;
  _meta?: Record<string, unknown>;
}

export type DisplayMode = "inline" | "fullscreen";

export interface HostContext {
  theme: "light" | "dark";
  displayMode: DisplayMode;
  availableDisplayModes: DisplayMode[];
  platform: "web" | "mobile";
  locale: string;
  timeZone: string;
  containerDimensions: { maxHeight: number; width: number };
  styles?: { variables: Record<string, string> };
}

export interface ViewHandlers {
  callTool(name: string, args: Record<string, unknown>): Promise<unknown>;
  readResource(uri: string): Promise<unknown>;
  openLink(url: string): void;
  resize(height: number): void;
  displayMode(mode: DisplayMode): DisplayMode;
}

const sources = (domains: string[] | undefined): string => (domains?.length ? ` ${domains.join(" ")}` : "");

/** The spec's policy from `_meta.ui.csp`: nothing leaves the view undeclared. */
export function cspFor(csp: ViewCsp = {}): string {
  return [
    "default-src 'none'",
    `script-src 'unsafe-inline'${sources(csp.resourceDomains)}`,
    `style-src 'unsafe-inline'${sources(csp.resourceDomains)}`,
    `img-src data: blob:${sources(csp.resourceDomains)}`,
    `font-src data:${sources(csp.resourceDomains)}`,
    `media-src data: blob:${sources(csp.resourceDomains)}`,
    `connect-src${sources(csp.connectDomains) || " 'none'"}`,
    `frame-src${sources(csp.frameDomains) || " 'none'"}`,
    `base-uri${sources(csp.baseUriDomains) || " 'none'"}`,
    "object-src 'none'",
    "form-action 'none'",
  ].join("; ");
}

const escapeAttribute = (text: string): string => text.replace(/&/g, "&amp;").replace(/"/g, "&quot;");

/** The view's HTML with its policy pinned first in <head>, ahead of anything it loads. */
export function viewDocument(html: string, csp?: ViewCsp): string {
  const meta = `<meta http-equiv="Content-Security-Policy" content="${escapeAttribute(cspFor(csp))}">`;
  const head = /<head[^>]*>/i.exec(html);
  if (head) return html.slice(0, head.index + head[0].length) + meta + html.slice(head.index + head[0].length);
  const root = /<html[^>]*>/i.exec(html);
  if (root) return html.slice(0, root.index + root[0].length) + `<head>${meta}</head>` + html.slice(root.index + root[0].length);
  return `<!doctype html><html><head>${meta}</head><body>${html}</body></html>`;
}

/** The result's text blocks, or a line saying there were none. */
export function resultText(result: ToolResult | undefined): string {
  const text = (result?.content ?? []).filter((block) => block.type === "text" && block.text).map((block) => block.text);
  return text.length ? text.join("\n\n") : "The tool returned no text.";
}

interface Rpc {
  jsonrpc?: string;
  id?: string | number;
  method?: string;
  params?: Record<string, unknown>;
  result?: unknown;
  error?: unknown;
}

const METHOD_NOT_FOUND = -32601;
const INVALID_PARAMS = -32602;
const SERVER_ERROR = -32000;

// Speaks for the host to one view. Input and result wait for `initialized`,
// and input always goes first, as the spec orders them.
export class ViewBridge {
  private initialized = false;
  private input: Record<string, unknown> | null = null;
  private result: ToolResult | null = null;
  private cancelledReason: string | null = null;
  private sent = { input: false, result: false };
  private nextId = 0;
  private pending = new Map<string | number, (value: unknown) => void>();
  /** Set once the view asks to start, telling a live view from a stuck one. */
  started = false;

  constructor(
    private readonly post: (message: Rpc) => void,
    private readonly handlers: ViewHandlers,
    private context: HostContext,
  ) {}

  setInput(args: Record<string, unknown>): void {
    this.input = args;
    this.flush();
  }

  setResult(result: ToolResult): void {
    this.result = result;
    this.flush();
  }

  cancel(reason: string): void {
    this.cancelledReason = reason;
    this.flush();
  }

  updateContext(patch: Partial<HostContext>): void {
    this.context = { ...this.context, ...patch };
    if (this.initialized) this.notify("ui/notifications/host-context-changed", patch);
  }

  /** Asks the view to let go first. Resolves on its reply or after a beat. */
  teardown(reason: string, waitMs = 500): Promise<void> {
    if (!this.started) return Promise.resolve();
    const id = `host-${++this.nextId}`;
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        resolve();
      }, waitMs);
      this.pending.set(id, () => {
        clearTimeout(timer);
        resolve();
      });
      this.post({ jsonrpc: "2.0", id, method: "ui/resource-teardown", params: { reason } });
    });
  }

  receive(data: unknown): void {
    if (!data || typeof data !== "object") return;
    const message = data as Rpc;
    if (message.jsonrpc !== "2.0") return;
    if (message.method === undefined) {
      if (message.id !== undefined) this.pending.get(message.id)?.(message.result);
      if (message.id !== undefined) this.pending.delete(message.id);
      return;
    }
    if (message.id === undefined) this.onNotification(message.method, message.params ?? {});
    else void this.onRequest(message.id, message.method, message.params ?? {});
  }

  private onNotification(method: string, params: Record<string, unknown>): void {
    if (method === "ui/notifications/initialized") {
      this.initialized = true;
      this.flush();
    } else if (method === "ui/notifications/size-changed" && typeof params.height === "number") {
      this.handlers.resize(params.height);
    }
  }

  private async onRequest(id: string | number, method: string, params: Record<string, unknown>): Promise<void> {
    try {
      this.reply(id, await this.answer(method, params));
    } catch (failure) {
      const error = failure as { code?: number; message?: string };
      this.post({ jsonrpc: "2.0", id, error: { code: error.code ?? SERVER_ERROR, message: error.message ?? String(failure) } });
    }
  }

  private async answer(method: string, params: Record<string, unknown>): Promise<unknown> {
    switch (method) {
      case "ui/initialize":
        this.started = true;
        return {
          protocolVersion: PROTOCOL_VERSION,
          hostInfo: { name: "aterm", version: "0" },
          hostCapabilities: { openLinks: {}, serverTools: {}, serverResources: {} },
          hostContext: this.context,
        };
      case "tools/call":
        if (typeof params.name !== "string") throw { code: INVALID_PARAMS, message: "tools/call needs a tool name." };
        return this.handlers.callTool(params.name, (params.arguments as Record<string, unknown>) ?? {});
      case "resources/read":
        if (typeof params.uri !== "string") throw { code: INVALID_PARAMS, message: "resources/read needs a uri." };
        return this.handlers.readResource(params.uri);
      case "ui/open-link": {
        const url = typeof params.url === "string" ? params.url : "";
        if (!/^https?:\/\//i.test(url)) throw { code: INVALID_PARAMS, message: "Only http and https links open." };
        this.handlers.openLink(url);
        return {};
      }
      case "ui/request-display-mode": {
        const asked = params.mode === "fullscreen" ? "fullscreen" : "inline";
        const mode = this.handlers.displayMode(asked);
        this.context = { ...this.context, displayMode: mode };
        return { mode };
      }
      case "ping":
        return {};
      default:
        throw { code: METHOD_NOT_FOUND, message: `This host does not answer ${method}.` };
    }
  }

  private flush(): void {
    if (!this.initialized) return;
    if (this.input && !this.sent.input) {
      this.sent.input = true;
      this.notify("ui/notifications/tool-input", { arguments: this.input });
    }
    if (!this.sent.input || this.sent.result) return;
    if (this.result) {
      this.sent.result = true;
      this.notify("ui/notifications/tool-result", this.result as Record<string, unknown>);
    } else if (this.cancelledReason !== null) {
      this.sent.result = true;
      this.notify("ui/notifications/tool-cancelled", { reason: this.cancelledReason });
    }
  }

  private reply(id: string | number, result: unknown): void {
    this.post({ jsonrpc: "2.0", id, result });
  }

  private notify(method: string, params: Record<string, unknown> | Partial<HostContext>): void {
    this.post({ jsonrpc: "2.0", method, params: params as Record<string, unknown> });
  }
}
