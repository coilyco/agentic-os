// COI-3363: `just aterm-client-scale` prints a JSON line per run.
import { test } from "@playwright/test";
import { longHistory, observeLive, observeLiveSeats, runOnce } from "./measure.ts";

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

test("the open seat after a long run, by how much history it holds", async ({ browser }) => {
  test.skip(!process.env.SCALE_HISTORY, "set SCALE_HISTORY to the item counts, such as 200,2000,8000");
  test.setTimeout(0);
  for (const history of process.env.SCALE_HISTORY!.split(",").map(Number)) {
    const earlier = Math.ceil(history / 50);
    console.log("SCALE " + JSON.stringify(await longHistory(browser, { seats: 10, history, earlier, windows: 3 })));
  }
});

test("a real daemon's seats through a client build of your choice, read only", async ({ browser }) => {
  test.skip(!process.env.SCALE_LIVE_DIST, "set SCALE_LIVE and SCALE_LIVE_DIST to a daemon page and a built dist");
  test.setTimeout(0);
  const options = { pages: Number(process.env.SCALE_PAGES ?? 40), watchMs: Number(process.env.SCALE_WATCH_MS ?? 15_000), skip: process.env.SCALE_SKIP ?? "sprite-dragonfly" };
  console.log("SCALE " + JSON.stringify(await observeLiveSeats(process.env.SCALE_LIVE ?? "http://127.0.0.1:7419/", process.env.SCALE_LIVE_DIST!, browser, options)));
});
