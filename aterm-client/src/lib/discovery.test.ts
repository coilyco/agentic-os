import { describe, expect, it } from "vitest";
import { FOUND_PREFIX, newFoundHosts, parseFound } from "./discovery";
import type { Host } from "./protocol";

function host(address: string, id = `saved:${address}`): Host {
  return { id, label: "x", address, kind: "daemon", status: { kind: "checking" } };
}

describe("parseFound", () => {
  it("keeps entries with a host and a numeric port, and names a nameless one by its host", () => {
    expect(parseFound([{ name: "tower", host: "tower.example.ts.net", port: "7419", version: "1.2" }, { host: "b.example.ts.net", port: "7419" }])).toEqual([
      { name: "tower", host: "tower.example.ts.net", port: "7419", version: "1.2" },
      { name: "b.example.ts.net", host: "b.example.ts.net", port: "7419" },
    ]);
  });

  it("drops anything malformed rather than failing the page", () => {
    expect(parseFound(undefined)).toEqual([]);
    expect(parseFound([null, 3, { host: "a" }, { host: "", port: "1" }, { host: "a.example.ts.net", port: "x/y" }])).toEqual([]);
  });
});

describe("newFoundHosts", () => {
  const found = [
    { name: "tower", host: "tower.example.ts.net", port: "7419" },
    { name: "laptop", host: "laptop.example.ts.net", port: "7419" },
  ];

  it("lists every found daemon, marked found, when the device knows none", () => {
    const added = newFoundHosts([], found);
    expect(added.map((entry) => entry.address)).toEqual(["wss://tower.example.ts.net:7419/", "wss://laptop.example.ts.net:7419/"]);
    expect(added.every((entry) => entry.found && entry.id.startsWith(FOUND_PREFIX) && entry.status.kind === "checking")).toBe(true);
  });

  it("skips a daemon already saved, found, or served as the local host", () => {
    const existing = [host("wss://tower.example.ts.net:7419/"), host("ws://127.0.0.1:7419", "local")];
    expect(newFoundHosts(existing, found).map((entry) => entry.label)).toEqual(["laptop"]);
    expect(newFoundHosts([...existing, ...newFoundHosts(existing, found)], found)).toEqual([]);
  });

  it("lists a daemon once when the reply repeats it", () => {
    expect(newFoundHosts([], [found[0]!, found[0]!])).toHaveLength(1);
  });
});
