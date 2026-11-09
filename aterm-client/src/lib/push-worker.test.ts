import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";

const SCOPE = "https://coilyco.dev/aterm/";
const SOURCE = readFileSync(new URL("../../pwa/sw.js", import.meta.url), "utf8").replace('"__SHELL__"', () => JSON.stringify({ version: "v1", files: ["./"] }));

type Handler = (event: unknown) => void;
interface FakeWindow {
  visibilityState: string;
  focused: boolean;
  focus: ReturnType<typeof vi.fn>;
  postMessage: ReturnType<typeof vi.fn>;
}

const windowOf = (visibilityState = "hidden", focused = false, focus = vi.fn().mockResolvedValue(undefined)): FakeWindow => ({ visibilityState, focused, focus, postMessage: vi.fn() });

/** Runs the worker against fakes, so what a push shows and a tap does need no browser. */
function boot(windows: FakeWindow[] = []) {
  const handlers: Record<string, Handler> = {};
  const shown: { title: string; options: Record<string, unknown>; close: ReturnType<typeof vi.fn> }[] = [];
  const opened: string[] = [];
  const self = {
    registration: { scope: SCOPE, showNotification: async (title: string, options: Record<string, unknown>) => void shown.push({ title, options, close: vi.fn() }),
      getNotifications: async ({ tag }: { tag: string }) => shown.filter((each) => each.options.tag === tag),
    },
    location: new URL(SCOPE),
    clients: { claim: async () => {}, matchAll: async () => windows, openWindow: async (url: string) => void opened.push(url) },
    addEventListener: (type: string, handler: Handler) => void (handlers[type] = handler),
  };
  new Function("self", "caches", "fetch", SOURCE)(self, {}, async () => new Response(""));
  const settle = async (type: string, event: object) => {
    let work: Promise<unknown> = Promise.resolve();
    handlers[type]!({ waitUntil: (promise: Promise<unknown>) => (work = promise), ...event });
    await work;
  };
  return {
    shown,
    opened,
    push: (payload: unknown) => settle("push", { data: payload === undefined ? null : { json: () => (typeof payload === "string" ? JSON.parse(payload) : payload) } }),
    tap: (data: unknown) => {
      const close = vi.fn();
      return settle("notificationclick", { notification: { close, data } }).then(() => close);
    },
  };
}

const WAITING = { kind: "ask", session: "frontend-eng-1", role: "frontend-eng", identity: "Imp-Dragonfly", title: "Imp-Dragonfly is asking", body: "Ship it?", tag: "aterm-frontend-eng-1" };

describe("a push for a waiting seat", () => {
  it("shows the daemon's title and body under the daemon's tag, with the session kept for the tap", async () => {
    const worker = boot();
    await worker.push(WAITING);
    expect(worker.shown).toHaveLength(1);
    expect(worker.shown[0]).toMatchObject({
      title: "Imp-Dragonfly is asking",
      options: { body: "Ship it?", tag: "aterm-frontend-eng-1", data: { session: "frontend-eng-1" }, icon: `${SCOPE}icon-192.png` },
    });
  });

  it("alerts again for a repeat under the same tag when no window is watching", async () => {
    const worker = boot([windowOf("hidden", false)]);
    await worker.push(WAITING);
    expect(worker.shown[0]!.options).toMatchObject({ silent: false, renotify: true });
  });

  it("shows it silently and closes it at once while a visible focused window carries the cue", async () => {
    const worker = boot([windowOf("hidden"), windowOf("visible", true)]);
    await worker.push(WAITING);
    expect(worker.shown).toHaveLength(1);
    expect(worker.shown[0]!.options).toMatchObject({ silent: true, renotify: false });
    expect(worker.shown[0]!.close).toHaveBeenCalledOnce();
  });

  it("leaves it in the tray when no window is watching, or the window is visible but not focused", async () => {
    for (const windows of [[], [windowOf("hidden", false)], [windowOf("visible", false)]]) {
      const worker = boot(windows);
      await worker.push(WAITING);
      expect(worker.shown[0]!.close).not.toHaveBeenCalled();
    }
  });

  it("shows a done silently and keeps it in the tray, since it fires for seats that are not asking this person", async () => {
    const worker = boot();
    await worker.push({ ...WAITING, kind: "done", title: "Imp-Dragonfly is done" });
    expect(worker.shown[0]!.options).toMatchObject({ silent: true, renotify: false });
    expect(worker.shown[0]!.close).not.toHaveBeenCalled();
  });

  it("keeps an ask and a prompt audible", async () => {
    for (const kind of ["ask", "prompt"]) {
      const worker = boot();
      await worker.push({ ...WAITING, kind });
      expect(worker.shown[0]!.options).toMatchObject({ silent: false, renotify: true });
    }
  });

  it("is audible for a window that is visible but not focused", async () => {
    const worker = boot([windowOf("visible", false)]);
    await worker.push(WAITING);
    expect(worker.shown[0]!.options).toMatchObject({ silent: false, renotify: true });
  });

  it("never asks for silent and renotify together, which the browser throws on", async () => {
    for (const windows of [[], [windowOf("visible", true)]]) {
      const worker = boot(windows);
      await worker.push(WAITING);
      const { silent, renotify } = worker.shown[0]!.options;
      expect(silent && renotify).toBe(false);
    }
  });

  it("still shows something for a push that is not JSON, so the browser adds no notice of its own", async () => {
    const worker = boot();
    await worker.push("not json {");
    expect(worker.shown).toHaveLength(1);
    expect(worker.shown[0]).toMatchObject({ title: "A seat is waiting", options: { tag: "aterm", body: "" } });
  });

  it("still shows something for a push with no payload at all", async () => {
    const worker = boot();
    await worker.push(undefined);
    expect(worker.shown[0]!.title).toBe("A seat is waiting");
  });

  it("builds a title and tag from what a thin payload does carry, and ignores fields of the wrong type", async () => {
    const worker = boot();
    await worker.push({ session: "s9", identity: "Frog-Ox", title: 7, body: { not: "text" }, tag: null });
    expect(worker.shown[0]).toMatchObject({ title: "Frog-Ox is waiting", options: { body: "", tag: "aterm-s9", data: { session: "s9" } } });
  });
});

describe("tapping a notification", () => {
  it("closes it, focuses the open window, and tells it which seat", async () => {
    const open = windowOf("hidden");
    const worker = boot([open]);
    const close = await worker.tap({ session: "frontend-eng-1" });
    expect(close).toHaveBeenCalledOnce();
    expect(open.focus).toHaveBeenCalledOnce();
    expect(open.postMessage).toHaveBeenCalledWith({ type: "aterm-open-seat", session: "frontend-eng-1" });
    expect(worker.opened).toEqual([]);
  });

  it("opens the app on the seat when no window is open, inside the worker's scope", async () => {
    const worker = boot([]);
    await worker.tap({ session: "frontend eng/1" });
    expect(worker.opened).toEqual([`${SCOPE}?seat=frontend%20eng%2F1`]);
  });

  it("still tells the page when the browser refuses to focus it", async () => {
    const open = windowOf("hidden", false, vi.fn().mockRejectedValue(new Error("no")));
    const worker = boot([open]);
    await worker.tap({ session: "s1" });
    expect(open.postMessage).toHaveBeenCalledWith({ type: "aterm-open-seat", session: "s1" });
  });

  it("opens the app at its start when the notification names no seat", async () => {
    const worker = boot([]);
    await worker.tap(undefined);
    expect(worker.opened).toEqual([SCOPE]);
  });
});
