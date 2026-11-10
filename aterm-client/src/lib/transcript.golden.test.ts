// The replies the daemon returns for its own fixtures, run through the client's decoder.
// The files belong to the daemon's tests. This skips where they are not checked out.
import { existsSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { decodeReply, rowsFor, withoutPeers } from "./transcript";

const dir = process.env.ATERM_TRANSCRIPT_GOLDEN ?? new URL("../../../aterm/testdata/transcript/", import.meta.url).pathname;

describe.skipIf(!existsSync(`${dir}claude.golden.json`))("the daemon's golden replies", () => {
  for (const reader of ["claude", "codex", "opencode"]) {
    const raw = JSON.parse(readFileSync(`${dir}${reader}.golden.json`, "utf8")) as { items: Record<string, unknown>[] };
    const reply = decodeReply("s", raw as unknown as Record<string, unknown>);

    it(`${reader}: every item decodes, in the order sent`, () => {
      expect(reply.reader).toBe(reader);
      expect(reply.items.map((item) => item.id)).toEqual(raw.items.map((item) => item.id));
    });

    it(`${reader}: a tool step has its class and summary, and an edit its counts`, () => {
      const tools = reply.items.filter((item) => item.kind === "tool");
      expect(tools.length).toBeGreaterThan(1);
      for (const tool of tools) expect(tool.tool?.summary, tool.id).toBeTruthy();
      expect(tools.some((tool) => tool.tool?.ok === false), "a failed call").toBe(true);
      expect(tools.some((tool) => tool.tool?.class === "edit" && tool.tool.diff), "an edit with a diffstat").toBe(true);
    });

    it(`${reader}: no level shows a peer message or a reply tagged to it, and Kai's prompts stay`, () => {
      const peers = reply.items.filter((item) => item.kind === "peer");
      expect(peers.length, "a peer in the fixture").toBeGreaterThan(0);
      for (const peer of peers) expect(peer.from?.identity, peer.id).toBeTruthy();
      const prompts = reply.items.filter((item) => item.kind === "prompt").map((item) => item.id);
      const kept = withoutPeers(reply.items);
      expect(kept.some((item) => item.kind === "peer")).toBe(false);
      expect(kept.filter((item) => item.kind === "prompt").map((item) => item.id)).toEqual(prompts);
      const unshared = reply.items.filter((item) => item.for !== undefined && !item.shared);
      for (const item of unshared) expect(kept.map((each) => each.id), item.id).not.toContain(item.id);
      for (const level of ["focus", "default", "verbose"] as const) {
        const text = rowsFor(reply.items, level).flatMap((row) => (row.kind === "tool" ? row.summary : row.text));
        for (const peer of peers) if (peer.text) expect(text.join("\n"), `${level} shows no peer text`).not.toContain(peer.text);
      }
    });

    it(`${reader}: a shared item stays and is marked, since it may be Kai's own work`, () => {
      const shared = reply.items.filter((item) => item.shared);
      const rows = rowsFor(reply.items, "verbose");
      for (const item of shared) {
        const row = rows.find((each) => each.key === item.id);
        if (row) expect(row.maybePeer, item.id).toBe(true);
      }
    });
  }
});
