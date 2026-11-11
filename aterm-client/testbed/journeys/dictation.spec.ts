// COI-3460: the composer's mic. The plugin is faked at `window.__TAURI__`, and `poll`
// answers from a script like the phone's recognizer.
import { expect, SEATS, test } from "../harness.ts";

type Bed = Parameters<Parameters<typeof test>[2]>[0]["bed"];
const sent = (bed: Bed) => bed.mock.typed(SEATS.imp.id).join("");

/** The app with a recognizer: `available`, then one scripted session per `start`. */
async function withRecognizer(page: import("@playwright/test").Page, script: object[], start?: { code: string; message: string }): Promise<void> {
  await page.addInitScript(
    ({ script, start }) => {
      let at = 0;
      let stopped = false;
      (window as unknown as { __TAURI__: object }).__TAURI__ = {
        core: {
          invoke: async (command: string) => {
            if (command === "plugin:dictation|available") return { available: true, permission: "prompt" };
            if (command === "plugin:dictation|start") {
              if (start) throw start;
              at = 0;
              stopped = false;
              return null;
            }
            if (command === "plugin:dictation|poll") {
              // After a stop the last scripted answer is the final one.
              const frame = stopped ? script.at(-1) : script[Math.min(at++, script.length - 2)];
              return frame;
            }
            if (command === "plugin:dictation|stop") {
              stopped = true;
              return null;
            }
            return null;
          },
        },
      };
    },
    { script, start },
  );
}

const frame = (phase: string, text: string, seq: number, error: number | null = null) => ({ phase, text, error, seq });

test("a tap on the mic puts spoken words in the composer, editable, and Send sends the edit", async ({ bed }) => {
  await withRecognizer(bed.page, [frame("listening", "", 1), frame("listening", "run the", 2), frame("listening", "run the gate", 3), frame("done", "run the gate please", 4)]);
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await field.fill("seat:");
  const mic = bed.page.getByRole("button", { name: "Dictate" });
  await expect(mic).toBeVisible();
  await mic.click();
  await expect(bed.page.getByRole("button", { name: "Stop dictating" })).toHaveAttribute("aria-pressed", "true");
  // Words show while the person is still talking, after what was already typed.
  await expect(field).toHaveValue("seat: run the gate");
  await bed.page.getByRole("button", { name: "Stop dictating" }).click();
  await expect(field).toHaveValue("seat: run the gate please");
  await expect(mic).toHaveAttribute("aria-pressed", "false");
  expect(sent(bed), "nothing reaches the seat before Send").toBe("");
  await field.fill("seat: run the gate now");
  await bed.page.locator("form.composer button.primary").click();
  await expect.poll(() => sent(bed)).toContain("seat: run the gate now");
});

test("typing while it listens keeps the keyboard's text and drops later speech", async ({ bed }) => {
  await withRecognizer(bed.page, [frame("listening", "", 1), frame("listening", "first words", 2), frame("done", "first words and more", 3)]);
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await bed.page.getByRole("button", { name: "Dictate" }).click();
  await expect(field).toHaveValue("first words");
  await field.focus();
  await bed.page.keyboard.insertText("!");
  await expect(bed.page.getByRole("button", { name: "Dictate" })).toBeVisible();
  await expect(field).toHaveValue("first words!");
});

test("a refused microphone says so in words and leaves the draft alone", async ({ bed }) => {
  await withRecognizer(bed.page, [frame("idle", "", 0)], { code: "denied", message: "no mic" });
  await bed.openSeat(SEATS.imp);
  const field = bed.page.locator("#composer-input");
  await field.fill("keep me");
  await bed.page.getByRole("button", { name: "Dictate" }).click();
  await expect(bed.page.getByRole("alert")).toContainText("The microphone is not allowed for this app");
  await expect(field).toHaveValue("keep me");
});

test("a browser has no mic button", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await expect(bed.page.locator("#composer-input")).toBeVisible();
  await expect(bed.page.getByRole("button", { name: "Dictate" })).toHaveCount(0);
});
