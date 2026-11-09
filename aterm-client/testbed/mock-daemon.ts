// A stand-in aterm daemon: the built client at /, aterm.daemon.v1 on a websocket there.
// Replays scenes, records input. Walkthrough: the aterm-client skill's testbed reference.
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { readFileSync, existsSync, statSync } from "node:fs";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { WebSocketServer, type WebSocket } from "ws";
import type { Scene } from "./scenes.ts";
import { sceneFor } from "./scenes.ts";

export const FORMAT = "aterm.daemon.v1";
const HERE = fileURLToPath(new URL(".", import.meta.url));
// ATERM_TESTBED_DIST points the bed at another build of the client.
const DIST = process.env.ATERM_TESTBED_DIST ?? join(HERE, "..", "dist");
const ROSTER = join(HERE, "..", "src", "lib", "fixtures", "roster.json");
/** What the real daemon offers on a loopback page, so the client takes its real paths. */
const FEATURES = ["pty-size", "terminals", "terminal-label", "browser", "open-url", "last-reply"];

const TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".woff2": "font/woff2",
  ".woff": "font/woff",
  ".webmanifest": "application/manifest+json",
};

/** A session as the daemon lists it, plus the scene that draws its screen. */
export interface SessionSpec {
  name: string;
  role: string;
  identity: string;
  seat: string;
  ready: boolean;
  bracketed_paste: boolean;
  kai_drafting: boolean;
  pending: number;
  degraded?: string[];
  context?: { tokens: number; window?: number; source: string };
  last_reply?: string;
  /** What a `last_reply` returns: the whole text, its age, and whether it was cut. */
  last_reply_text?: string;
  last_reply_age_seconds?: number;
  last_reply_truncated?: boolean;
  /** Seconds since the seat last wrote, as the daemon's session view carries it. */
  quiet_seconds?: number;
  /** The seat's "what I'm doing" line and how long ago it set it, in seconds. */
  activity?: string;
  activity_age_seconds?: number;
  /** Which scene draws it: a name from scenes.ts. */
  scene: string;
}

interface Live {
  spec: SessionSpec;
  scene: Scene;
  /** The box each attached socket asked for. The daemon sizes the PTY to it. */
  boxes: Map<WebSocket, { rows: number; cols: number }>;
  /** Columns a wider client holds the PTY at, by the daemon's largest-client rule. */
  wideCols?: number;
}

export interface Mock {
  /** http origin. The page and its websocket both live here. */
  url: string;
  /** Every `input` the client typed into a seat, decoded, in order. */
  typed(session: string): string[];
  /** The box the client last asked a seat for: rows and columns, as the PTY is sized. */
  boxOf(session: string): { rows: number; cols: number } | undefined;
  /** Milliseconds since the client last sent a frame. Large once a resize storm ends. */
  idleMs(): number;
  /** Every `answer` and `cancel_ask` the client sent. */
  answers: { ask_id: string; picks?: number[]; text?: string; cancelled?: boolean }[];
  launches: { role: string; seat: string }[];
  /** Every `browser_*` frame and `subscribe` the client sent about a page, in order. */
  browserCalls: { type: string; session?: string; take?: boolean; url?: string; channel?: string }[];
  /** The scene a seat runs, to read what a native dialog received. */
  sceneOf(session: string): Scene | undefined;
  /** Size a seat's PTY to at least this many columns, as a wider client does. */
  holdWide(session: string, cols: number): void;
  /** Replace a seat's scene and redraw it on every client attached. */
  setScene(session: string, scene: Scene): void;
  /** Give a seat a finished reply: the first line rides the view, the text a request. */
  setLastReply(session: string, reply: { text: string; ageSeconds?: number; truncated?: boolean }): void;
  /** Send one daemon frame to every client, as the daemon would push it. */
  push(frame: Record<string, unknown>): void;
  /** Add a seat to the roster list, as a launch does. */
  addSession(spec: SessionSpec): void;
  /** Close the websocket from the daemon's side, as a daemon restart does. */
  dropClients(): void;
  /** Open sockets go silent without closing, as a slept phone's link does.
   * Later dials are unaffected. */
  silenceClients(): void;
  /** `refuse` fails a new dial at once, `hang` never answers it, `ok` serves it.
   * Open sockets are left alone. */
  dials(mode: "ok" | "refuse" | "hang"): void;
  /** Refuses the next `count` dials, then `dials` rules: a cold start that fails once. */
  refuseNext(count: number): void;
  /** Dials that reached the daemon, answered or not, since the mock started. */
  dialCount(): number;
  /** Frame types the daemon received, in order, to see that a probe went out. */
  heard: string[];
  /** Every proof the client presented in a `resume` frame, in order. */
  resumes: string[];
  /** The resume window passes: every proof the daemon issued stops holding. */
  endResumeWindow(): void;
  stop(): Promise<void>;
}

