// COI-3495: a Messages row sent by this seat shows this seat's own creature.
// A row's target is a session name and the art is keyed by role slug, so a
// lookup by target found nothing and drew an empty square.
import { expect, kindOf, SEATS, test, type Bed } from "../harness.ts";

const message = (id: string, from: string, target: string) => ({
  type: "message",
  message: { id, from, target, session: SEATS.imp.id, state: "delivered", text: "Run the gate." },
});

async function openMessages(bed: Bed, desktop: boolean) {
  const { page } = bed;
  if (desktop) {
    const toggle = page.getByRole("button", { name: /side panel/i }).first();
    if (await toggle.isVisible()) await toggle.click();
  }
  await page.getByRole("tab", { name: /^Messages/ }).click();
}

test("a row sent by this seat shows this seat's creature, not an empty square", async ({ bed }, info) => {
  await bed.openSeat(SEATS.imp);
  bed.mock.push(message("peer-1", "eng-platform Beetle-Ox", "frontend-eng"));
  bed.mock.push(message("self-1", "frontend-eng Imp-Dragonfly", "sysadmin-senior-turtle-ox"));
  await openMessages(bed, kindOf(info) === "desktop");

  for (const [name, who] of [[/Beetle-Ox/, "a peer's row"], [/^this seat/, "this seat's own row"]] as const) {
    const row = bed.page.getByRole("button", { name });
    await expect(row, who).toHaveCount(1);
    const art = row.locator("img.creature");
    await expect(art, `${who} shows creature art`).toHaveCount(1);
    expect(await art.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0), `${who} art loaded`).toBe(true);
  }
  // Compared by eye in the report, not against a baseline, until a Linux baseline exists.
  await bed.step("rows", { compare: false });
});
