import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const SCOPE = "https://mac.example.ts.net:7419/";
const SOURCE = readFileSync(new URL("../../pwa/sw.js", import.meta.url), "utf8").replace(
  '"__SHELL__"',
  () => JSON.stringify({ version: "v1", files: ["./", "./assets/app.js"] }),
);

type Handler = (event: unknown) => void;

/** Runs the worker against fakes, so its rules need no browser. */
function boot(network: (url: string) => Promise<Response>) {
  const handlers: Record<string, Handler> = {};
  const stores = new Map<string, Map<string, Response>>();
  const cacheOf = (name: string) => {
    if (!stores.has(name)) stores.set(name, new Map());
    const store = stores.get(name)!;
    return {
      put: async (key: string, response: Response) => void store.set(key, response),
      match: async (request: string | Request) => store.get(typeof request === "string" ? request : request.url)?.clone(),
    };
  };
  const caches = {
    open: async (name: string) => cacheOf(name),
    keys: async () => [...stores.keys()],
    delete: async (name: string) => stores.delete(name),
    match: async () => undefined,
  };
  const self = {
    registration: { scope: SCOPE },
    location: new URL(SCOPE),
    clients: { claim: async () => {} },
    addEventListener: (type: string, handler: Handler) => void (handlers[type] = handler),
  };
  const fetcher = (input: string | Request) => network(typeof input === "string" ? input : input.url);
  new Function("self", "caches", "fetch", SOURCE)(self, caches, fetcher);

  const settle = async (type: string, extra: object = {}) => {
    let work: Promise<unknown> = Promise.resolve();
    handlers[type]!({ waitUntil: (promise: Promise<unknown>) => (work = promise), ...extra });
    await work;
  };
  const respond = async (request: object) => {
    let answer: Promise<Response> | undefined;
    handlers.fetch!({ request, respondWith: (promise: Promise<Response>) => (answer = promise) });
    return answer ? await answer : undefined;
  };
  return { stores, settle, respond };
}

const page = (body: string) => new Response(body, { status: 200 });
const asked = (url: string, mode = "cors", method = "GET") => ({ url, mode, method });

describe("the app-shell worker", () => {
  it("keeps a copy of every shell file when it installs", async () => {
    const worker = boot(async (url) => page(url));
    await worker.settle("install");
    expect([...worker.stores.get("aterm-shell-v1")!.keys()]).toEqual([`${SCOPE}`, `${SCOPE}assets/app.js`]);
  });

  it("refuses to install when the shell came back through a redirect", async () => {
    const redirected = Object.defineProperty(page("sign in"), "redirected", { value: true });
    const worker = boot(async () => redirected);
    await expect(worker.settle("install")).rejects.toThrow(/did not come back as itself/);
  });

  it("answers a launch with the cached shell when the daemon is down", async () => {
    let up = true;
    const worker = boot(async (url) => (up ? page(`live ${url}`) : Promise.reject(new TypeError("Failed to fetch"))));
    await worker.settle("install");
    up = false;
    const shell = await worker.respond(asked(SCOPE, "navigate"));
    expect(await shell!.text()).toBe(`live ${SCOPE}`);
  });

  it("lets the network answer a launch when the daemon is up, including a refusal", async () => {
    const worker = boot(async () => new Response("no", { status: 403 }));
    const answer = await worker.respond(asked(SCOPE, "navigate"));
    expect(answer!.status).toBe(403);
  });

  it("serves a built file from the cache, and passes everything else to the network", async () => {
    const worker = boot(async (url) => page(`net ${url}`));
    await worker.settle("install");
    expect(await (await worker.respond(asked(`${SCOPE}assets/app.js`)))!.text()).toBe(`net ${SCOPE}assets/app.js`);
    expect(await worker.respond(asked(`${SCOPE}assets/app.js`, "cors", "POST"))).toBeUndefined();
    expect(await worker.respond(asked("https://other.example/assets/app.js"))).toBeUndefined();
  });

  it("drops an older version's cache when it takes over", async () => {
    const worker = boot(async (url) => page(url));
    worker.stores.set("aterm-shell-v0", new Map());
    worker.stores.set("unrelated", new Map());
    await worker.settle("activate");
    expect([...worker.stores.keys()]).toEqual(["unrelated"]);
  });
});
