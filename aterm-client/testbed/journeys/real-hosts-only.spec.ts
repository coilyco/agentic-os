// COI-3743: a fresh client lists only real hosts. The shipped build has no Demo host,
// no "Try the demo", and no dev Mock host, and the test bed's daemon is the one row.
import { expect, test } from "../harness.ts";

test("a fresh client lists only the real host, with no demo or mock row", async ({ bed }, info) => {
  await bed.page.goto("/");
  await expect(bed.page.getByRole("tab", { name: /this Mac/ })).toBeVisible();
  await expect(bed.page.getByRole("button", { name: "Try the demo" })).toHaveCount(0);
  await expect(bed.page.getByRole("tab", { name: /Demo host|Mock host/ })).toHaveCount(0);
  await expect(bed.page.getByText(/scripted, (no daemon|dev only)/)).toHaveCount(0);
  await expect(bed.page).not.toHaveTitle(/^DEMO/);
  await bed.page.screenshot({ path: info.outputPath("first-screen.png") });
});
