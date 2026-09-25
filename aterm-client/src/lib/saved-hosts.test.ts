import { describe, expect, it } from "vitest";
import { parseHostInput } from "./saved-hosts";

describe("parseHostInput", () => {
  it("turns a tailnet name into the daemon's wss address", () => {
    expect(parseHostInput(" mac.example.ts.net ")).toEqual({ label: "mac", address: "wss://mac.example.ts.net:7419/" });
  });

  it("keeps a port and a full URL", () => {
    expect(parseHostInput("mac.example.ts.net:7421").address).toBe("wss://mac.example.ts.net:7421/");
    expect(parseHostInput("ws://127.0.0.1:7420").address).toBe("ws://127.0.0.1:7420/");
  });

  it("refuses a bare IP, since the daemon answers by name", () => {
    expect(() => parseHostInput("100.64.0.1")).toThrow(/tailnet name/);
  });

  it("refuses empty or path-shaped input", () => {
    expect(() => parseHostInput("  ")).toThrow();
    expect(() => parseHostInput("mac/aterm")).toThrow(/not a host name/);
  });
});
