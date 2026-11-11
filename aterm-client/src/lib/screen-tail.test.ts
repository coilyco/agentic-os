import { describe, expect, it } from "vitest";
import { MAX_SEATS, ScreenTails, TAIL_BYTES } from "./screen-tail";

const bytes = (text: string) => new TextEncoder().encode(text);
const text = (data: Uint8Array | undefined) => (data ? new TextDecoder().decode(data) : "");

describe("what a seat's terminal drew", () => {
  it("is kept in order and handed back whole with the offset after it", () => {
    const tails = new ScreenTails();
    tails.keep("s", "run1");
    expect(tails.resume("s"), "nothing drawn yet").toBeNull();
    tails.note("s", bytes("hello "), 0);
    tails.note("s", bytes("world"), 6);
    const back = tails.resume("s")!;
    expect(text(back.bytes)).toBe("hello world");
    expect(back.end).toBe(11);
  });

  it("draws only what is new when a frame overlaps what is held, and nothing for a repeat", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    tails.note("s", bytes("abcdef"), 0);
    const overlap = tails.note("s", bytes("defgh"), 3);
    expect(text(overlap.data!)).toBe("gh");
    expect(overlap.reset).toBe(false);
    expect(tails.note("s", bytes("abc"), 0)).toEqual({ data: null, reset: false });
    expect(text(tails.resume("s")!.bytes)).toBe("abcdefgh");
  });

  it("asks for the screen to be cleared when a frame does not join what is held", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    tails.note("s", bytes("old screen"), 0);
    const gap = tails.note("s", bytes("new"), 500);
    expect(gap.reset).toBe(true);
    expect(text(gap.data!)).toBe("new");
    expect(text(tails.resume("s")!.bytes), "the old bytes are gone").toBe("new");
    expect(tails.resume("s")!.end).toBe(503);
  });

  it("takes the first frame of a seat it was just told to keep as the start, with no reset", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    const first = tails.note("s", bytes("replay"), 9000);
    expect(first).toEqual({ data: bytes("replay"), reset: false });
    expect(tails.resume("s")!.end).toBe(9006);
  });

  it("holds at most 1 MiB, dropping the oldest bytes and keeping the offset exact", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    const piece = new Uint8Array(TAIL_BYTES / 2 + 10).fill(1);
    tails.note("s", piece, 0);
    tails.note("s", piece, piece.length);
    const back = tails.resume("s")!;
    expect(back.bytes.length).toBe(TAIL_BYTES);
    expect(back.end).toBe(piece.length * 2);
  });

  it("starts over for a different run of the same seat, and never for the same one", () => {
    const tails = new ScreenTails();
    tails.keep("s", "run1");
    tails.note("s", bytes("x"), 0);
    tails.keep("s", "run1");
    expect(tails.resume("s")).not.toBeNull();
    tails.keep("s", "run2");
    expect(tails.resume("s")).toBeNull();
  });

  it("reads a first frame with no offset as the start of the stream, which is how the daemon sends 0", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    tails.note("s", bytes("young seat"), undefined);
    tails.note("s", bytes(" more"), 10);
    const back = tails.resume("s")!;
    expect(text(back.bytes)).toBe("young seat more");
    expect(back.end).toBe(15);
  });

  it("forgets a seat whose frame has no offset, since it cannot be joined", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    tails.note("s", bytes("x"), 0);
    expect(tails.note("s", bytes("y"), undefined).data).not.toBeNull();
    expect(tails.resume("s")).toBeNull();
  });

  it("keeps the most recent seats only, and passes a seat it does not keep straight through", () => {
    const tails = new ScreenTails();
    for (let at = 0; at <= MAX_SEATS; at++) {
      tails.keep(`s${at}`, "r");
      tails.note(`s${at}`, bytes("x"), 0);
    }
    expect(tails.resume("s0"), "the oldest was dropped").toBeNull();
    expect(tails.resume(`s${MAX_SEATS}`)).not.toBeNull();
    expect(tails.note("other", bytes("z"), 0)).toEqual({ data: bytes("z"), reset: false });
  });

  it("clears what was drawn when the screen is asked for whole", () => {
    const tails = new ScreenTails();
    tails.keep("s", "r");
    tails.note("s", bytes("x"), 0);
    tails.clear("s");
    expect(tails.resume("s")).toBeNull();
  });
});
