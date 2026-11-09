// COI-3437: a host added by its bare tailnet name is dialled on 443 (Tailscale Serve),
// and a name with a port keeps the port it was typed with.
import { expect, test } from "../harness.ts";

const NAME = "mac.example.ts.net";

async function add(page: import("@playwright/test").Page, input: string): Promise<void> {
  await page.goto("/");
  await page.getByLabel("Add a host by its tailnet name").fill(input);
  await page.getByRole("button", { name: "Add host" }).click();
}

test("a bare tailnet name is dialled on 443", async ({ bed }) => {
  const dialled: string[] = [];
  bed.page.on("websocket", (socket) => dialled.push(socket.url()));
  await add(bed.page, NAME);
  await expect(bed.page.getByRole("status").filter({ hasText: `wss://${NAME}/` })).toBeVisible();
  expect(dialled.filter((url) => url.includes(NAME)).every((url) => url === `wss://${NAME}/`), `dialled ${dialled.join(", ")}`).toBe(true);
  const stored = await bed.page.evaluate(() => localStorage.getItem("aterm.hosts.v1"));
  expect(JSON.parse(stored ?? "[]")).toEqual([{ label: "mac", address: `wss://${NAME}/` }]);
  await bed.step("bare-name", { compare: false });
});

test("a name typed with a port keeps that port", async ({ bed }) => {
  await add(bed.page, `${NAME}:7419`);
  await expect(bed.page.getByRole("status").filter({ hasText: `wss://${NAME}:7419/` })).toBeVisible();
  const stored = await bed.page.evaluate(() => localStorage.getItem("aterm.hosts.v1"));
  expect(JSON.parse(stored ?? "[]")).toEqual([{ label: "mac", address: `wss://${NAME}:7419/` }]);
});
