// COI-3334 and COI-3243 bug 3: the ?taptrace record Kai hands back from her phone.
import { card, expect, kindOf, loadFonts, SEATS, test } from "../harness.ts";
import { ClaudeAsk, MULTI } from "../scenes.ts";
import type { TraceEvent } from "../../src/lib/tap-trace.ts";

const ask = {
  type: "ask",
  ask: {
    id: "trace-1",
    session: SEATS.imp.id,
    header: "Scope",
    question: "Should the seat list keep its alerts pinned, or put them behind one button?",
    options: [
      { label: "Pinned", description: "Always visible, takes space." },
      { label: "Behind a button", description: "One tap away." },
    ],
    allow_other: true,
    multi: false,
  },
};

type Bed = Parameters<Parameters<typeof test>[2]>[0]["bed"];

// Like bed.openSeat, with the flag in the address.
async function openTraced(bed: Bed, flag: string): Promise<void> {
  await bed.page.goto(`/${flag}`);
  await loadFonts(bed.page);
  await bed.page.getByRole("tab", { name: /this Mac/ }).click();
  await bed.page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
  await expect(bed.page.locator(".xterm-rows")).toBeVisible();
}

const trace = (bed: Bed) => bed.page.evaluate(() => (window as unknown as { __tapTrace?: () => TraceEvent[] }).__tapTrace?.() ?? null);
const kinds = (events: TraceEvent[]) => events.map((event) => event.type);

test("with no flag nothing is recorded and no bar shows", async ({ bed }) => {
  await openTraced(bed, "");
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).click();
  await expect.poll(() => bed.mock.answers).toHaveLength(1);
  expect(await trace(bed), "the page offers a record with no flag").toBeNull();
  await expect(bed.page.locator("[data-taptrace]")).toHaveCount(0);
});

test("one tap on a daemon card's option leaves an ordered record", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await openTraced(bed, "?taptrace=1");
  bed.mock.push(ask);
  await expect(card(bed.page)).toBeVisible();
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).tap();
  await expect.poll(() => bed.mock.answers).toHaveLength(1);
  await expect.poll(async () => kinds((await trace(bed))!), "the card never left").toContain("card-gone");
  const events = (await trace(bed))!;
  // Everything before the card is the walk to the seat. The tap is what follows it.
  const tap = events.slice(events.findIndex((event) => event.type === "card-shown"));
  const order = kinds(tap);
  for (const wanted of ["touchstart", "pointerdown", "touchend", "mousedown", "mouseup", "click", "answer", "card-gone"]) {
    expect(order, `no ${wanted} in ${order.join(" ")}`).toContain(wanted);
  }
  expect(order.indexOf("touchstart")).toBeLessThan(order.indexOf("touchend"));
  expect(order.indexOf("mousedown")).toBeLessThan(order.indexOf("click"));
  expect(order.indexOf("click")).toBeLessThan(order.indexOf("answer"));
  expect(events.map((event) => event.t), "times go backwards").toEqual([...events.map((event) => event.t)].sort((a, b) => a - b));
  const click = tap.find((event) => event.type === "click")!;
  expect(click.target).toMatch(/button\.option\[1\]$/);
  expect(click.renderer).toBe("daemon-card");
  expect(events.at(-1)!.renderer, "after the answer the terminal shows").toBe("terminal");
});

test("a three-question submit records each step and the card's renderer", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await openTraced(bed, "?taptrace=1");
  const dialog = new ClaudeAsk(MULTI);
  bed.mock.setScene(SEATS.imp.id, dialog);
  const pane = card(bed.page);
  await expect(pane).toBeVisible();
  await pane.getByRole("button", { name: /Teal/ }).tap();
  await expect(pane.getByRole("heading", { name: /routes/ })).toBeVisible();
  await pane.getByRole("button", { name: /Qwen/ }).tap();
  await pane.getByRole("button", { name: /^Next$/ }).tap();
  await expect(pane.getByRole("heading", { name: /name/ })).toBeVisible();
  await pane.getByRole("button", { name: /Fern/ }).tap();
  await expect(pane.getByRole("button", { name: /Submit answers/ })).toBeVisible();
  await pane.getByRole("button", { name: /Submit answers/ }).tap();
  await expect.poll(() => dialog.phase).toBe("done");
  const events = (await trace(bed))!;
  expect(events.filter((event) => event.type === "click").length, "one click per tap").toBeGreaterThanOrEqual(5);
  expect(events.filter((event) => event.type === "answer").length).toBe(4);
  expect(events.some((event) => event.type === "screen" && event.detail === "menu")).toBe(true);
  const onCard = events.filter((event) => event.type === "click" && /button\.(option|button)/.test(event.target));
  expect(onCard.length).toBe(5);
  expect(new Set(onCard.map((event) => event.renderer))).toEqual(new Set(["screen-card"]));
});

test("the record holds no seat words, address or token, and one tap copies it", async ({ bed }) => {
  test.skip(kindOf(test.info()) !== "phone", "touch");
  await bed.page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", { value: { writeText: async (text: string) => ((window as unknown as { __copied: string }).__copied = text) } });
  });
  await openTraced(bed, "?taptrace=1");
  bed.mock.push(ask);
  await card(bed.page).getByRole("button", { name: /Behind a button/ }).tap();
  await expect.poll(() => bed.mock.answers).toHaveLength(1);
  await bed.page.getByRole("button", { name: /Copy trace/ }).tap();
  await expect(bed.page.getByRole("button", { name: "Trace copied" })).toBeVisible();
  const copied = await bed.page.evaluate(() => (window as unknown as { __copied: string }).__copied);
  expect(copied).toContain("taptrace v1");
  expect(copied).toMatch(/click \S*button\.option\[1\] @/);
  const host = new URL(bed.mock.url).host;
  for (const banned of ["Behind a button", "Pinned", "alerts pinned", "trace-1", host, "127.0.0.1", "token", "Bearer", SEATS.imp.name, SEATS.imp.id]) {
    expect(copied, `the record holds ${banned}`).not.toContain(banned);
  }
  expect(copied.length).toBeLessThanOrEqual(7000);
});
