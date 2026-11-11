import { describe, expect, it, vi } from "vitest";
import { SAVE_CHUNK_BYTES, saveAvailable, saveInApp } from "./save-file";

type Call = [string, Record<string, unknown> | undefined];

/** A plugin that keeps what it is sent, and can be told to fail one command. */
function plugin(fail?: { command: string; error: unknown }) {
  const calls: Call[] = [];
  const written: Uint8Array[] = [];
  const invoke = vi.fn(async (command: string, args?: Record<string, unknown>) => {
    calls.push([command, args]);
    const verb = command.replace("plugin:savefile|", "");
    if (fail?.command === verb) throw fail.error;
    if (verb === "begin") return { id: "7", name: String(args?.name) };
    if (verb === "write") written.push(Uint8Array.from(atob(String(args?.data)), (c) => c.charCodeAt(0)));
    if (verb === "finish") return { name: "print-log (1).gcode", place: "your Downloads folder" };
    if (verb === "available") return { place: "your Downloads folder" };
    return null;
  });
  return { invoke, calls, written };
}

describe("whether this app can save", () => {
  it("is a place in words when the plugin answers, and nothing in a browser or an older app", async () => {
    expect(await saveAvailable(plugin().invoke)).toBe("your Downloads folder");
    expect(await saveAvailable(null)).toBeNull();
    expect(await saveAvailable(vi.fn(async () => undefined))).toBeNull();
    expect(await saveAvailable(vi.fn(async () => Promise.reject({ code: "other", message: "no such plugin" })))).toBeNull();
  });
});

describe("saving a file in pieces", () => {
  it("opens, writes every byte in order in pieces under the cap, and finishes under the name it was given", async () => {
    const bytes = new Uint8Array(SAVE_CHUNK_BYTES * 2 + 5).map((_, at) => at % 251);
    const { invoke, calls, written } = plugin();
    const seen: number[] = [];
    const result = await saveInApp(new Blob([bytes], { type: "text/plain" }), "print-log.gcode", invoke, (done) => seen.push(done));
    expect(calls.map(([command]) => command.replace("plugin:savefile|", ""))).toEqual(["begin", "write", "write", "write", "finish"]);
    expect(calls[0]![1]).toEqual({ name: "print-log.gcode", mime: "text/plain" });
    expect(written.every((piece) => piece.length <= SAVE_CHUNK_BYTES)).toBe(true);
    expect(Buffer.concat(written)).toEqual(Buffer.from(bytes));
    expect(seen).toEqual([SAVE_CHUNK_BYTES, SAVE_CHUNK_BYTES * 2, bytes.length]);
    expect(result).toEqual({ ok: true, message: "Saved print-log (1).gcode to your Downloads folder." });
  });

  it("saves an empty file without a write", async () => {
    const { invoke, calls } = plugin();
    expect((await saveInApp(new Blob([]), "empty.txt", invoke)).ok).toBe(true);
    expect(calls.map(([command]) => command)).toEqual(["plugin:savefile|begin", "plugin:savefile|finish"]);
  });

  it("cancels what it opened and says in words when a piece fails", async () => {
    const { invoke, calls } = plugin({ command: "write", error: { code: "unwritable", message: "No space left on device" } });
    const result = await saveInApp(new Blob([new Uint8Array(10)]), "a.bin", invoke);
    expect(result).toEqual({ ok: false, message: "a.bin (10 B) was not saved: No space left on device. Nothing was left in Downloads." });
    expect(calls.at(-1)).toEqual(["plugin:savefile|cancel", { id: "7" }]);
  });

  it("names a busy plugin and a missing folder in words and does not cancel what it never opened", async () => {
    const busy = plugin({ command: "begin", error: { code: "busy", message: "Too many saves are open at once." } });
    expect((await saveInApp(new Blob(["x"]), "a.txt", busy.invoke)).message).toContain("another save is still open");
    expect(busy.calls.map(([command]) => command)).toEqual(["plugin:savefile|begin"]);
    const none = plugin({ command: "begin", error: { code: "no_folder", message: "" } });
    expect((await saveInApp(new Blob(["x"]), "a.txt", none.invoke)).message).toContain("this device has no Downloads folder");
  });
});
