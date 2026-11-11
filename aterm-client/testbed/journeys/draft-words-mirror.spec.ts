// COI-3619: the WebView commits local storage late, so a force-stop soon after typing can
// lose words. A second copy in IndexedDB brings them back. These journeys lose the first.
import { expect, SEATS, test, type Bed, seatEntry } from "../harness.ts";

const field = (bed: Bed) => bed.page.getByRole("textbox", { name: /^Message / });
const STORED = "aterm.drafts.v1";
const reopen = async (bed: Bed) => {
  await bed.page.reload();
  await seatEntry(bed.page, SEATS.imp.name).click();
};
// A read waits behind the put, so a copy it finds is one that has committed.
const mirrored = (bed: Bed, key: string) =>
  bed.page.evaluate(
    (id) =>
      new Promise<string | undefined>((done) => {
        const open = indexedDB.open("aterm.draft-words.v1", 1);
        open.onsuccess = () => {
          const get = open.result.transaction("words").objectStore("words").get(id);
          get.onsuccess = () => done(get.result?.text);
          get.onerror = () => done(undefined);
        };
        open.onerror = () => done(undefined);
      }),
    key,
  );

test("words come back from the mirror when local storage never committed them", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("Ask Beetle-Ox whether the gate is green");
  await expect.poll(() => mirrored(bed, `local/${SEATS.imp.id}`), "the mirror holds the words").toBe("Ask Beetle-Ox whether the gate is green");
  // What a force-stop leaves behind when the WebView had not yet written local storage.
  await bed.page.evaluate((key) => localStorage.removeItem(key), STORED);
  await reopen(bed);
  await expect(field(bed), "the words are back from the mirror").toHaveValue("Ask Beetle-Ox whether the gate is green");
});

test("an older mirror never replaces newer words in local storage", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("first words");
  await expect.poll(() => mirrored(bed, `local/${SEATS.imp.id}`)).toBe("first words");
  // Local storage moves on while the mirror stays behind, as if its put never landed.
  await bed.page.evaluate(
    ([key, id]) => localStorage.setItem(key, JSON.stringify({ [id]: { text: "newer words", at: Date.now() + 60_000 } })),
    [STORED, `local/${SEATS.imp.id}`] as const,
  );
  await reopen(bed);
  await expect(field(bed)).toHaveValue("newer words");
});

test("an emptied box stays empty, so the mirror does not bring old words back", async ({ bed }) => {
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("to be cleared");
  await expect.poll(() => mirrored(bed, `local/${SEATS.imp.id}`)).toBe("to be cleared");
  await field(bed).fill("");
  await expect.poll(() => mirrored(bed, `local/${SEATS.imp.id}`), "the copy goes with the words").toBeUndefined();
  await reopen(bed);
  await expect(field(bed)).toHaveValue("");
});

test("a mirror delete cut short by a reload does not bring emptied words back", async ({ bed }) => {
  const key = `local/${SEATS.imp.id}`;
  await bed.openSeat(SEATS.imp);
  await field(bed).fill("to be cleared");
  await expect.poll(() => mirrored(bed, key)).toBe("to be cleared");
  await field(bed).fill("");
  await expect.poll(() => mirrored(bed, key)).toBeUndefined();
  // Put the old copy back, as if the reload had ended the delete before it committed.
  await bed.page.evaluate(
    ([id]) =>
      new Promise<void>((done) => {
        const open = indexedDB.open("aterm.draft-words.v1", 1);
        open.onsuccess = () => {
          const tx = open.result.transaction("words", "readwrite");
          tx.objectStore("words").put({ key: id, text: "to be cleared", at: Date.now() - 1000 });
          tx.oncomplete = () => done();
        };
      }),
    [key] as const,
  );
  await reopen(bed);
  await expect(field(bed), "stale words do not come back over an emptied box").toHaveValue("");
});
