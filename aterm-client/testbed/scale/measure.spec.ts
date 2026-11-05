// COI-3363: `just aterm-client-scale` prints a JSON line per run.
import { test } from "@playwright/test";
import { observeLive, runOnce } from "./measure.ts";

test("measure the client at growing seat counts", async ({ browser }) => {
  test.setTimeout(0);
  const counts = (process.env.SCALE_COUNTS ?? "25,100").split(",").map(Number);
  const runs = Number(process.env.SCALE_RUNS ?? 1);
  for (const count of counts) {
    for (let run = 1; run <= runs; run++) console.log("SCALE " + JSON.stringify({ run, ...(await runOnce(count, browser)) }));
  }
});

test("observe the real client on a real daemon, read only", async ({ browser }) => {
  test.skip(!process.env.SCALE_LIVE, "set SCALE_LIVE to a daemon page, such as http://127.0.0.1:7419/");
  test.setTimeout(0);
  console.log("SCALE " + JSON.stringify(await observeLive(process.env.SCALE_LIVE!, browser)));
});
