// The host side of an MCP Apps view, spoken by ext-apps' AppBridge over postMessage.
// Spec: modelcontextprotocol/ext-apps, specification/2026-01-26.
// Why one sandboxed iframe, not the spec's double iframe: see the architecture notes.
import { ProtocolError, ProtocolErrorCode, type CallToolResult, type ReadResourceResult, type Transport } from "@modelcontextprotocol/client";
import { AppBridge, type McpUiHostContext } from "@modelcontextprotocol/ext-apps/app-bridge";

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
  /** The view finished its handshake. */
  started(): void;
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

// Speaks for the host to one view. Input and result wait for `initialized`,
// and input always goes first, as the spec orders them.
export class ViewBridge {
  private readonly bridge: AppBridge;
  private initialized = false;
  private input: Record<string, unknown> | null = null;
  private result: ToolResult | null = null;
  private cancelledReason: string | null = null;
  private sent = { input: false, result: false };
  /** Set once the view completes its handshake, telling a live view from a stuck one. */
  started = false;

  constructor(
    private readonly handlers: ViewHandlers,
    private context: HostContext,
  ) {
    // No MCP client: the daemon gateway is the server, so calls go through the handlers.
    this.bridge = new AppBridge(
      null,
      { name: "aterm", version: "0" },
      { openLinks: {}, serverTools: {}, serverResources: {} },
      { hostContext: this.context as McpUiHostContext },
    );
    this.bridge.oninitialized = () => {
      this.initialized = true;
      this.started = true;
      this.handlers.started();
      // The handshake carried the context as of then. Later changes go out now.
      this.bridge.setHostContext(this.context as McpUiHostContext);
      this.flush();
    };
    this.bridge.onsizechange = ({ height }) => {
      if (typeof height === "number") this.handlers.resize(height);
    };
    this.bridge.oncalltool = async ({ name, arguments: args }) => (await this.handlers.callTool(name, args ?? {})) as CallToolResult;
    this.bridge.onreadresource = async ({ uri }) => (await this.handlers.readResource(uri)) as ReadResourceResult;
    this.bridge.onopenlink = async ({ url }) => {
      if (!/^https?:\/\//i.test(url)) throw new ProtocolError(ProtocolErrorCode.InvalidParams, "Only http and https links open.");
      this.handlers.openLink(url);
      return {};
    };
    this.bridge.onrequestdisplaymode = async ({ mode }) => {
      const shown = this.handlers.displayMode(mode === "fullscreen" ? "fullscreen" : "inline");
      this.updateContext({ displayMode: shown });
      return { mode: shown };
    };
  }

  connect(transport: Transport): Promise<void> {
    return this.bridge.connect(transport);
  }

  /** Lets go of the transport's listener. Teardown first to give the view a say. */
  close(): Promise<void> {
    return this.bridge.close();
  }

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
    if (this.initialized) this.bridge.setHostContext(this.context as McpUiHostContext);
  }

  /** Asks the view to let go first. Resolves on its reply or after a beat. */
  async teardown(waitMs = 500): Promise<void> {
    if (!this.started) return;
    await this.bridge.teardownResource({}, { timeout: waitMs }).catch(() => undefined);
  }

  private flush(): void {
    if (!this.initialized) return;
    if (this.input && !this.sent.input) {
      this.sent.input = true;
      void this.bridge.sendToolInput({ arguments: this.input });
    }
    if (!this.sent.input || this.sent.result) return;
    if (this.result) {
      this.sent.result = true;
      void this.bridge.sendToolResult(this.result as CallToolResult);
    } else if (this.cancelledReason !== null) {
      this.sent.result = true;
      void this.bridge.sendToolCancelled({ reason: this.cancelledReason });
    }
  }
}
