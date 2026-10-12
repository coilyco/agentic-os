---
name: tooling-verify-for-the-human
description: Before telling a human their ask is done, split their literal words into every item they named and reproduce their own observation per item. Triggers - verify for the human, ask is done, both, all, every, plush, acceptance check, reply done.
---

# Verify against the requester's words

A green check proves what it tested. When the check was written in a seat's
terms (a folder name, a label, a record's acceptance line), it proves the
seat's restatement of the ask and says nothing about the ask. COI-2683 is the
case: Kai asked for both Alpacacorn packs and every check passed with one.

**Applies to** any seat or the director, at the moment of telling a human
their ask is done, fixed, live, or listed.

## The rule

1. **Quote the ask verbatim**, then split it into every item it names. Each
   pack, page, surface, count, and search word is an item.
2. **A plural, "both", "all", "every", a number, or a nickname is an item set,
   not one item.** Resolve its size against the outside source (the author's
   mod.io listing, the vendor page, the upstream registry), never against the
   repo's own docs or the seat's own tracker. The docs are what was wrong.
3. **Reproduce the human's own observation per item.** The same search term,
   the same page, the same surface they would use. A relayed claim from another
   seat is not a reproduction. Mark it `relayed` and it fails the line.
4. **Report one line per item**, then one count line:

   ```
   <item> | <exact check> | <raw output, pasted>
   named <N>, resolved <N> against <outside source>
   ```

5. **A line that fails, returns zero, or was not run means not done.** Report
   the failing line as it is. Never drop it, and never reword the item until
   it passes.

## Replay of the COI-2683 case

State before eco-ops #158 (`900ec44`), ask: "both Alpacacorn packs on the
cycle 15 server and on the website", search word "plush". Raw outputs are as
recorded on COI-2683, since #158 has since fixed the state.

* `both packs` - author's mod.io listing, count Alpacacorn item packs - 2 (pack 2 is mod.io 5091180)
* `pack 1 on server` - list the server Mods dir for `AlpacacornItemPack` - present
* `pack 2 on server` - list the server Mods dir for `AlpacacornItemPackClothDecorations` - **no such folder, FAILS**
* `"plush" on /mods` - search /mods for `plush` - **0 hits, FAILS** (the summary read "Decorative and practical furniture and items")
* count line: named 2 packs, resolved 2 against mod.io, 1 on server

COI-2672's acceptance, "`get_mods` lists `AlpacacornItemPack`", passes this
state. Under the rule it is the `pack 1 on server` line only, and the two
failing lines above stop the report.
