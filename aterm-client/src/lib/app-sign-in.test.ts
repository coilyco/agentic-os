import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// app.svelte.ts dials hosts on import, so the browser globals it reads are stubbed first.
class NoSocket {
  addEventListener(): void {}
  send(): void {}
  close(): void {}
}

async function mock() {
  vi.stubGlobal("WebSocket", NoSocket);
  vi.stubGlobal("location", { search: "", hostname: "localhost", protocol: "http:", host: "localhost" });
  vi.stubGlobal("document", { visibilityState: "visible", title: "", addEventListener() {}, removeEventListener() {} });
  return import("./app.svelte");
}

beforeEach(() => vi.resetModules());
afterEach(() => vi.unstubAllGlobals());

const SEAT = "prod-director-breezy-griffin-ab12";
const AUTHORIZE = "https://claude.com/cai/oauth/authorize?state=S";

describe("a seat's sign-in card and its own authorize page (COI-3803)", () => {
  it("drops the seat's authorize page card while its sign-in card is up, and keeps every other page", async () => {
    const { app, offerPage, setSigningIn } = await mock();
    offerPage(SEAT, AUTHORIZE);
    offerPage(SEAT, "https://docs.example.com/");
    offerPage("other-seat", AUTHORIZE);
    setSigningIn(SEAT, true);
    expect(app.openedPages.map((page) => [page.sessionId, page.url])).toEqual([
      [SEAT, "https://docs.example.com/"],
      ["other-seat", AUTHORIZE],
    ]);
    offerPage(SEAT, AUTHORIZE);
    expect(app.openedPages).toHaveLength(2);
  });

  it("offers it again once the sign-in card is gone", async () => {
    const { app, offerPage, setSigningIn } = await mock();
    setSigningIn(SEAT, true);
    setSigningIn(SEAT, false);
    offerPage(SEAT, AUTHORIZE);
    expect(app.openedPages.map((page) => page.url)).toEqual([AUTHORIZE]);
  });
});
