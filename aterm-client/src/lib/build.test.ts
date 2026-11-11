import { describe, expect, it } from "vitest";
import { builtHost } from "./build";

describe("builtHost", () => {
  it("names a host from the address and label the build was given", () => {
    expect(builtHost("ws://127.0.0.1:7419/", "this Mac")).toEqual({ label: "this Mac", address: "ws://127.0.0.1:7419/" });
  });

  it("names none when the build gave no address, or one that is not a websocket", () => {
    expect(builtHost(undefined, "this Mac")).toBeNull();
    expect(builtHost("", "this Mac")).toBeNull();
    expect(builtHost("127.0.0.1:7419", "this Mac")).toBeNull();
  });

  it("falls back to a plain label", () => {
    expect(builtHost("wss://tower.example.ts.net/", undefined)?.label).toBe("this host");
  });
});
