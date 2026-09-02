---
name: tooling-aterm-splitscreen
description: Split an aterm session's kitty window and draw an image in the new pane over the graphics protocol, then restore. Triggers - split screen, side by side, kitty pane, kitty remote control, kitten icat, show an image in the terminal, background image, creature plate.
---

# aterm split screen

Use this to put something beside a running session: an image, a preview, a
second program. The window is kitty, launched by [aterm](../../../docs/aterm.md),
and the agent drives it over remote control rather than by printing escapes.

## Reach for the verb first

`aterm pane on -- <command>` and `aterm pane off` do the whole split-and-restore
cycle, creature included, and `off` works after the agent that split the window
has died. Use them instead of the calls below whenever the window came from
aterm: [docs/aterm-pane.md](../../../docs/aterm-pane.md).

The rest of this skill is what the verbs do, and it is what you still need for a
kitty aterm did not open, for a pane layout the verbs do not cover, and for
drawing into the pane once it exists, which stays hand-rolled.

## Transport

* An agent inside an aterm session has **no controlling terminal**. `tty`
  reports `not a tty` and `/dev/tty` fails with `device not configured`, so
  graphics escapes written to the agent's own stdout reach nothing.
* `$KITTY_LISTEN_ON` holds the socket. Every command here is
  `kitty @ --to "$KITTY_LISTEN_ON" <verb>`.
* A process launched into a new pane **does** get a controlling terminal, so
  measuring and drawing happen there, never in the agent shell.

## Vocabulary

kitty calls a pane a `window` and calls the real window an `os-window`, so
`close-window` and `close-os-window` are one keystroke apart and do very
different things. `--type=overlay` is also a pane, stacked over the session
instead of beside it, which covers the thing you were reading.

## Split

* **The tab layout decides the direction, not a launch flag.** `fat` puts the
  new pane below. `tall` puts it to the right at 50/50. Run
  `goto-layout tall` before launching, or accept a horizontal band.
* `launch --dont-take-focus` leaves the keyboard in the session pane.
* `launch --var role=<name>` tags the pane at birth, which is the handle for
  every later command.

```
kitty @ --to "$KITTY_LISTEN_ON" goto-layout tall
kitty @ --to "$KITTY_LISTEN_ON" launch --dont-take-focus --var role=preview <cmd>
```

## Matching

`var:` first. It is set by the launcher, nothing inside the pane can rewrite
it, and it survives moves and layout changes. `id:` is exact and never reused,
so it is correct now and wrong after any relaunch. `title:` is a regex against
a string the pane's own program owns, and an agent session rewrites its title
continuously as its state changes. `num:` and `neighbor:` are positional and
shift the moment a pane is added, closed, or focused.

## Draw

`kitten icat` is contain-only. `--place` scales to **fit** the rectangle,
`--fit` offers width, height, both, or none, `--scale-up` only applies
alongside `--place`, and there is no crop option at all. Filling a pane edge
to edge therefore means measuring the pane and cropping before the draw:
[pane-image.md](references/pane-image.md).

## The creature moves with the split (`aterm pane` does this for you)

The plate places the role creature at 60% of the window width, top right,
which is exactly where a `tall` right-hand pane lands, so splitting hides it.
Recompose and restore: [creature-plate.md](references/creature-plate.md).
Plate geometry and the tint that makes it readable are owned by
[the creature doc](../../../docs/aterm-creature.md).

## Restore

Put the background back before closing the pane, so no state is left behind:

```
kitty @ --to "$KITTY_LISTEN_ON" set-background-image --layout cscaled <original-plate>
kitty @ --to "$KITTY_LISTEN_ON" close-window --match var:role=preview
kitty @ --to "$KITTY_LISTEN_ON" goto-layout fat
```

Everything above is runtime-only and dies with the OS window. Never pass
`--configured` to `set-background-image`, which would rewrite the value new
windows inherit.
