// COI-3752: in the app, Notify when closed registers the push plugin's token with the
// broker, and off drops it. The Treehouse build names accounts and a broker that never
// resolve, so this journey plays both, and the plugin, itself.
import { expect, test as base, type Page, type Route } from "@playwright/test";
import { kindOf } from "../harness.ts";
import { startMock, type Mock } from "../mock-daemon.ts";

const ACCOUNTS = "https://accounts.testbed.invalid";
const BROKER = "https://broker.testbed.invalid";
const TOKEN = "fcm-testbed:token_1";

type Sent = { path: string; body: unknown };

const test = base.extend<{ mock: Mock }>({
  mock: async ({}, use) => {
    const mock = await startMock({ dist: "dist-treehouse" });
    await use(mock);
    await mock.stop();
  },
  baseURL: async ({ mock }, use) => {
    await use(mock.url);
  },
});

const cors = { "access-control-allow-origin": "*", "access-control-allow-headers": "authorization, content-type", "access-control-allow-methods": "GET, POST, OPTIONS" };

/** The push and notification plugins as the page sees them. `register` may reject. */
async function plugin(page: Page, register: { code: string } | null = null): Promise<void> {
  await page.addInitScript((refusal) => {
    let made: string | null = null;
    (window as unknown as { __TAURI__: object }).__TAURI__ = {
      core: {
        invoke: async (command: string) => {
          if (command === "plugin:notification|is_permission_granted") return true;
          if (command === "plugin:push|register") {
            if (refusal) throw { code: refusal.code, message: "refused" };
            made = "fcm-testbed:token_1";
            return { token: made, device: "Pixel 8" };
          }
          if (command === "plugin:push|token") return made;
          return null;
        },
        addPluginListener: async () => ({ unregister: async () => {} }),
      },
    };
  }, register);
}

/** Accounts' page sends the hop straight back, and the broker records what reaches it. */
async function services(page: Page): Promise<Sent[]> {
  const sent: Sent[] = [];
  await page.route(`${ACCOUNTS}/**`, async (route: Route) => {
    const url = new URL(route.request().url());
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (url.pathname === "/signin") {
      const back = new URL(url.searchParams.get("redirect_uri")!);
      back.search = new URLSearchParams({ code: "code-1", state: url.searchParams.get("state")! }).toString();
      // A page that sends the browser back, since WebKit will not fulfill a redirect.
      return route.fulfill({ status: 200, contentType: "text/html", body: `<script>location.replace(${JSON.stringify(back.toString())})</script>` });
    }
    if (url.pathname === "/v1/auth/handoff") {
      return route.fulfill({ status: 200, headers: cors, json: { access_token: "acc-1", expires_in: 900, refresh_token: "ref-1", account_id: "acct-1", name: "Tester" } });
    }
    return route.fulfill({ status: 404, headers: cors, json: { error: "not_found", message: "not here" } });
  });
  await page.route(`${BROKER}/**`, async (route: Route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (path === "/v1/daemons") return route.fulfill({ status: 200, headers: cors, json: { daemons: [] } });
    sent.push({ path, body: request.postDataJSON() });
    return route.fulfill({ status: 204, headers: cors });
  });
  return sent;
}

async function openSettings(page: Page): Promise<import("@playwright/test").Locator> {
  await page.getByRole("tab", { name: /this Mac/ }).click();
  await page.getByRole("region", { name: "Seats", exact: true }).getByRole("button", { name: "Settings", exact: true }).click();
  const settings = page.getByRole("region", { name: "Settings" });
  await expect(settings).toBeVisible();
  return settings;
}

const signIn = async (page: Page) => {
  await page.getByRole("button", { name: "Sign in with a passkey" }).click();
  await expect(page.getByText("Signed in as Tester.")).toBeVisible();
};

test("signed out, the switch says to sign in and cannot turn on", async ({ page }, info) => {
  await plugin(page);
  const sent = await services(page);
  await page.goto("/");
  const settings = await openSettings(page);
  const toggle = settings.getByRole("switch", { name: "Notify when closed" });
  await expect(toggle).toBeDisabled();
  await expect(settings.locator("#alerts-push-note")).toHaveText("Sign in to your Treehouse account to turn this on.");
  await page.screenshot({ path: info.outputPath("signed-out.png") });
  expect(sent).toEqual([]);
});

test("on registers the token with the broker, and off drops it", async ({ page }, info) => {
  await plugin(page);
  const sent = await services(page);
  await page.goto("/");
  await signIn(page);
  const settings = await openSettings(page);
  const toggle = settings.getByRole("switch", { name: "Notify when closed" });
  await expect(toggle).toBeEnabled();
  await expect(toggle).not.toBeChecked();

  await toggle.click();
  await expect(toggle).toBeChecked();
  await expect.poll(() => sent).toEqual([{ path: "/v1/push/apps", body: { platform: "android", token: TOKEN, device: "Pixel 8" } }]);
  await page.screenshot({ path: info.outputPath("on.png") });

  await toggle.click();
  await expect(toggle).not.toBeChecked();
  await expect.poll(() => sent.at(-1)).toEqual({ path: "/v1/push/apps/unregister", body: { token: TOKEN } });
  await expect.poll(() => page.evaluate(() => localStorage.getItem("aterm.app-push.v1")), { message: "off is kept on this device" }).toBeNull();
  expect(sent).toHaveLength(2);
});

test("a build with no push says plainly it is not set up, and the switch stays off", async ({ page }, info) => {
  await plugin(page, { code: "push_off" });
  const sent = await services(page);
  await page.goto("/");
  await signIn(page);
  const settings = await openSettings(page);
  const toggle = settings.getByRole("switch", { name: "Notify when closed" });
  await toggle.click();
  const note = settings.getByRole("alert");
  // A phone build without the server's config, or the Mac app, which has no push service.
  await expect(note).toHaveText(
    kindOf(info) === "desktop"
      ? "This app cannot show notifications while it is closed. Notifications while it is open still work."
      : /not set up on this server\. Notifications while the app is open still work\.|cannot show notifications while it is closed/,
  );
  await expect(toggle).not.toBeChecked();
  await page.screenshot({ path: info.outputPath("not-set-up.png") });
  expect(sent).toEqual([]);
});
