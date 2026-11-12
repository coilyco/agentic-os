import { describe, expect, it, vi } from "vitest";
import { HandoffError } from "./handoff";
import { systemBrowser } from "./system-browser";

// The page's half of the `systembrowser` plugin (COI-3657), scripted. Checked: the names
// agreed with its owner, and what the page says when the plugin answers badly.
function plugin(answers: { open?: () => Promise<unknown>; take?: () => Promise<unknown> } = {}) {
  const handlers = new Map<string, (payload: unknown) => void>();
  const unregister = vi.fn(async () => {});
  const invoke = vi.fn(async (command: string) => (command.endsWith("|open") ? answers.open?.() : answers.take?.()));
  const addPluginListener = vi.fn(async (_plugin: string, event: string, handler: (payload: unknown) => void) => {
    handlers.set(event, handler);
    return { unregister };
  });
  return { scope: { __TAURI__: { core: { invoke, addPluginListener } } }, invoke, addPluginListener, handlers, unregister };
}

describe("systemBrowser", () => {
  it("is null in a browser, and in an app without the plugin channel", () => {
    expect(systemBrowser({})).toBeNull();
    expect(systemBrowser({ __TAURI__: { core: { invoke: () => {} } } })).toBeNull();
  });

  it("opens the address through the plugin's open command", async () => {
    const p = plugin();
    await systemBrowser(p.scope)!.open("https://accounts.example.tech/signin?x=1");
    expect(p.invoke).toHaveBeenCalledWith("plugin:systembrowser|open", { url: "https://accounts.example.tech/signin?x=1" });
  });

  it("says why, in words, when the plugin refuses the address", async () => {
    const p = plugin({ open: () => Promise.reject("host is not an accounts host") });
    const failure = await systemBrowser(p.scope)!.open("https://elsewhere.example/signin").catch((error) => error);
    expect(failure).toBeInstanceOf(HandoffError);
    expect(failure.message).toBe("This app could not open the sign-in page: host is not an accounts host");
  });

  it("still says it in words when the refusal has no reason", async () => {
    const p = plugin({ open: () => Promise.reject(undefined) });
    await expect(systemBrowser(p.scope)!.open("https://x")).rejects.toThrow("This app could not open the sign-in page.");
  });

  it("takes the pending return once, and reads a null, an empty answer or a failure as nothing", async () => {
    const address = "dev.coilyco.treehouse:/signin?code=c&state=s";
    expect(await systemBrowser(plugin({ take: async () => ({ url: address }) }).scope)!.takeReturn()).toBe(address);
    expect(await systemBrowser(plugin({ take: async () => address }).scope)!.takeReturn()).toBe(address);
    for (const take of [async () => ({ url: null }), async () => ({}), async () => null, async () => "", async () => 7, () => Promise.reject("no")]) {
      expect(await systemBrowser(plugin({ take }).scope)!.takeReturn()).toBeNull();
    }
  });

  it("hears the return and the dismissal on the plugin listener channel, and ignores a return with no address", () => {
    const p = plugin();
    const browser = systemBrowser(p.scope)!;
    const returned = vi.fn();
    const closed = vi.fn();
    browser.onReturn(returned);
    browser.onClosed(closed);
    expect(p.addPluginListener.mock.calls.map((call) => [call[0], call[1]])).toEqual([
      ["systembrowser", "return"],
      ["systembrowser", "closed"],
    ]);
    p.handlers.get("return")!({ url: "dev.coilyco.treehouse:/signin?code=c&state=s" });
    p.handlers.get("return")!({});
    p.handlers.get("return")!(null);
    p.handlers.get("closed")!({});
    expect(returned).toHaveBeenCalledTimes(1);
    expect(returned).toHaveBeenCalledWith("dev.coilyco.treehouse:/signin?code=c&state=s");
    expect(closed).toHaveBeenCalledTimes(1);
  });

  it("unregisters on stop, whether the channel had answered yet or not", async () => {
    const late = plugin();
    const stopEarly = systemBrowser(late.scope)!.onReturn(() => {});
    stopEarly();
    await vi.waitFor(() => expect(late.unregister).toHaveBeenCalledTimes(1));
    const settled = plugin();
    const stopLate = systemBrowser(settled.scope)!.onClosed(() => {});
    await vi.waitFor(() => expect(settled.addPluginListener).toHaveBeenCalled());
    await Promise.resolve();
    stopLate();
    await vi.waitFor(() => expect(settled.unregister).toHaveBeenCalledTimes(1));
  });
});
