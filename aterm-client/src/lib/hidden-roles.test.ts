import { describe, expect, it } from "vitest";
import { flipHidden, parseHidden, splitHidden } from "./hidden-roles";

const seat = (id: string, role: string) => ({ id, role });

describe("parseHidden", () => {
  it("reads a list of slugs and nothing else", () => {
    expect(parseHidden('["eng-platform","scientist"]')).toEqual(["eng-platform", "scientist"]);
    expect(parseHidden('["a",1]')).toEqual(["a"]);
    expect(parseHidden(null)).toEqual([]);
    expect(parseHidden("not json")).toEqual([]);
    expect(parseHidden('{"a":1}')).toEqual([]);
  });
});

describe("flipHidden", () => {
  it("hides a role and shows it again", () => {
    expect(flipHidden("a", [])).toEqual(["a"]);
    expect(flipHidden("a", ["a", "b"])).toEqual(["b"]);
  });
});

describe("splitHidden", () => {
  const all = [seat("p1", "eng-platform"), seat("p2", "eng-platform"), seat("f1", "frontend-eng"), seat("d1", "prod-director")];

  it("takes a hidden role's quiet seats out and leaves every other role", () => {
    const out = splitHidden(all, ["eng-platform"], () => false);
    expect(out.hidden.map((s) => s.id)).toEqual(["p1", "p2"]);
    expect(out.shown.map((s) => s.id)).toEqual(["f1", "d1"]);
  });

  it("keeps a hidden role's seat that holds a question, is pinned or is open", () => {
    const out = splitHidden(all, ["eng-platform"], (s) => s.id === "p2");
    expect(out.hidden.map((s) => s.id)).toEqual(["p1"]);
    expect(out.shown.map((s) => s.id)).toEqual(["p2", "f1", "d1"]);
  });

  it("hides nothing when no role is hidden", () => {
    expect(splitHidden(all, [], () => false).hidden).toEqual([]);
  });
});
