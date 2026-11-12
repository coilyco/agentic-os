// COI-3434: Feedback works on a real host, and says "newer" only of an older one.
import { expect, kindOf, SEATS, test } from "../harness.ts";
import type { Page } from "@playwright/test";

const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
const opener = /Seats, starting a seat, hosts and settings/;

// A phone keeps the entry in its menu. Where Send can work it also has the bar's icon.
async function entry(page: Page, phone: boolean) {
  if (phone) await page.getByRole("button", { name: opener }).click();
  return page.getByRole("button", { name: /Send feedback/ }).last();
}

test.describe("a current host that keeps feedback", () => {
  test.use({ mockOptions: { feedback: true, version: "aos-v0.578.0" } });

  test("sends a note with a screenshot and names the record the host kept", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    await (await entry(bed.page, kindOf(info) === "phone")).click();
    await bed.page.getByLabel("What should change on this screen?").fill("the send button is cut off");
    await bed.page.locator('dialog input[type="file"]').setInputFiles({ name: "shot.png", mimeType: "image/png", buffer: PIXEL });
    await expect(bed.page.getByRole("button", { name: "Remove screenshot 1" })).toBeVisible();
    await bed.page.locator("dialog").getByRole("button", { name: "Send", exact: true }).click();

    await expect(bed.page.locator("dialog code.mono")).toHaveText(/^2026-11-11-0000-0001$/);
    expect(bed.mock.feedbacks, "the host kept one report").toHaveLength(1);
    const kept = bed.mock.feedbacks[0]!;
    expect(kept.text).toBe("the send button is cut off");
    expect(kept.images[0]!.equals(PIXEL), "the screenshot's own bytes").toBe(true);
    expect(Object.keys(kept.screen), "the screen is under its own key, with the fields the daemon reads").toEqual(expect.arrayContaining(["view", "host", "viewport", "build", "app", "at"]));
    expect(bed.mock.feedbackFrames).toEqual(["feedback_image", "feedback"]);
  });

  test("a refused note keeps the screenshot sent, and the retry sends only the closing frame", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    await (await entry(bed.page, kindOf(info) === "phone")).click();
    await bed.page.getByLabel("What should change on this screen?").fill("again");
    await bed.page.locator('dialog input[type="file"]').setInputFiles({ name: "shot.png", mimeType: "image/png", buffer: PIXEL });
    bed.mock.refuseFeedbackOnce("session_descendant");
    await bed.page.locator("dialog").getByRole("button", { name: "Send", exact: true }).click();
    await expect(bed.page.getByRole("alert")).toContainText("does not take feedback from inside a seat");
    await bed.page.locator("dialog").getByRole("button", { name: "Send", exact: true }).click();
    await expect(bed.page.locator("dialog code.mono")).toBeVisible();
    expect(bed.mock.feedbackFrames, "the picture went up once").toEqual(["feedback_image", "feedback", "feedback"]);
  });
});

test.describe("a host that is really older", () => {
  test.use({ mockOptions: { version: "aos-v0.517.0" } });

  test("the entry says it needs a newer Treehouse, and the sheet cannot send", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    const phone = kindOf(info) === "phone";
    if (phone) await bed.page.getByRole("button", { name: opener }).click();
    await expect(bed.page.getByText("This host needs a newer Treehouse.").locator("visible=true")).toHaveCount(1);
    await expect(bed.page.getByRole("button", { name: "Send feedback", exact: true }), "no bar icon where Send cannot work").toHaveCount(0);
  });
});

test.describe("a current host with nowhere to keep feedback", () => {
  test.use({ mockOptions: { version: "aos-v0.578.0" } });

  test("does not tell the person to update anything", async ({ bed }, info) => {
    await bed.openSeat(SEATS.imp);
    if (kindOf(info) === "phone") await bed.page.getByRole("button", { name: opener }).click();
    await expect(bed.page.getByText("This host has nowhere to keep feedback.").locator("visible=true")).toHaveCount(1);
    await expect(bed.page.getByRole("button", { name: /Send feedback/ }).last(), "the entry does not say to update").not.toContainText(/newer Treehouse/);
  });
});
