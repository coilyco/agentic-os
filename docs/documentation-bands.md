# Documentation size bands

A repo declares one band and gets its three caps: lines and chars per Markdown
file, and how many `docs/*.md` files it may carry.

| band | lines | chars | docs |
| --- | --- | --- | --- |
| `small` | 40 | 3,000 | 20 |
| `large` | 120 | 8,000 | 40 |

```toml
[tool.agentic-os.documentation-layout]
band = "small"
```

Every repo declares, small included. There is no default to fall into: an
undeclared repo and a deliberately small one would otherwise be the same file,
and only one of them has had the decision made. A repo outgrowing `small`
should hit a cap and argue for `large`, not find out it was never on a band.
A typo fails the same way a missing declaration does, so nothing is measured
against a band it did not name. Non-Python repos declare the same key in
`.agentic-os.toml`.

## Why a count cap exists at all

A per-doc size cap does not bound a docs folder. It reshapes it. A repo that
caps length and not count answers every over-long doc by splitting it, and the
folder grows without any single file ever failing.

`sirens-echo` is the proof: 156 docs, median 2,935 chars, largest 3,989 against
a 4,000 cap. Not one file over the limit, and a folder nobody can read.

## Why lines bind before chars

Measured across the fleet, Markdown here runs about 49 characters per line. So
40 lines is roughly 1,960 characters and 120 lines is roughly 5,880. The char
cap sits above both on purpose: it is the backstop that catches a doc dense
with tables or code, not the everyday constraint.

That relationship is what makes the pair work. A char cap set near the line
cap's natural size would fire constantly on ordinary prose; one set far above
it would never fire at all.

## The two caps multiply

Count times lines is a total documentation budget, and it is the number worth
arguing about rather than either cap alone.

* `small` - 20 x 40 = 800 lines
* `large` - 40 x 120 = 4,800 lines

A repo cannot escape the budget by trading one cap against the other. Merging
two docs to clear the count spends the line cap, and splitting one to clear the
line cap spends the count.

## No per-file escape

`excludes` still governs placement and flatness, and no longer reaches either
size cap or the count. A generated file that lands over the cap is a generator
emitting too much, and the fix is the generator.

The rule this replaced allowed a per-file exemption, and the exemptions
accumulated exactly where the pressure was highest: one repo excluded nine
individual `SKILL.md` files, two excluded `docs/FEATURES.md` from the cap that
exists to keep it an inventory.
