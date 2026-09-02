# Moving the creature out from behind a pane

The plate places the role creature at 60% of the window width, top right, on a
16:10 canvas that `cscaled` covers. A `tall` split puts the new pane over
exactly that region, so the creature ends up behind whatever was just launched.
Recomposing the plate moves it into the surviving pane.

## Read the current one

`aterm pane off` does not read it at all: it re-derives the plate from the role
in the window's own session card, which is why it still restores after the
process that changed the background is gone. Everything below is the fallback
for a window aterm did not open.

There is no getter. The remote-control API has `set-background-image` and no
matching read, `ls` reports only `background_opacity`, and `get-colors` returns
palette entries. The launch arguments are the read path, because aterm passes
the plate as an override:

```
ps -o command= -p "$(pgrep -f 'MacOS/kitty' | head -1)" | tr ' ' '\n' | grep background
```

That yields `background_image`, `background_image_layout=cscaled`,
`background_image_linear=yes`, and `background_tint`. A plate set at runtime
through `set-background-image` appears in neither the config nor `ps`, so
record the original path before changing it.

## Recompose

Work from the **cached plate**, never from `aterm/icons/<role>.icns`. The cache
copy already carries the glare rolloff that keeps faint text legible over the
art, and regenerating from the icon silently drops it.

```sh
magick "$PLATE" -trim +repage /tmp/creature-only.png          # true ink bounds
magick /tmp/creature-only.png -resize "${W}x" -background none /tmp/creature-scaled.png
magick -size "${CANVAS_W}x${CANVAS_H}" xc:none \
       /tmp/creature-scaled.png -geometry "+${X}+${Y}" -composite "$NEW"
kitty @ --to "$KITTY_LISTEN_ON" set-background-image --layout cscaled "$NEW"
```

Two numbers decide the result:

* **Canvas aspect.** Match the OS window's, and `cscaled` covers with no crop,
  so the composited position is exactly where the art lands. Leave it at 16:10
  and the crop described in the creature doc applies instead.
* **Creature width.** The art is 56% of the plate width as ink, so it does not
  fit a 50% pane. Anything above about 45% of the window width crosses the
  divider. Shrinking is the cost of moving it, and it is the only cost.

## Restore

```
kitty @ --to "$KITTY_LISTEN_ON" set-background-image --layout cscaled "$ORIGINAL_PLATE"
```

Never `--configured`, which rewrites what new windows inherit. Never overwrite
the file under `~/Library/Caches/aterm/creature`, which aterm owns and names by
art and geometry together. Write the recomposed plate somewhere else.
