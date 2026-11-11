# One meaning per colour (COI-3583)

Every colour in the client says one thing, so a reader learns the screen once. The tokens live in the block at the top of `aterm-client/src/app.css`, and [features.md](features.md) says how they are read. This page says what each one is for. A new surface follows it, and a colour that wants a second meaning gets a new token instead.

## The rule

* **Green** (`--brand`, which `--ok` aliases) - on, and go. A button to press, a switch that is on, a focus ring, a link, a toggled tab, a host that is up. A seat that is waiting or has something new is not green.
* **Seat mark** (`--seat-mark`) - one neutral cream every seat shares, on its selected row, its name's role label, and the terminal's cursor and selection. Which seat it is comes from the creature and the name. The roster's `favorite_color` is read and not drawn, and a roster colour change touches nothing here (COI-3621).
* **Amber** (`--warn`, `--warn-text`, `--warn-fill`, `--warn-line`) - take care, or a seat is waiting on a person: the rail and tint on its row, the "?" on its avatar, "asking you", and the "N waiting" pill.
* **Red** (`--danger`, `--danger-text`, `--danger-fill`, `--danger-line`) - failed, or cannot be undone. It shows only when something has.
* **Text, soft, muted** - words, in falling weight. `--line` and `--control-line` draw every border that means nothing, the header's rule among them. A seat with new output is a white dot and a white state line.
* **The brown steps** (`--ground`, `--terminal`, `--surface`, `--raised`, `--selected`, `--hover`) - depth and selection, never a signal.
* **The terminal palette** (`--ansi-*`) - the harness's own. The client chooses the 16 values and never recolours the harness's output. Chrome reads none of them.

## How many hues the chrome shows

Two, and a third on failure: green for on, amber for waiting, red for failed. The creature art and the harness's own palette sit outside that count. Before COI-3621 the same busy seat showed four, green plus one per role in view (pink, red, teal and olive for the four seats), and every role in the roster added another. The `colour-rule` journey's picture is the check.

## The terminal palette

Every ANSI foreground except black reads at 4.5:1 or better on `--terminal`, and `ansi-palette.test.ts` holds it. Black is the `--terminal` colour itself, so a harness that paints a black background draws no rectangle. Red is `#ee7769` at 6.03:1, the lowest. The xterm defaults it replaced read 2.57 to 2.88:1 for red, blue and magenta.

## The before and after

The `colour-rule` journey builds one busy seat: a diff, test results, a link, and both 8-colour ramps. `ATERM_TESTBED_COMPARE=0 just aterm-client-testbed colour-rule --project desktop-1440` writes `01-busy-seat.png`. It compares no baseline, so a retune never churns one.

## The light theme (COI-3554)

The same roles on paper: `:root[data-theme="light"]` in `app.css` redefines every token but the plate. The brown steps run to cream, green, amber and red darken until they read as text, and the rule above holds unchanged. `ansi-palette.test.ts` holds the contrast on both grounds.

* **Text** - `--text` 13.9:1, `--text-soft` 8.8:1 and `--muted` 6.5:1 on the ground, and 5.2:1 or better on the lowest step. Green as text is 5.8:1 on the ground and 4.7:1 on `--selected`, and its button ink reads 6.4:1.
* **Signals** - amber text is 5.8:1 and red text 6.8:1 on the ground, and 5.8 and 6.6 on their own fills. `--warn` and `--danger` are marks, not text, at 3.9 and 4.7.
* **Borders** - `--line` 1.4:1 and `--control-line` 2.0:1, the same soft edge the dark theme draws, so a control is found by its fill and label first.
* **Terminal** - black is dark ink and white is the ground, so a harness that paints a white background draws no rectangle. Bright white is the strongest ink. Every other foreground reads 4.7:1 or better. A harness that draws plain white text for a dark ground shows it as the ground, as in any light terminal theme, and Dark is one tap away.
* **Outside** - the Android shell tints its status and navigation icons from the theme. The web manifest's splash colour stays the brown, since a manifest holds one.
