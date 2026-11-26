// COI-3437: a host added by its bare tailnet name is dialled on 443 (Tailscale Serve),
// and a name with a port keeps the port it was typed with.
import { expect, test } from "../harness.ts";

const NAME = "mac.example.ts.net";

// The dial, not the "Asking the daemon at" line: a name that never resolves fails in
// about 10 ms, so that line is often gone before a locator sees it.
async function add(page: import("@playwright/test").Page, input: string): Promise<() => string[]> {
  const dialled: string[] = [];
  page.on("websocket", (socket) => dialled.push(socket.url()));
  await page.goto("/");
  await page.getByLabel("Add a host by its tailnet name").fill(input);
  await page.getByRole("button", { name: "Add host" }).click();
  const toName = () => dialled.filter((url) => url.includes(NAME));
  await expect.poll(() => toName().length, { message: "the added host is dialled" }).toBeGreaterThan(0);
  return toName;
}

test("a bare tailnet name is dialled on 443", async ({ bed }) => {
  const dialled = await add(bed.page, NAME);
  expect(new Set(dialled()), `dialled ${dialled().join(", ")}`).toEqual(new Set([`wss://${NAME}/`]));
  const stored = await bed.page.evaluate(() => localStorage.getItem("aterm.hosts.v1"));
  expect(JSON.parse(stored ?? "[]")).toEqual([{ label: "mac", address: `wss://${NAME}/` }]);
  await bed.step("bare-name", { compare: false });
});

test("a name typed with a port keeps that port", async ({ bed }) => {
  const dialled = await add(bed.page, `${NAME}:7419`);
  expect(new Set(dialled()), `dialled ${dialled().join(", ")}`).toEqual(new Set([`wss://${NAME}:7419/`]));
  const stored = await bed.page.evaluate(() => localStorage.getItem("aterm.hosts.v1"));
  expect(JSON.parse(stored ?? "[]")).toEqual([{ label: "mac", address: `wss://${NAME}:7419/` }]);
});
