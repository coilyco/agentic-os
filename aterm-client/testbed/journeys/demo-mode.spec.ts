// COI-3315: the first screen's "Try the demo" opens made-up seats with no network and no
// real shell, marks every screen, and Leave demo returns to "Pick a host".
import { expect, test } from "../harness.ts";

test("the demo is entered from the first screen, marked, and left again", async ({ bed }) => {
  const dialled: string[] = [];
  bed.page.on("websocket", (socket) => dialled.push(socket.url()));
  await bed.page.goto("/");
  await expect(bed.page.getByRole("button", { name: "Try the demo" })).toBeVisible();
  await bed.step("first-screen", { compare: false });

  await bed.page.getByRole("button", { name: "Try the demo" }).click();
  const banner = bed.page.getByRole("region", { name: "Demo mode" });
  await expect(banner).toBeVisible();
  await expect(bed.page).toHaveTitle(/^DEMO \/\//);
  await bed.step("first-seat", { compare: false });
  // The test bed seeds a real host the page probes at launch. The demo adds nothing.
  const before = dialled.length;
  await bed.page.waitForTimeout(3000);
  expect(dialled.length, `the demo dialled ${dialled.slice(before).join(", ")}`).toBe(before);

  await bed.page.getByRole("button", { name: "Leave demo" }).click();
  await expect(bed.page.getByRole("heading", { name: "Pick a host" })).toBeVisible();
  await expect(banner).toBeHidden();
});
