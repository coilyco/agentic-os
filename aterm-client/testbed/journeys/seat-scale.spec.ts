// COI-3363: the client holds 100 seats. The testbed reference says how.
// A quadratic roster frame is guarded, without timing noise, by app-scale.test.ts.
import { expect, test } from "@playwright/test";
import { longTasksWhileUsed } from "../scale/measure.ts";

test.beforeEach(({}, info) => test.skip(info.project.name !== "desktop-1440", "one desktop size is enough, and it is slow"));

test("100 seats open, switching seats and typing stay under a 100 ms long task", async ({ browser }) => {
  test.setTimeout(120_000);
  const seen = await longTasksWhileUsed(100, browser);
  expect(seen.rows).toBe(100);
  expect(seen.over100, `longest task ${seen.longestMs} ms`).toBe(0);
});
