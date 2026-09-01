---
name: tooling-sirens-dowel-temporal-brand
description: Temporal's visual system as Dowel's room knowledge - the three published brand colors, the named gradient set, the semantic token layer, Aeonik typography, scale, page architecture, and the workflow-state visual language. Use when the deployed role is engineer and the instance is sirens-dowel, and the room asks about Temporal's look, colors, type, or how its workflow views encode state.
---

# Temporal brand system

Dowel sits in a Temporal room. This file is what Dowel knows about how Temporal
looks, so an answer about the brand is sourced instead of guessed.

Exact values live in [`references/palette.md`](references/palette.md). Scale,
page form, and the state-visualization language live in
[`references/architecture.md`](references/architecture.md). Dowel reads the
reference before quoting a number, because the published palette and the
shipped token layer are two different things and the difference is exactly what
someone in a Temporal room would catch.

## Fetch first, and use this as the floor

`temporal.io` is on the fetch allowlist, and the lane's standing rule is that a
question about a moving product gets answered from a page fetched this turn.
The brand page and the shipped stylesheet move the same way, so that rule
reaches here too. Dowel fetches when the answer has to be current, and where a
fetched page and this file disagree, the page wins and there is nothing to
reconcile.

What this file is for is the floor under that. It is the grounding when a fetch
is not worth a turn, the thing that makes a stale answer recognisable as stale,
and the record of which layer a value came from, which a raw stylesheet does not
tell you.

## Activation wall

This is reference knowledge, not a mandate to art-direct. Dowel answers what is
asked and does not turn a passing question about a color into a design review.

The lane's work product is still one message in one channel, so a full palette
dump is almost never the answer. Dowel gives the value asked for, names where it
comes from, and offers the rest only if the room wants it.
[`tooling-sirens-dowel-contract`](../tooling-sirens-dowel-contract/COMPOSED.md)
governs the seat, and nothing here changes it.

Temporal owns this system. Dowel describes it and never implies coilyco speaks
for Temporal or has any license to their marks.

## Take the three published colors as fixed

Temporal publishes exactly three brand colors and nothing else.

* **UV** - `#444CE7` - rgb(68 76 231) - the sole brand accent, carrying brand surface, brand text, information state, and focus.
* **Space Black** - `#141414` - rgb(20 20 20) - the default page ground, a warm-neutral near-black instead of true black.
* **Off White** - `#F8FAFC` - rgb(248 250 252) - primary text on the dark ground and the inverse surface when a section flips light.

Every other value in the shipped site is a derived semantic token or a gradient
stop. The three are the brand and everything downstream is the system built on
top of them. Saying which layer a value came from is the whole difference
between an accurate answer and a plausible one.

## Read the ground as dark by default

The site sets its background surface to Space Black and its primary text to Off
White, then flips to an inverse surface for the sections that need to read as
paper. Light is the exception that gets declared, not the base that gets
overridden.

The neutral ramp between those poles is a desaturated blue-grey instead of a
grey, which is what keeps a near-black page from reading as dead. Borders,
table chrome, and secondary text all sit on that ramp.

## Carry the gradient set, not a gradient

Four gradients are named in Temporal's own SVG definitions, and each is a
two-stop linear ramp with no third color.

* **purple-ultraviolet** - `#B664FF` to `#444CE7` - the primary mark treatment, landing on the brand accent.
* **pink** - `#FF5555` to `#B664FF`
* **green** - `#C3FF62` to `#1FF1A5`
* **mist** - `#34D399` to `#FF6BFF`

These belong to marks, icon fills, and illustration. They are not page
backgrounds and they never sit under body text.

## Set type in one family and one mono

* **Aeonik** - the entire text face, from Air through Black, upright and italic. It is a geometric sans with mechanical construction and a wide weight range, which is what lets a single family carry both a 3.75rem hero and 0.75rem table chrome.
* **Aeonik Air** - shipped as its own family instead of a weight, so the hairline display cut has to be requested by name.
* **Noto Sans Mono** - code, event identifiers, and machine values.

Aeonik is a commercial release from CoType Foundry. Asked how to reproduce the
look, Dowel says the typeface is paid instead of pointing at a lookalike.

## Make state legible before making it beautiful

Temporal's workflow views encode state in five separable channels, and each
channel answers exactly one question. This is the part of their design worth
talking about, because it is a real idea instead of a color choice.

* **Dots** - a dot is one event. Position carries sequence.
* **Lines** - a line is a connection between events, and line weight distinguishes group from detail. A dashed line that animates forward means pending.
* **Icons** - an icon carries category alone, never status.
* **Colors** - color carries status first and category only as a secondary echo. Red is failure, dashed red is retrying, dashed purple is pending, green is completion.
* **Liveness** - the view updates in real time, and pending work is attached to the thing it belongs to instead of parked in a separate panel.

The transferable rule is one channel, one question. When status and category
compete for the same channel, neither reads.

## Reject the near-misses

* Sampling a hex from a screenshot instead of the token layer, which produces a compressed approximation of a published value.
* Quoting a derived token as a brand color, which is the most common way this goes wrong.
* Treating a campaign announcement banner color as brand, since that bar is content-driven and changes per campaign.
* Treating an embedded third-party widget's color as brand.
* Using true black instead of Space Black, which loses the warmth the palette depends on.
* Letting an icon carry status, which collapses the channel separation the state language rests on.

## Verify before answering

* Name which layer a value came from, published brand or derived token.
* Say the value is from a reading of the shipped site when it is, instead of implying Temporal published it.
* Check contrast claims against both the dark ground and the inverse surface before making one.
* Say so plainly when a question reaches past what is recorded here, since a confident invented hex is worse in this room than a short answer.

## Provenance

Extracted from [`temporal.io/brand`](https://temporal.io/brand), the shipped
site stylesheet, and
[the new-UI article](https://temporal.io/blog/the-dark-magic-of-workflow-exploration),
read in August 2026. Temporal owns these assets and this file is a reading of
them, not a grant to use them. A shipped token layer moves without notice, so a
value here can go stale, which is why the fetch rule above outranks it.
