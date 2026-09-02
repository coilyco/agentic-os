# Filling a pane with an image

`kitten icat` can only fit, never cover, so an image whose aspect differs from
the pane gets bars, and an image smaller than the pane is drawn at natural size
with the rest left empty. Reaching edge to edge means measuring the pane in
pixels, cropping to exactly that, and placing the result.

## The three traps

* **Measure from `/dev/tty`, not stdout.** Each kitty pane has its own pty with
  `ws_xpixel` and `ws_ypixel` set, so `TIOCGWINSZ` returns pixels as well as
  cells. Inside `$(...)` stdout is a pipe and the ioctl raises `ENOTTY`, which
  fails as an empty measurement rather than an error.
* **The `--place` offset separator is `x`.** `--place 84x55@0x0` is valid and
  `@0,0` is rejected as `Invalid --place specification`.
* **The source must exceed the pane.** There is no upscale without `--place`,
  and even with it `--scale-up` preserves aspect, so a small source can never
  fill. Fetch or generate above the measured size.

`--place` also leaves the cursor at the top left of the image instead of on the
line after it, which is what stops a full-height draw from scrolling its own
top row off the screen.

## Recipe

Run this as the pane's own command, so it measures the pane it is drawing into.

```sh
#!/bin/sh
SRC="$1"; OUT="$2"
set -- $(python3 - <<'PY'
import fcntl, os, struct, sys, termios
fd = os.open('/dev/tty', os.O_RDONLY)
rows, cols, xpx, ypx = struct.unpack('HHHH', fcntl.ioctl(fd, termios.TIOCGWINSZ, b'\0' * 8))
print(cols, rows, xpx, ypx)
PY
)
COLS=$1 ROWS=$2 XPX=$3 YPX=$4

# Cover, not contain: scale by max(tw/w, th/h) so both axes reach the target,
# then centre-crop the overflow away.
python3 - "$SRC" "$OUT" "$XPX" "$YPX" <<'PY'
import subprocess, sys
src, out, tw, th = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
g = subprocess.run(['sips', '-g', 'pixelWidth', '-g', 'pixelHeight', src],
                   capture_output=True, text=True).stdout
w = int([l for l in g.splitlines() if 'pixelWidth' in l][0].split(':')[1])
h = int([l for l in g.splitlines() if 'pixelHeight' in l][0].split(':')[1])
s = max(tw / w, th / h)
subprocess.run(['sips', '-z', str(max(th, round(h * s))), str(max(tw, round(w * s))),
                src, '--out', out], capture_output=True)
subprocess.run(['sips', '-c', str(th), str(tw), out, '--out', out], capture_output=True)
PY

kitten icat --place "${COLS}x${ROWS}@0x0" "$OUT"
read _park   # hold the pane open; Enter closes it
```

`sips` ships with macOS, so no ImageMagick is needed for the crop. With
ImageMagick present, `magick SRC -resize WxH^ -gravity center -extent WxH OUT`
is the same two steps in one command.

## Redraw

The measurement happens once at startup, so a resized pane keeps the old
geometry until the command is re-run. Wrap the measure-and-draw block in a
`trap ... WINCH` loop if the pane needs to track its size.
