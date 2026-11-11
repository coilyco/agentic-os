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
const FEATURES = ["pty-size", "terminals", "terminal-label", "browser", "open-url", "background", "close-unpushed", "reopen", "origin", "last-reply", "notify"];

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
  /** How long the daemon has seen the present state. */
  state_seconds?: number;
  /** The seat's "what I'm doing" line and how long ago it set it, in seconds. */
  activity?: string;
  activity_age_seconds?: number;
  /** The screen reading, the `ref` a close takes, and background tasks running. */
  state?: "starting" | "busy" | "idle" | "prompt";
  ref?: string;
  origin?: "window" | "headless";
  /** The daemon refuses an unforced close, as it does for commits on no remote. */
  unpushed?: boolean;
  background?: number;
  /** The daemon refuses a close with this text, as it does for a held draft. */
  refuse_close?: string;
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
  /** Rows a raw client caps the PTY at, however many rows another client asks for. */
  capRows?: number;
  /** Bytes sent to each socket so far, the stream offset when `offsets` is on. */
  sent: Map<WebSocket, number>;
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
  /** The `push_*` frames the client sent, in order. */
  pushCalls: { type: string; push?: unknown }[];
  /** Every `close` target the client sent, as `name@ref`. */
  closes: string[];
  /** Every `close` sent with `force`, as `name@ref`, and every `reopen` target. */
  forced: string[];
  reopens: string[];
  /** Every `browser_*` frame and `subscribe` the client sent about a page, in order. */
  browserCalls: { type: string; session?: string; take?: boolean; url?: string; channel?: string }[];
  /** The scene a seat runs, to read what a native dialog received. */
  sceneOf(session: string): Scene | undefined;
  /** Size a seat's PTY to at least this many columns, as a wider client does. */
  holdWide(session: string, cols: number): void;
  /** Hold a seat's PTY to at most this many rows, as a raw client's window does. */
  capRows(session: string, rows: number): void;
  /** Replace a seat's scene and redraw it on every client attached. */
  setScene(session: string, scene: Scene): void;
  /** Give a seat a finished reply: the first line rides the view, the text a request. */
  setLastReply(session: string, reply: { text: string; ageSeconds?: number; truncated?: boolean }): void;
  /** Send one daemon frame to every client, as the daemon would push it. */
  push(frame: Record<string, unknown>): void;
  /** A seat's page, as `aterm artifact add` registers it, replayed on subscribe. */
  addArtifact(session: string, artifact: Record<string, unknown>): void;
  /** The daemon closes a page, as a seat's remove or its end does. */
  closeArtifact(session: string, id: string): void;
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
  /** Each `image_upload` taken: the seat, and the bytes sent. */
  uploads: { session: string; bytes: Buffer }[];
  /** The daemon refuses every `image_upload` with this reason, until set back to null. */
  refuseUploads(reason: string | null): void;
  /** Each `feedback` taken: the note, the screen it names, and the screenshots' bytes. */
  feedbacks: { text: string; screen: Record<string, unknown>; images: Buffer[] }[];
  /** Every `feedback_image` and `feedback` frame heard, to count what a retry resent. */
  feedbackFrames: string[];
  /** The daemon refuses the next `feedback` frame with this reason, then serves again. */
  refuseFeedbackOnce(reason: string): void;
  /** Staged screenshot tokens stop holding, as ten minutes or a redial does. */
  expireFeedbackImages(): void;
  /** What a seat's transcript holds. The mock answers `transcript` from it. */
  setTranscript(session: string, items: unknown[]): void;
  /** Items written to a seat's transcript after the client's last read. */
  appendTranscript(session: string, items: unknown[]): void;
  /** The daemon refuses a seat's `transcript` with this reason, until set to null. */
  refuseTranscript(session: string, reason: string | null): void;
  /** Delays this seat's `transcript` answer by this many ms, 0 to stop. */
  holdTranscript(session: string, ms: number): void;
  /** Every `attach` the client sent, with the offset it named, in order. */
  attachCalls: { session: string; replay: boolean; offset?: number }[];
  /** Every `transcript` request the client sent, in order. */
  transcriptCalls: { session: string; since?: string; before?: string; limit?: number }[];
  /** A seat shares a file. The mock keeps its bytes and pushes the descriptor. */
  shareMedia(session: string, file: { name: string; mime: string; kind: "image" | "video" | "file"; bytes: Buffer; unreadable?: boolean }): string;
  /** The host drops a shared file, as it does when a seat's shelf is full. */
  closeMedia(session: string, id: string): void;
  /** Every `media_read` the client sent: the file and where in it. */
  mediaReads: { media: string; offset: number; length: number }[];
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
 * `images`: Add image shows. `webPush`: Notify when closed shows. */
