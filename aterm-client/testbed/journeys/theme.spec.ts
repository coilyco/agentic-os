// COI-3554: light, dark or the device's, chosen in Settings and kept on this device.
import { expect, loadFonts, SEATS, test } from "../harness.ts";

const esc = (code: string, text: string) => `\x1b[${code}m${text}\x1b[0m`;
const ramp = (from: number) => [0, 1, 2, 3, 4, 5, 6, 7].map((n) => esc(`${from + n}`, "███")).join(" ");
const screen = [
  `${esc("1;36", "●")} ${esc("1", "Update(aterm-client/src/lib/choices.ts)")}`,
  `    ${esc("31", "-  const next = rows.find(isNext);")}`,
  `    ${esc("32", "+  const next = rows.findLast(isNext);")}`,
  `  ${esc("32", "✓")} 412 tests passed  ${esc("33", "⚠")} one warning  ${esc("31", "✗")} one failed`,
  `${esc("35", "◆")} ${esc("36", "https://github.com/coilyco/agentic-os/pull/346")}  ${esc("90", "dim note")}`,
  `  normal  ${ramp(30)}`,
  `  bright  ${ramp(90)}`,
];

const LIGHT_GROUND = "rgb(243, 235, 225)";
const DARK_GROUND = "rgb(31, 19, 7)";
const LIGHT_TERMINAL = "rgb(251, 247, 241)";
const DARK_TERMINAL = "rgb(39, 26, 12)";

type Page = import("@playwright/test").Page;
const theme = (page: Page) => page.locator("html").getAttribute("data-theme");
const ground = (page: Page) => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
const seatsHome = (page: Page) => page.getByRole("region", { name: "Seats", exact: true });

test("Settings picks the theme, System follows the device, and the pick survives a reload", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  expect(await theme(page), "System on a light device").toBe("light");
  expect(await ground(page)).toBe(LIGHT_GROUND);

  await seatsHome(page).getByRole("button", { name: "Settings", exact: true }).click();
  const settings = page.getByRole("region", { name: "Settings" });
  const group = settings.getByRole("radiogroup", { name: "Theme" });
  await expect(group.getByRole("radio", { name: "System" })).toBeChecked();
  await expect(settings).toContainText("System follows the device, which is light now");

  await page.emulateMedia({ colorScheme: "dark" });
  await expect.poll(() => theme(page), "System follows the device live").toBe("dark");
  expect(await ground(page)).toBe(DARK_GROUND);

  await group.getByRole("radio", { name: "Light" }).click();
  await expect.poll(() => theme(page)).toBe("light");
  expect(await page.evaluate(() => localStorage.getItem("aterm.theme.v1"))).toBe("light");
  expect(await ground(page), "Light holds on a dark device").toBe(LIGHT_GROUND);

  await page.reload();
  expect(await theme(page), "set before the app mounts").toBe("light");
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await seatsHome(page).getByRole("button", { name: "Settings", exact: true }).click();
  await expect(settings.getByRole("radio", { name: "Light" })).toBeChecked();

  // Arrow keys move between the three as a radio group does.
  await settings.getByRole("radio", { name: "Light" }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(settings.getByRole("radio", { name: "Dark" })).toBeChecked();
  await expect.poll(() => theme(page)).toBe("dark");
});

test("the seats and a seat read in light, and an open terminal follows the device live", async ({ bed }) => {
  const page = bed.page;
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await loadFonts(page);
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await expect(seatsHome(page)).toBeVisible();
  await bed.step("seats-light");

  await bed.openSeat(SEATS.imp);
  bed.mock.setScene(SEATS.imp.id, { render: () => screen });
  await expect(bed.screen()).toContainText("bright");
  const paint = () => page.evaluate(() => getComputedStyle(document.querySelector(".xterm-scrollable-element")!).backgroundColor);
  expect(await paint(), "the terminal on paper").toBe(LIGHT_TERMINAL);
  await bed.step("seat-light");

  await page.emulateMedia({ colorScheme: "dark" });
  await expect.poll(paint, "the terminal repaints without a reload").toBe(DARK_TERMINAL);
});
