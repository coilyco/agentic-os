import { describe, expect, it } from "vitest";
import { daemonPage, diagnose } from "./reach";

const ADDRESS = "wss://mac.example.ts.net:7419/";

function refusal(layer: string | null, text: string, status = 403): Response {
  return new Response(`aterm daemon: ${text}\n`, { status, headers: layer ? { "X-Aterm-Refusal": layer } : {} });
}

/** An ask that records each call and answers from the script, in order. */
function script(...steps: Array<Response | Error>) {
  const calls: Array<{ url: string; init: RequestInit }> = [];
  const ask = async (url: string, init: RequestInit): Promise<Response> => {
    calls.push({ url, init });
    const step = steps[Math.min(calls.length, steps.length) - 1]!;
    if (step instanceof Error) throw step;
    return step;
  };
  return { ask, calls };
}

describe("daemonPage", () => {
  it("turns a socket address into the daemon's page and refuses anything else", () => {
    expect(daemonPage("wss://mac.example.ts.net:7419/some/path?x=1")?.toString()).toBe("https://mac.example.ts.net:7419/");
    expect(daemonPage("ws://127.0.0.1:7419")?.toString()).toBe("http://127.0.0.1:7419/");
    expect(daemonPage("https://mac.example.ts.net")).toBeNull();
    expect(daemonPage("not an address")).toBeNull();
  });
});

describe("diagnose", () => {
  it.each([
    ["ownership", "that device belongs to another tailnet user", "refused this device"],
    ["name", "open this daemon by its tailnet name", "refused this address"],
    ["origin", "this page may not open a session socket", "refused this page's origin"],
  ])("names a %s refusal the daemon sent back", async (layer, text, lead) => {
    const { ask, calls } = script(refusal(layer, text));
    const found = await diagnose(ADDRESS, ask);
    expect(found.layer).toBe(layer);
    expect(found.message).toContain(lead);
    expect(found.message).toContain(text);
    expect(calls).toHaveLength(1);
    expect(calls[0]!.url).toBe("https://mac.example.ts.net:7419/_aterm/reach");
  });

  it("reports an admitted device whose socket still failed", async () => {
    const { ask } = script(new Response('{"layer":"ok"}', { status: 200 }));
    expect((await diagnose(ADDRESS, ask)).layer).toBe("ok");
  });

  it("says a daemon that answered unreadably did answer, so the name, TLS and route work", async () => {
    // An older build sends no CORS header, so only the opaque ask resolves.
    const { ask, calls } = script(new TypeError("Failed to fetch"), new Response(null, { status: 200 }));
    const found = await diagnose(ADDRESS, ask);
    expect(found.layer).toBe("answered");
    expect(found.message).toContain("https://mac.example.ts.net:7419/");
    expect(calls[1]!.init.mode).toBe("no-cors");
  });

  it("says nothing answered, and what a browser cannot tell apart, when both asks fail", async () => {
    const { ask, calls } = script(new TypeError("Failed to fetch"), new TypeError("Failed to fetch"));
    const found = await diagnose(ADDRESS, ask);
    expect(found.layer).toBe("network");
    expect(found.message).toMatch(/Private DNS/);
    expect(found.message).toMatch(/error code/);
    expect(calls).toHaveLength(2);
  });

  it("reads an unnamed error status as an answer that was not a refusal", async () => {
    const { ask } = script(refusal(null, "oops", 500));
    const found = await diagnose(ADDRESS, ask);
    expect(found.layer).toBe("answered");
    expect(found.message).toContain("HTTP 500");
  });

  it("treats a hung ask as no answer", async () => {
    const hung = (_url: string, init: RequestInit) =>
      new Promise<Response>((_resolve, reject) => init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError"))));
    expect((await diagnose(ADDRESS, hung, 10)).layer).toBe("network");
  });

  it("does not ask when the address is not a daemon's", async () => {
    const { ask, calls } = script(new Response("", { status: 200 }));
    expect((await diagnose("nonsense", ask)).layer).toBe("network");
    expect(calls).toHaveLength(0);
  });
});