export type MockOptions = { sessions?: SessionSpec[]; port?: number; lock?: boolean; images?: boolean; feedback?: boolean; version?: string; transcript?: boolean; media?: boolean; artifacts?: boolean; offsets?: boolean; webPush?: boolean; launchDelayMs?: number; pinnedRoles?: string[]; noBrowser?: boolean };
export async function startMock(options: MockOptions = {}): Promise<Mock> {
  if (!existsSync(join(DIST, "index.html"))) {
    throw new Error("The mock daemon serves aterm-client/dist, which is not built. Run `just aterm-client-testbed`, which builds it first.");
  }
  const roster = JSON.parse(readFileSync(ROSTER, "utf8")) as { roles: { slug: string; pinned_by_default?: boolean }[] };
  // Roles the roster marks pinned by default, as a daemon with the attribute would.
  for (const role of roster.roles) if (options.pinnedRoles?.includes(role.slug)) role.pinned_by_default = true;
  // A daemon with no browser to stream does not list the feature.
  const served = () => (options.noBrowser ? FEATURES.filter((feature) => feature !== "browser") : FEATURES);
  const live = new Map<string, Live>();
  const typedBy = new Map<string, string[]>();
  let uploadRefusal: string | null = null;
  let feedbackRefusal: string | null = null;
  const staged = new Map<string, Buffer>();
  let stagedCount = 0;
  const clients = new Set<WebSocket>();
  // Sockets that went silent: they stay open but speak and hear nothing.
  const silent = new WeakSet<WebSocket>();
  let dialMode: "ok" | "refuse" | "hang" = "ok";
  let dials = 0;
  let refusals = 0;
  const heard: string[] = [];
  let lastFrameAt = Date.now();
  const lastBox = new Map<string, { rows: number; cols: number }>();
  const closed = new Map<string, SessionSpec>();
  const artifactFrames = new Map<string, Record<string, unknown>>();
  const mock = { answers: [] as Mock["answers"], launches: [] as Mock["launches"], closes: [] as string[], forced: [] as string[], reopens: [] as string[], browserCalls: [] as Mock["browserCalls"], pushCalls: [] as Mock["pushCalls"], resumes: [] as string[], uploads: [] as Mock["uploads"], feedbacks: [] as Mock["feedbacks"], feedbackFrames: [] as string[], mediaReads: [] as Mock["mediaReads"], transcriptCalls: [] as Mock["transcriptCalls"], attachCalls: [] as Mock["attachCalls"] };
  const shared = new Map<string, { session: string; name: string; mime: string; kind: string; bytes: Buffer; at: number; unreadable?: boolean }>();
  const mediaFrame = (id: string) => {
    const file = shared.get(id)!;
    return { type: "media", session: file.session, media: { id, name: file.name, mime: file.mime, kind: file.kind, size: file.bytes.length, at: new Date(file.at).toISOString() } };
  };
  const transcripts = new Map<string, unknown[]>();
  const transcriptRefusals = new Map<string, string>();
  const transcriptHolds = new Map<string, number>();
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
  const pushFeature = options.webPush ? ["web-push"] : [];

  const add = (spec: SessionSpec): void => {
    live.set(spec.name, { spec, scene: sceneFor(spec.scene, spec), boxes: new Map(), sent: new Map() });
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
    const box = { rows: Math.min(own.rows, seat.capRows ?? own.rows), cols: Math.max(own.cols, seat.wideCols ?? 0) };
    send(socket, { type: "size", session: seat.spec.name, rows: box.rows, cols: box.cols });
    const text = attach && seat.scene.history ? seat.scene.history(box.cols, box.rows) : paint(seat.scene.render(box.cols, box.rows));
    const data = encode(text);
    // With `offsets` the stream is counted as the daemon counts it.
    const at = seat.sent.get(socket) ?? 0;
    seat.sent.set(socket, at + Buffer.from(data, "base64").length);
    send(socket, { type: "output", session: seat.spec.name, data, ...(options.offsets ? { offset: at } : {}) });
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
            send(socket, { type: "welcome", format: FORMAT, version: options.version ?? "testbed", features: [...served(), ...unlockedFeatures, ...pushFeature], typing: { allowed: false, reason: "passkey_required", device_key: "enrolled" } });
          } else {
            send(socket, { type: "welcome", format: FORMAT, version: options.version ?? "testbed", features: [...served(), ...(options.images ? ["image-upload"] : []), ...(options.feedback ? ["feedback"] : []), ...(options.transcript ? ["transcript"] : []), ...(options.media ? ["media"] : []), ...(options.artifacts ? ["artifacts"] : []), ...pushFeature], typing: { allowed: true } });
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
          if (frame.channel === "media") for (const id of shared.keys()) send(socket, mediaFrame(id));
          if (frame.channel === "views") for (const page of artifactFrames.values()) send(socket, page);
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
        case "push_key":
        case "push_subscribe":
        case "push_unsubscribe": {
          if (!options.webPush) return void send(socket, { type: "error", id: frame.id, error: `the test bed daemon does not model ${frame.type}` });
          mock.pushCalls.push({ type: frame.type, push: frame.push });
          const reply = { push_key: "push_key", push_subscribe: "push_subscribed", push_unsubscribe: "push_unsubscribed" }[frame.type];
          // A 65 byte uncompressed P-256 point, base64url, as the daemon's public key is.
          send(socket, { type: reply, id: frame.id, ...(frame.type === "push_key" ? { key: Buffer.alloc(65, 4).toString("base64url") } : {}) });
          return;
        }
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
        case "transcript": {
          const session = String(frame.session);
          const since = typeof frame.since === "string" ? frame.since : undefined;
          const before = typeof frame.before === "string" ? frame.before : undefined;
          const limit = typeof frame.limit === "number" ? frame.limit : undefined;
          mock.transcriptCalls.push({ session, ...(since ? { since } : {}), ...(before ? { before } : {}), ...(limit ? { limit } : {}) });
          const reason = transcriptRefusals.get(session);
          if (reason) {
            send(socket, { type: "error", id: frame.id, session, reason, error: `no transcript: ${reason}` });
            return;
          }
          const all = transcripts.get(session) ?? [];
          // A cursor is an index here. The client never reads one.
          const size = limit ?? 50;
          const to = before === undefined ? all.length : Number(before);
          const from = since !== undefined ? Number(since) : Math.max(0, to - size);
          if (!Number.isInteger(from) || !Number.isInteger(to)) {
            send(socket, { type: "error", id: frame.id, session, reason: "bad_cursor", error: "bad cursor" });
            return;
          }
          const answer = { type: "transcript", id: frame.id, session, reader: "claude", items: all.slice(from, to), cursor: String(all.length), ...(from > 0 ? { older: String(from) } : {}), more: from > 0, updated_at: new Date().toISOString() };
          const hold = transcriptHolds.get(session) ?? 0;
          if (hold > 0) setTimeout(() => send(socket, answer), hold);
          else send(socket, answer);
          return;
        }
        case "media_read": {
          const id = String(frame.media_id);
          const file = shared.get(id);
          const offset = Number(frame.offset ?? 0);
          const length = Number(frame.length ?? 0);
          mock.mediaReads.push({ media: id, offset, length });
          if (!file || file.unreadable) return void send(socket, { type: "error", id: frame.id, reason: "media_unknown", error: "no such file" });
          const slice = file.bytes.subarray(offset, offset + length);
          send(socket, { type: "media_data", id: frame.id, media_id: id, offset, size: file.bytes.length, eof: offset + slice.length >= file.bytes.length, data: slice.toString("base64") });
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
          mock.attachCalls.push({ session: seat.spec.name, replay: frame.replay === true, ...(typeof frame.offset === "number" ? { offset: frame.offset } : {}) });
          // A client that names an offset it has drawn up to is owed nothing more.
          if (options.offsets && typeof frame.offset === "number" && frame.offset >= (seat.sent.get(socket) ?? 0)) return;
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
        case "feedback_image": {
          mock.feedbackFrames.push("feedback_image");
          const token = `tok-${(stagedCount += 1)}`;
          staged.set(token, Buffer.from(String(frame.data ?? ""), "base64"));
          send(socket, { type: "feedback_image_saved", id: frame.id, image: token });
          return;
        }
        case "feedback": {
          mock.feedbackFrames.push("feedback");
          if (feedbackRefusal) {
            const reason = feedbackRefusal;
            feedbackRefusal = null;
            send(socket, { type: "error", id: frame.id, code: 6, error: "refused", reason });
            return;
          }
          const tokens = Array.isArray(frame.images) ? (frame.images as string[]) : [];
          if (tokens.some((token) => !staged.has(token))) {
            send(socket, { type: "error", id: frame.id, code: 6, error: "refused", reason: "image_expired" });
            return;
          }
          mock.feedbacks.push({ text: String(frame.text ?? ""), screen: (frame.screen ?? {}) as Record<string, unknown>, images: tokens.map((token) => staged.get(token)!) });
          send(socket, { type: "feedback_saved", id: frame.id, record: `2026-11-11-0000-${String(mock.feedbacks.length).padStart(4, "0")}` });
          return;
        }
        case "image_upload": {
          if (!seat) return;
          if (uploadRefusal) {
            send(socket, { type: "error", id: frame.id, code: 6, error: "refused", reason: uploadRefusal });
            return;
          }
          mock.uploads.push({ session: seat.spec.name, bytes: Buffer.from(String(frame.data ?? ""), "base64") });
          send(socket, { type: "image_uploaded", id: frame.id, session: frame.session, path: `/tmp/aterm/images/${seat.spec.name}-1/upload-${mock.uploads.length}.png` });
          return;
        }
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
        case "close": {
          const target = String(frame.target);
          mock.closes.push(target);
          const [name, ref] = target.split("@");
          const seat = live.get(name ?? "");
          if (!seat || seat.spec.ref !== ref) return void send(socket, { type: "error", id: frame.id, error: `${target} names no live session` });
          if (seat.spec.refuse_close) return void send(socket, { type: "error", id: frame.id, error: seat.spec.refuse_close });
          if (frame.force) mock.forced.push(target);
          else if (seat.spec.unpushed) return void send(socket, { type: "error", id: frame.id, reason: "unpushed_work", error: `${seat.spec.identity} holds commits on no remote.` });
          closed.set(target, seat.spec);
          live.delete(seat.spec.name);
          broadcast(sessionsFrame());
          send(socket, { type: "closed", id: frame.id, session: seat.spec.name, code: 0 });
          return;
        }
        case "reopen": {
          const target = String(frame.target);
          mock.reopens.push(target);
          const spec = closed.get(target);
          if (!spec) return void send(socket, { type: "error", id: frame.id, error: "no conversation is on record for that seat" });
          send(socket, { type: "reopened", id: frame.id, role: spec.role, seat: spec.seat });
          setTimeout(() => {
            add({ ...spec, name: `${spec.name}-2`, ref: `${spec.ref ?? "0"}2`.slice(0, 8) });
            broadcast(sessionsFrame());
          }, 300);
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
    closes: mock.closes,
    forced: mock.forced,
    reopens: mock.reopens,
    browserCalls: mock.browserCalls,
    pushCalls: mock.pushCalls,
    sceneOf: (session) => live.get(session)?.scene,
    holdWide(session, cols) {
      const seat = live.get(session);
      if (!seat) throw new Error(`no seat named ${session}`);
      seat.wideCols = cols;
      redraw(seat);
    },
    capRows(session, rows) {
      const seat = live.get(session);
      if (!seat) throw new Error(`no seat named ${session}`);
      seat.capRows = rows;
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
    addArtifact(session, artifact) {
      const frame = { type: "artifact", session, artifact };
      artifactFrames.set(String(artifact.id), frame);
      broadcast(frame);
    },
    closeArtifact(session, id) {
      artifactFrames.delete(id);
      broadcast({ type: "artifact_closed", session, artifact_id: id });
    },
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
    uploads: mock.uploads,
    attachCalls: mock.attachCalls,
    transcriptCalls: mock.transcriptCalls,
    setTranscript: (session, items) => void transcripts.set(session, items),
    appendTranscript: (session, items) => void transcripts.set(session, [...(transcripts.get(session) ?? []), ...items]),
    refuseTranscript: (session, reason) => void (reason ? transcriptRefusals.set(session, reason) : transcriptRefusals.delete(session)),
    holdTranscript: (session, ms) => void (ms > 0 ? transcriptHolds.set(session, ms) : transcriptHolds.delete(session)),
    mediaReads: mock.mediaReads,
    shareMedia(session, file) {
      const id = `media-${shared.size + 1}`;
      shared.set(id, { session, ...file, at: Date.now() });
      for (const socket of clients) send(socket, mediaFrame(id));
      return id;
    },
    closeMedia(session, id) {
      shared.delete(id);
      for (const socket of clients) send(socket, { type: "media_closed", session, media_id: id });
    },
    feedbacks: mock.feedbacks,
    feedbackFrames: mock.feedbackFrames,
    refuseFeedbackOnce(reason) {
      feedbackRefusal = reason;
    },
    expireFeedbackImages() {
      staged.clear();
    },
    refuseUploads(reason) {
      uploadRefusal = reason;
    },
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
