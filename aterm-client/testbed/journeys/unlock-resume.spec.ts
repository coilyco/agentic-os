// Journey: a locked phone wakes and redials (COI-3305). Inside the resume window the
// unlock holds, with no button and no prompt. After it the button and one prompt return.
import { expect, test as base } from "@playwright/test";
import { SEATS } from "../harness.ts";
import { startMock, type Mock } from "../mock-daemon.ts";

const test = base.extend<{ mock: Mock }>({
  mock: async ({}, use) => {
    const mock = await startMock({ lock: true });
    await use(mock);
    await mock.stop();
  },
  baseURL: async ({ mock }, use) => {
    await use(mock.url);
  },
});

test("an unlock survives a redial inside the window and ends after it", async ({ page, mock }, info) => {
  // The app's plugin, as a page sees it. Each sign is one fingerprint prompt.
  await page.addInitScript(() => {
    const w = window as unknown as { __TAURI__: unknown; prompts: number };
    w.prompts = 0;
    w.__TAURI__ = {
      core: {
        invoke: async (command: string) => {
          // Another plugin's call, like the share target's `take`, is not a prompt.
          if (!command.startsWith("plugin:devicekey|")) return null;
          if (command.endsWith("status")) return { enrolled: true, key_id: "k1", biometric: "ready" };
          w.prompts += 1;
          return { key_id: "k1", signature: "SIG" };
        },
      },
    };
  });
  const prompts = () => page.evaluate(() => (window as unknown as { prompts: number }).prompts);
  const shot = (name: string) => page.screenshot({ path: info.outputPath(`${name}.png`) });

  await page.goto("/");
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await page.getByRole("tab", { name: new RegExp(SEATS.imp.name) }).first().click();
  // The seat's own lock. The host page shows an Unlock section even when unlocked.
  const readOnly = page.getByText("Read only.").first();
  const unlock = page.getByRole("button", { name: "Unlock with fingerprint" });
  await expect(readOnly).toBeVisible();
  await expect(unlock).toBeVisible();
  await shot("1-locked");

  await unlock.click();
  await expect(readOnly).toBeHidden();
  expect(await prompts()).toBe(1);

  // The phone locks and wakes: the socket drops and the redial welcome says locked.
  mock.dropClients();
  await expect.poll(() => mock.resumes.length, { timeout: 10000 }).toBe(1);
  await expect(readOnly).toBeHidden();
  expect(await prompts()).toBe(1);
  await shot("2-resumed");

  // The window ends while the phone is away.
  mock.endResumeWindow();
  mock.dropClients();
  const ended = page.getByRole("status").filter({ hasText: "Your unlock ended while this device was away" });
  await expect(ended).toBeVisible({ timeout: 10000 });
  await expect(readOnly).toBeVisible();
  await expect(unlock).toBeVisible();
  expect(await prompts()).toBe(1);
  await shot("3-ended");

  await unlock.click();
  await expect(readOnly).toBeHidden();
  expect(await prompts()).toBe(2);
});
