import { beforeEach, describe, expect, it } from "vitest";
import { checkShares, discard, merge, share } from "./share.svelte";
import type { Shared } from "./share-target";

const file = (name: string) => new File([new Uint8Array([1])], name, { type: "image/png" });
const shared = (names: string[], text = "", notes: string[] = []): Shared => ({ files: names.map(file), text, notes });

beforeEach(() => discard());

describe("merge", () => {
  it("starts from the first take, with at most four images and a note for the rest", () => {
    const merged = merge(null, shared(["1", "2", "3", "4", "5"]));
    expect(merged.files.map((each) => each.name)).toEqual(["1", "2", "3", "4"]);
    expect(merged.notes).toHaveLength(1);
    expect(merged.notes[0]).toContain("5");
  });

  it("adds a second share to the first instead of replacing it", () => {
    const merged = merge(shared(["a", "b", "c"], "one", ["n1"]), shared(["d", "e"], "two", ["n2"]));
    expect(merged.files.map((each) => each.name)).toEqual(["a", "b", "c", "d"]);
    expect(merged.text).toBe("one\ntwo");
    expect(merged.notes[0]).toBe("n1");
    expect(merged.notes[1]).toBe("n2");
    expect(merged.notes[2]).toContain("e");
  });
});

describe("checkShares", () => {
  it("opens the sheet only when something was shared", async () => {
    await checkShares(async () => null);
    expect(share.open).toBe(false);
    await checkShares(async () => shared(["a"]));
    expect(share.open).toBe(true);
    expect(share.pending?.files).toHaveLength(1);
  });

  it("discard drops the share and closes the sheet", async () => {
    await checkShares(async () => shared(["a"]));
    discard();
    expect(share.pending).toBeNull();
    expect(share.open).toBe(false);
  });
});
