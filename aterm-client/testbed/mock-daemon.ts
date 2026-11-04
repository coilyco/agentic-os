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
const FEATURES = ["pty-size", "terminals", "terminal-label"];

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
  /** Which scene draws it: a name from scenes.ts. */
  scene: string;
}

interface Live {
  spec: SessionSpec;
  scene: Scene;
  /** The box each attached socket asked for. The daemon sizes the PTY to it. */
  boxes: Map<WebSocket, { rows: number; cols: number }>;
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
  /** The scene a seat runs, to read what a native dialog received. */
  sceneOf(session: string): Scene | undefined;
  /** Replace a seat's scene and redraw it on every client attached. */
  setScene(session: string, scene: Scene): void;
  /** Send one daemon frame to every client, as the daemon would push it. */
  push(frame: Record<string, unknown>): void;
  /** Add a seat to the roster list, as a launch does. */
  addSession(spec: SessionSpec): void;
  /** Close the websocket from the daemon's side, as a daemon restart does. */
  dropClients(): void;
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

export async function startMock(options: { sessions?: SessionSpec[]; port?: number } = {}): Promise<Mock> {
  if (!existsSync(join(DIST, "index.html"))) {
    throw new Error("The mock daemon serves aterm-client/dist, which is not built. Run `just aterm-client-testbed`, which builds it first.");
  }
  const roster = JSON.parse(readFileSync(ROSTER, "utf8")) as unknown;
  const live = new Map<string, Live>();
  const typedBy = new Map<string, string[]>();
  const clients = new Set<WebSocket>();
  let lastFrameAt = Date.now();
  const lastBox = new Map<string, { rows: number; cols: number }>();
  const mock = { answers: [] as Mock["answers"], launches: [] as Mock["launches"] };

  const add = (spec: SessionSpec): void => {
    live.set(spec.name, { spec, scene: sceneFor(spec.scene, spec), boxes: new Map() });
  };
  for (const spec of options.sessions ?? loadSessions()) add(spec);

  const send = (socket: WebSocket, frame: Record<string, unknown>): void => {
    if (socket.readyState === socket.OPEN) socket.send(JSON.stringify(frame));
  };
  const broadcast = (frame: Record<string, unknown>): void => {
    for (const socket of clients) send(socket, frame);
  };
  const sessionsFrame = (): Record<string, unknown> => ({ type: "sessions", sessions: [...live.values()].map(({ spec }) => withoutScene(spec)), terminals: [] });

  /** Draw a seat at the size its client asked for. A client of this one's size only. */
  const draw = (seat: Live, socket: WebSocket): void => {
    const box = seat.boxes.get(socket);
    if (!box) return;
    send(socket, { type: "size", session: seat.spec.name, rows: box.rows, cols: box.cols });
    send(socket, { type: "output", session: seat.spec.name, data: encode(paint(seat.scene.render(box.cols, box.rows))) });
  };
  const redraw = (seat: Live): void => {
    for (const socket of seat.boxes.keys()) draw(seat, socket);
  };

  const wss = new WebSocketServer({ noServer: true });
  wss.on("connection", (socket) => {
    clients.add(socket);
    socket.on("close", () => {
      clients.delete(socket);
      for (const seat of live.values()) seat.boxes.delete(socket);
    });
    socket.on("message", (raw) => {
      lastFrameAt = Date.now();
      const frame = JSON.parse(String(raw)) as Record<string, unknown> & { type: string; id?: string; session?: string };
      const seat = frame.session ? live.get(frame.session) : undefined;
      switch (frame.type) {
        case "hello":
          send(socket, { type: "welcome", format: FORMAT, version: "testbed", features: FEATURES, typing: { allowed: true } });
          return;
        case "subscribe":
          if (frame.channel === "sessions") send(socket, sessionsFrame());
          return;
        case "list":
          send(socket, { ...sessionsFrame(), id: frame.id });
          return;
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
          if (frame.replay) draw(seat, socket);
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
          }, 400);
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
  server.on("upgrade", (request, socket, head) => wss.handleUpgrade(request, socket, head, (ws) => wss.emit("connection", ws, request)));
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
    sceneOf: (session) => live.get(session)?.scene,
    setScene(session, scene) {
      const seat = live.get(session);
      if (!seat) throw new Error(`no seat named ${session}`);
      seat.scene = scene;
      redraw(seat);
    },
    push: broadcast,
    addSession(spec) {
      add(spec);
      broadcast(sessionsFrame());
    },
    dropClients() {
      for (const socket of clients) socket.close();
    },
    async stop() {
      for (const socket of clients) socket.terminate();
      wss.close();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    },
  };
}

function withoutScene(spec: SessionSpec): Omit<SessionSpec, "scene"> {
  const { scene: _scene, ...view } = spec;
  return view;
}

function serve(request: IncomingMessage, response: ServerResponse): void {
  const path = normalize(decodeURIComponent(new URL(request.url ?? "/", "http://x").pathname)).replace(/^(\.\.[/\\])+/, "");
  let file = join(DIST, path);
  if (!file.startsWith(DIST) || !existsSync(file) || statSync(file).isDirectory()) file = join(DIST, "index.html");
  response.writeHead(200, { "content-type": TYPES[extname(file)] ?? "application/octet-stream", "cache-control": "no-store" });
  response.end(readFileSync(file));
}
