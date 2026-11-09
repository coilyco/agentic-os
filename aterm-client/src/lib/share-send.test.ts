import { describe, expect, it, vi } from "vitest";
import { sendToSeat, type SendIo } from "./share-send";

function rig() {
  const typed: [string, string][] = [];
  const queue: { run: () => void; ms: number }[] = [];
  const io: SendIo = { input: (id, data) => typed.push([id, data]), later: (run, ms) => queue.push({ run, ms }) };
  return { io, typed, queue };
}
const file = (name: string) => new File([new Uint8Array([1])], name, { type: "image/png" });

describe("sendToSeat", () => {
  it("pastes each path alone, then the words naming each path, then Enter a beat later", async () => {
    const { io, typed, queue } = rig();
    const upload = vi.fn(async (_id: string, image: Blob) => `/images/${(image as File).name}`);
    const line = await sendToSeat({ upload }, io, { id: "s1", seat: "claude", paste: true }, " what is this ", [file("a.png"), file("b.png")]);
    expect(upload).toHaveBeenCalledTimes(2);
    expect(typed).toEqual([["s1", "\x1b[200~/images/a.png\x1b[201~"]]);
    queue.forEach((step) => step.run());
    expect(typed.map(([, data]) => data)).toEqual([
      "\x1b[200~/images/a.png\x1b[201~",
      "\x1b[200~/images/b.png\x1b[201~",
      "\x1b[200~[Attached image 1 of 2: /images/a.png] [Attached image 2 of 2: /images/b.png] what is this\x1b[201~",
      "\r",
    ]);
    expect(queue.map((step) => step.ms)).toEqual([200, 400, 700]);
    expect(line).toBeNull();
  });

  it("types the escaped path and the words on one line for a seat with no bracketed paste, and says goose cannot attach", async () => {
    const { io, typed } = rig();
    const line = await sendToSeat({ upload: async () => "/tmp/my images/a.png" }, io, { id: "s2", seat: "goose", paste: false }, "hi\nthere", [file("a.png")]);
    expect(typed).toEqual([["s2", "/tmp/my\\ images/a.png hi there\r"]]);
    expect(line).toContain("goose does not attach images");
  });

  it("sends nothing at all when an upload fails, so the caller keeps its screenshot and words", async () => {
    const { io, typed, queue } = rig();
    const upload = vi.fn().mockResolvedValueOnce("/images/a.png").mockRejectedValueOnce(new Error("The host took that as over 3 MB."));
    await expect(sendToSeat({ upload }, io, { id: "s1", seat: "claude", paste: true }, "x", [file("a.png"), file("b.png")])).rejects.toThrow("over 3 MB");
    expect(typed).toEqual([]);
    expect(queue).toEqual([]);
  });

  it("reports progress before each upload", async () => {
    const { io } = rig();
    const progress = vi.fn();
    await sendToSeat({ upload: async () => "/p" }, io, { id: "s1", seat: "claude", paste: true }, "", [file("a.png"), file("b.png")], progress);
    expect(progress.mock.calls).toEqual([[0, 2], [1, 2]]);
  });
});
