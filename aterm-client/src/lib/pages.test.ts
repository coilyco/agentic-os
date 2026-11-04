import { describe, expect, it } from "vitest";
import { MAX_OFFERED, cardText, offer, pageLabel, type OpenedPage } from "./pages";

const page = (sessionId: string, url: string, at = 0): Omit<OpenedPage, "id"> => ({ sessionId, url, at });

describe("offer", () => {
  it("keeps cards in the order the seats asked", () => {
    const list = offer(offer([], page("a", "https://a.example/"), 1), page("b", "https://b.example/"), 2);
    expect(list.map((each) => each.sessionId)).toEqual(["a", "b"]);
  });

  it("makes a seat's repeat of an address one card, the newest", () => {
    const first = offer([], page("a", "https://a.example/login", 1), 1);
    const again = offer(first, page("a", "https://a.example/login", 9), 2);
    expect(again).toEqual([{ id: 2, sessionId: "a", url: "https://a.example/login", at: 9 }]);
  });

  it("keeps two seats' cards for the same address, since each one asked", () => {
    const list = offer(offer([], page("a", "https://x.example/"), 1), page("b", "https://x.example/"), 2);
    expect(list).toHaveLength(2);
  });

  it("drops the oldest once a seat floods the screen", () => {
    let list: OpenedPage[] = [];
    for (let id = 1; id <= MAX_OFFERED + 2; id++) list = offer(list, page("a", `https://a.example/${id}`), id);
    expect(list).toHaveLength(MAX_OFFERED);
    expect(list[0]?.url).toBe("https://a.example/3");
    expect(list.at(-1)?.url).toBe(`https://a.example/${MAX_OFFERED + 2}`);
  });
});

describe("pageLabel", () => {
  it("shows host and path", () => {
    expect(pageLabel("https://example.com/login")).toBe("example.com/login");
    expect(pageLabel("https://example.com/")).toBe("example.com");
  });

  it("hides the query, which may carry a login's state, and says it is there", () => {
    expect(pageLabel("https://accounts.example.com/o/auth?client_id=abc&state=SECRET")).toBe("accounts.example.com/o/auth?…");
    expect(pageLabel("https://accounts.example.com/o/auth?state=SECRET")).not.toContain("SECRET");
  });

  it("keeps a port, and cuts a long path with an ellipsis", () => {
    expect(pageLabel("http://localhost:5173/a")).toBe("localhost:5173/a");
    expect(pageLabel(`https://example.com/${"x".repeat(100)}`, 20)).toBe("example.com/xxxxxxx…");
  });

  it("names nothing it cannot read as an http or https address", () => {
    expect(pageLabel("file:///etc/passwd")).toBe("a page");
    expect(pageLabel("javascript:alert(1)")).toBe("a page");
    expect(pageLabel("not a url")).toBe("a page");
  });
});

describe("cardText", () => {
  it("names the seat, and counts what waits behind", () => {
    expect(cardText("Beetle-Ox", 0)).toBe("Beetle-Ox opened a page");
    expect(cardText("Beetle-Ox", 1)).toBe("Beetle-Ox opened a page, and 1 more is waiting");
    expect(cardText("Beetle-Ox", 3)).toBe("Beetle-Ox opened a page, and 3 more are waiting");
  });
});
