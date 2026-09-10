# Writing a guide

[`documentation-bands`](documentation-bands.md) carries the caps, the band
table and the cross-link rule. This page is the other half: which repos may
have a `guides/` shelf at all, and how to tell whether the thing in your head
is a guide or a reference page.

Getting it wrong is cheap in one direction and expensive in the other. A guide
filed as reference gets cut until the part that mattered is gone. A reference
page filed as a guide sits on a deliberately scarce shelf and crowds out
something that needed the room.

## Only a flagship repo has a shelf

**`guides/` is for the externally facing flagship repos and nowhere else.**
A guide is written for somebody outside the estate who is adopting the thing
and needs to be walked through it. That reader exists for umbra, mcp-beaver,
housecast and agent-compose. Everywhere else the reader is Kai or an agent,
and what they need is reference in `docs/` or a skill.

This repo used to carry a shelf and no longer does. Internal procedure is
still procedure, and it lives in `docs/*.md` under the ordinary per-doc caps
rather than on a shelf with none. If your repo is not one of the four, the
rest of this page is background: you are writing a `docs/` page.

## The test

**Can a reader follow it with their hands?**

A guide is the concrete steps a human performs to accomplish a task, in the
order they perform them. The commands they run, the values they supply, what
they should see after each step, and what to do when a step does something
else. Number them. The reader has a terminal open while they read.

Prose about a task is not a guide. A page that explains an area, argues for a
shape, or recounts how something came to be can be accurate and worth keeping
and still belongs in `docs/` or on the tracker record. Length does not convert
it, and the roomier cap on this shelf pays for steps and worked output rather
than for paragraphs. If you cannot point at the step a reader performs first,
you do not have a guide yet.

When the first test leaves you unsure, use the second one. **Cut it to
reference length in your head. Does the value survive?** A reference page loses
nothing by being short and scannable. Somebody arrives knowing what they want,
reads the two paragraphs that answer it, and leaves. A procedure loses the
sequence, and the sequence was the content.

## The failure that produced the type

A walkthrough was written for a project whose `docs/` sat at exactly its
count cap. It was refused three ways at once, and the count cap was the
interesting one. **The page was not too long. The shelf was full and the page
was not that kind of page.**

The remedy text at the time said to split the doc into smaller `docs/*.md`
files, which followed literally destroys a procedure: you get three reference
fragments and no sequence. It says something else now. That is the part worth
keeping, because a validator's suggested fix is read as authoritative by
whoever hit it at midnight.

## What guides/ is not

**It is not overflow for a full `docs/`.** This is the failure most available to
you, and it is available precisely when you are frustrated.

The same evening the type shipped, a comment cap forced explanatory prose out of
two configuration files. The obvious destination doc was at exactly its line
cap, and `docs/` was at its count cap. `guides/` would have taken it. Putting
configuration commentary there would have satisfied every validator and been
wrong, because nobody reads it in order and nobody is doing anything while they
read it. The prose went onto the tracker record instead, and the shelf stayed
for pages that earn it.

Three shapes that look like guides and are not:

* **A tour of a subsystem.** If it has no reader goal, it is architecture
  reference with narration attached.
* **A long changelog or migration note.** Sequential, but the reader is not
  performing the sequence.
* **A design rationale.** Valuable, and it belongs beside the thing it explains
  rather than on a shelf a reader consults while working.
* **A block of prose with no steps in it.** The plainest case and the most
  common. If the page has no command, no input and nothing for the reader to
  do, no amount of subject-matter weight makes it a guide.

## What an over-cap guide usually means

A guide that will not fit is rarely a guide that needs splitting. It is almost
always a guide that **grew a reference section**.

Look for the block that answers "how does X work" rather than "do this next."
That block is a `docs/` page, and lifting it out usually takes the guide back
under cap on its own while making both halves better. The guide then links to it
as `../docs/<name>.md`.

If nothing lifts out and it is still over, the walkthrough is covering two
tasks. Split it by task rather than by length, and accept that both halves cost
a slot on a deliberately scarce shelf.

## Nothing caps how many you have

There was a count cap and it was removed on purpose. A repository can need many
walkthroughs or none, and a number could not tell which.

So nothing external stops guides proliferating, and the test at the top of this
page is what does. A page without a reader doing something in order is not a
guide, and putting it in `guides/` does not make it one. The size caps still
bind, and so does the flat-directory rule.

The failure to watch for is a `guides/` directory that has quietly become a
second `docs/` with longer pages. If you cannot say who reads each one and what
they are doing while they read it, that is what you have.

## Cross-linking out

`dead-cross-links` resolves a relative link against the file it sits in, so a
bare `foo.md` inside a guide resolves to `guides/foo.md` and fails. Reference
`docs/` as `../docs/<name>.md`. This is the one mechanical thing that catches
everybody once, and it fails loudly, which is the good case.