function encode(text: string): string {
  return Buffer.from(text, "utf8").toString("base64");
}

/** A full redraw: clear, home, rows joined. A harness does this on resize. */
export function paint(lines: string[]): string {
  return `\x1b[2J\x1b[H${lines.join("\r\n")}`;
}

export function loadSessions(): SessionSpec[] {
  return JSON.parse(readFileSync(new URL("./fixtures/sessions.json", import.meta.url), "utf8")) as SessionSpec[];
}

/** `lock`: a device kept read only until a key unlocks it (COI-3305).
 * `images`: a host that takes an image, so the composer shows Add image. */
export type MockOptions = { sessions?: SessionSpec[]; port?: number; lock?: boolean; images?: boolean; launchDelayMs?: number };
export async function startMock(options: MockOptions = {}): Promise<Mock> {
  if (!existsSync(join(DIST, "index.html"))) {
    throw new Error("The mock daemon serves aterm-client/dist, which is not built. Run `just aterm-client-testbed`, which builds it first.");
  }
  const roster = JSON.parse(readFileSync(ROSTER, "utf8")) as unknown;
  const live = new Map<string, Live>();
  const typedBy = new Map<string, string[]>();
  const clients = new Set<WebSocket>();
  // Sockets that went silent: they stay open but speak and hear nothing.
  const silent = new WeakSet<WebSocket>();
  let dialMode: "ok" | "refuse" | "hang" = "ok";
  let dials = 0;
  let refusals = 0;
  const heard: string[] = [];
  let lastFrameAt = Date.now();
  const lastBox = new Map<string, { rows: number; cols: number }>();
  const mock = { answers: [] as Mock["answers"], launches: [] as Mock["launches"], browserCalls: [] as Mock["browserCalls"], resumes: [] as string[] };
  // A seat's browser: the agent drives until a screen takes control.
  // `client` is the name each connection was given.
  const pages = new Map<string, { holder: WebSocket | null; url: string }>();
  const names = new WeakMap<WebSocket, string>();
  const browserState = (session: string, socket: WebSocket, id?: unknown): Record<string, unknown> => {
    const page = pages.get(session) ?? { holder: null, url: "about:blank" };
    return { type: "browser_state", id, session, state: "live", driver: page.holder ? "person" : "agent", ...(page.holder ? { holder: names.get(page.holder) } : {}), url: page.url, title: "", client: names.get(socket) };
  };
  const tellWatchers = (session: string): void => {
    for (const socket of clients) send(socket, browserState(session, socket));
  };
  // Proofs the locked daemon honours. One use each, as the contract says.
  const proofs = new Set<string>();
  let proofCount = 0;
  const mintProof = (): string => {
    const proof = `proof-${++proofCount}`;
    proofs.add(proof);
    return proof;
  };
  const unlockedFeatures = ["device-key", "resume"];

  const add = (spec: SessionSpec): void => {
    live.set(spec.name, { spec, scene: sceneFor(spec.scene, spec), boxes: new Map() });
  };
  for (const spec of options.sessions ?? loadSessions()) add(spec);

  const send = (socket: WebSocket, frame: Record<string, unknown>): void => {
    if (socket.readyState === socket.OPEN && !silent.has(socket)) socket.send(JSON.stringify(frame));
  };
  const broadcast = (frame: Record<string, unknown>): void => {
    for (const socket of clients) send(socket, frame);
  };
  const sessionsFrame = (): Record<string, unknown> => ({ type: "sessions", sessions: [...live.values()].map(({ spec }) => withoutScene(spec)), terminals: [] });

  /** Draw a seat at the size its client asked for. A client of this one's size only. */
  const draw = (seat: Live, socket: WebSocket, attach = false): void => {
    const own = seat.boxes.get(socket);
    if (!own) return;
    const box = { rows: own.rows, cols: Math.max(own.cols, seat.wideCols ?? 0) };
    send(socket, { type: "size", session: seat.spec.name, rows: box.rows, cols: box.cols });
    const text = attach && seat.scene.history ? seat.scene.history(box.cols, box.rows) : paint(seat.scene.render(box.cols, box.rows));
    send(socket, { type: "output", session: seat.spec.name, data: encode(text) });
  };
  const redraw = (seat: Live): void => {
    for (const socket of seat.boxes.keys()) draw(seat, socket);
  };

  const hung = new Set<import("node:stream").Duplex>();
  const wss = new WebSocketServer({ noServer: true });
  wss.on("connection", (socket) => {
    clients.add(socket);
    names.set(socket, `screen-${clients.size}`);
    socket.on("close", () => {
      clients.delete(socket);
      for (const seat of live.values()) seat.boxes.delete(socket);
    });
    socket.on("message", (raw) => {
      if (silent.has(socket)) return;
      lastFrameAt = Date.now();
      const frame = JSON.parse(String(raw)) as Record<string, unknown> & { type: string; id?: string; session?: string };
      heard.push(frame.type);
      const seat = frame.session ? live.get(frame.session) : undefined;
      switch (frame.type) {
        case "hello":
          if (options.lock) {
            send(socket, { type: "welcome", format: FORMAT, version: "testbed", features: [...FEATURES, ...unlockedFeatures], typing: { allowed: false, reason: "passkey_required", device_key: "enrolled" } });
          } else {
            send(socket, { type: "welcome", format: FORMAT, version: "testbed", features: options.images ? [...FEATURES, "image-upload"] : FEATURES, typing: { allowed: true } });
          }
          return;
        case "device_assert_begin":
          send(socket, { type: "device_assert_challenge", id: frame.id, challenge: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" });
          return;
        case "device_assert_finish":
          send(socket, { type: "device_asserted", id: frame.id, resume: mintProof(), expires_in: 600 });
          send(socket, { type: "typing", typing: { allowed: true } });
          return;
        case "resume": {
          const presented = String(frame.resume);
          mock.resumes.push(presented);
          if (!proofs.delete(presented)) {
            send(socket, { type: "error", id: frame.id, code: 6, error: "the unlock ended", reason: "resume_ended" });
            return;
          }
          send(socket, { type: "resumed", id: frame.id, resume: mintProof(), expires_in: 600 });
          send(socket, { type: "typing", typing: { allowed: true } });
          return;
        }
        case "subscribe":
          if (frame.channel === "sessions") send(socket, sessionsFrame());
          if (frame.channel === "open_url") {
            mock.browserCalls.push({ type: "subscribe", channel: "open_url" });
            send(socket, { type: "subscribed", id: frame.id, channel: "open_url" });
          }
          return;
        case "browser_watch":
          mock.browserCalls.push({ type: "browser_watch", session: String(frame.session) });
          if (!pages.has(String(frame.session))) pages.set(String(frame.session), { holder: null, url: "about:blank" });
          send(socket, browserState(String(frame.session), socket, frame.id));
          return;
        case "browser_unwatch":
          mock.browserCalls.push({ type: "browser_unwatch", session: String(frame.session) });
          return;
        case "browser_control": {
          const session = String(frame.session);
          mock.browserCalls.push({ type: "browser_control", session, take: frame.take === true });
          const page = pages.get(session);
          if (page) page.holder = frame.take === true ? socket : null;
          tellWatchers(session);
          return;
        }
        case "browser_navigate": {
          const session = String(frame.session);
          mock.browserCalls.push({ type: "browser_navigate", session, url: String(frame.url) });
          const page = pages.get(session);
          // The daemon refuses a drive from a screen that does not hold control.
          if (page?.holder === socket) {
            page.url = String(frame.url);
            tellWatchers(session);
          }
          return;
        }
        case "list":
          send(socket, { ...sessionsFrame(), id: frame.id });
          return;
        case "last_reply": {
          const spec = live.get(String(frame.target))?.spec;
          const reply = spec?.last_reply_text === undefined ? { session: String(frame.target), text: "" } : { session: spec.name, text: spec.last_reply_text, truncated: spec.last_reply_truncated === true, at: new Date(Date.now() - (spec.last_reply_age_seconds ?? 0) * 1000).toISOString() };
          send(socket, { type: "last_reply", id: frame.id, session: reply.session, reply });
          return;
        }
        case "roster":
          send(socket, { type: "roster", id: frame.id, roster });
          return;
        case "attach":
          if (!seat) return;
          if (typeof frame.rows === "number" && typeof frame.cols === "number") {
            seat.boxes.set(socket, { rows: frame.rows, cols: frame.cols });
            lastBox.set(seat.spec.name, { rows: frame.rows, cols: frame.cols });
          }
          // A watch without a box (the activity monitor) takes no replay.
          if (frame.replay) draw(seat, socket, true);
          return;
        case "detach":
          seat?.boxes.delete(socket);
          return;
        case "resize":
          if (seat && typeof frame.rows === "number" && typeof frame.cols === "number") {
            seat.boxes.set(socket, { rows: frame.rows, cols: frame.cols });
            lastBox.set(seat.spec.name, { rows: frame.rows, cols: frame.cols });
            draw(seat, socket);
          }
          return;
        case "input": {
          if (!seat) return;
          const text = Buffer.from(String(frame.data ?? ""), "base64").toString("utf8");
          typedBy.set(seat.spec.name, [...(typedBy.get(seat.spec.name) ?? []), text]);
          if (seat.scene.input?.(text)) redraw(seat);
          return;
        }
        case "launch": {
          const role = String(frame.role);
          const seatKey = String(frame.seat ?? "claude");
          mock.launches.push({ role, seat: seatKey });
          send(socket, { type: "launched", id: frame.id, role });
          // The window opens on the host, then the seat shows up in the next listing.
          const identity = "New-Seat";
          setTimeout(() => {
            add({ name: `${role}-new-seat`, role, identity, seat: seatKey, ready: true, bracketed_paste: true, kai_drafting: false, pending: 0, scene: "claude-idle" });
            broadcast(sessionsFrame());
          }, options.launchDelayMs ?? 400);
          return;
        }
        case "answer":
          mock.answers.push({ ask_id: String(frame.ask_id), picks: frame.picks as number[], ...(frame.text ? { text: String(frame.text) } : {}) });
          broadcast({ type: "asked", ask_id: frame.ask_id, state: "answered" });
          return;
        case "cancel_ask":
          mock.answers.push({ ask_id: String(frame.ask_id), cancelled: true });
          broadcast({ type: "asked", ask_id: frame.ask_id, state: "cancelled" });
          return;
        default:
          // A frame this daemon does not model. Say so in its own words, never silence.
          send(socket, { type: "error", id: frame.id, error: `the test bed daemon does not model ${frame.type}` });
      }
    });
  });

  const server: Server = createServer((request, response) => serve(request, response));
  server.on("upgrade", (request, socket, head) => {
    dials += 1;
    // A stopped daemon refuses at once. A dead path takes the SYN and answers nothing.
    if (dialMode === "refuse" || refusals > 0) {
      refusals = Math.max(0, refusals - 1);
      return void socket.destroy();
    }
    if (dialMode === "hang") {
      hung.add(socket);
      return;
    }
    wss.handleUpgrade(request, socket, head, (ws) => wss.emit("connection", ws, request));
  });
  await new Promise<void>((resolve) => server.listen(options.port ?? 0, "127.0.0.1", resolve));
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;

  return {
    url: `http://127.0.0.1:${port}`,
    typed: (session) => typedBy.get(session) ?? [],
    boxOf: (session) => lastBox.get(session),
    idleMs: () => Date.now() - lastFrameAt,
    answers: mock.answers,
    launches: mock.launches,
    browserCalls: mock.browserCalls,
    sceneOf: (session) => live.get(session)?.scene,
    holdWide(session, cols) {
      const seat = live.get(session);
      if (!seat) throw new Error(`no seat named ${session}`);
      seat.wideCols = cols;
      redraw(seat);
    },
    setScene(session, scene) {
      const seat = live.get(session);
      if (!seat) throw new Error(`no seat named ${session}`);
      seat.scene = scene;
      redraw(seat);
    },
    setLastReply(session, reply) {
      const seat = live.get(session);
      if (!seat) throw new Error(`no seat named ${session}`);
      Object.assign(seat.spec, { last_reply: reply.text.split("\n")[0], last_reply_text: reply.text, last_reply_age_seconds: reply.ageSeconds ?? 0, last_reply_truncated: reply.truncated === true });
      broadcast(sessionsFrame());
    },
    push: broadcast,
    addSession(spec) {
      add(spec);
      broadcast(sessionsFrame());
    },
    dropClients() {
      for (const socket of clients) socket.close();
    },
    silenceClients() {
      for (const socket of clients) silent.add(socket);
    },
    dials(mode) {
      dialMode = mode;
    },
    refuseNext(count) {
      refusals = count;
    },
    dialCount: () => dials,
    heard,
    resumes: mock.resumes,
    endResumeWindow: () => proofs.clear(),
    async stop() {
      for (const socket of clients) socket.terminate();
      for (const socket of hung) socket.destroy();
      wss.close();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    },
  };
}

function withoutScene(spec: SessionSpec): Record<string, unknown> {
  const { scene: _scene, activity_age_seconds: age, last_reply_text: _text, last_reply_age_seconds: replyAge, last_reply_truncated: _cut, ...view } = spec;
  return {
    ...view,
    ...(spec.activity && age !== undefined ? { activity_at: new Date(Date.now() - age * 1000).toISOString() } : {}),
    ...(spec.last_reply && replyAge !== undefined ? { last_reply_at: new Date(Date.now() - replyAge * 1000).toISOString() } : {}),
  };
}

function serve(request: IncomingMessage, response: ServerResponse): void {
  const path = normalize(decodeURIComponent(new URL(request.url ?? "/", "http://x").pathname)).replace(/^(\.\.[/\\])+/, "");
  let file = join(DIST, path);
  if (!file.startsWith(DIST) || !existsSync(file) || statSync(file).isDirectory()) file = join(DIST, "index.html");
  response.writeHead(200, { "content-type": TYPES[extname(file)] ?? "application/octet-stream", "cache-control": "no-store" });
  response.end(readFileSync(file));
}
