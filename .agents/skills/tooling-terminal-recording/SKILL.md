---
name: tooling-terminal-recording
description: Record a terminal as a GIF or video. VHS for deterministic commands, asciinema for sessions whose timing cannot be predicted, a human-run live take only for what the text stream cannot carry. Use when producing a demo, README asset, or social clip from CLI output.
compatibility: Requires `vhs`, `asciinema`, and `agg`. VHS also pulls `ttyd` and `ffmpeg`.
metadata:
  source-url: https://github.com/charmbracelet/vhs
---

# Terminal recording

The agent never performs a live take. Timing, retakes, and the operator's
attention are all removable from the critical path, and removing them is the
point of both tools here. The one exception is content the text stream cannot
carry, which the human records instead of the agent. See below.

## Choosing the tool

VHS renders a `.tape` script to GIF, MP4, or WebM with no live session. It suits
any command that is fast and deterministic. A wrong pause is an edit to a number
instead of another take.

asciinema records a real session to a text cast. It suits anything whose output
timing cannot be known in advance, which includes every agent session, since
responses vary by tens of seconds. `agg` converts a cast to GIF.

## Measure before choosing dimensions

1. The agent measures the widest line the command emits, because that number
   dictates every other setting:

   ```text
   COMMAND | awk '{print length}' | sort -rn | head -1
   ```

2. The agent converts columns to pixels at roughly `0.83 * FontSize` per column,
   then picks `Set Width` to fit the measured width without wrapping.
3. The agent checks the resulting aspect ratio against the destination. Output
   past about 120 columns yields a letterbox strip that suits a README banner
   and fails on a social post. When the ratio will not work, the fix is a
   narrower command instead of a smaller font: pipe through `head` and `cut`,
   or record a different subcommand.

## Recording a live session

The agent records with an idle cap so the operator does not have to be quick,
attentive, or uninterrupted while the recording runs:

```text
asciinema rec -i 2 demo.cast
agg demo.cast demo.gif
```

Every pause longer than the `-i` value clamps to that value on playback. The
cast stays plain text with per-event timings afterwards, so pacing remains
editable long after the session.

## When the text stream is not enough

Both tools above record text. A cast is a stream of characters and escape
sequences, so anything the terminal draws outside that stream is absent from the
recording: an image sent over the kitty graphics protocol, a background image,
the creature plate, pane layout after a split. `agg` renders a cast as text and
cannot reproduce any of it. VHS drives its own headless terminal and never sees
the real window at all.

That leaves a screen recording, with two rules.

**The human records, not the agent.** Screen capture on macOS is display-scoped.
`screencapture -v` records a display, `-V` bounds the duration, and there is no
window-id equivalent for video, so `ffmpeg` via avfoundation is display-scoped
too. Worse, the TCC grant attaches to the shared binary behind every seat rather
than to one role, so granting it to an agent grants every seat capture of
whatever is frontmost. A still capture taken this way has already returned
another seat's private session. The human recording with the built-in recorder
needs no grant, keeps the permission surface at zero, and lets the human choose
the frame.

**The agent drives, the human shoots.** The agent resets state and clears
scrollback (`kitty @ action clear_terminal scrollback active` clears the pane
without touching the agent's own context), then responds to cue lines agreed
before the take so nothing is improvised on camera. Short replies land after the
visual instead of narrating it.

Turn `viewMode` to compact in `/config` first. Verbose expands every tool call
inline and is the single largest source of on-screen noise. `aterm --no-motion`
skips the identity card animation and `--silent` skips the launch sound, both
documented for exactly this.

## Cut the take down

A raw macOS recording is 120fps, full display resolution, and carries an audio
track of the room. All three are wrong for a social clip and the audio is a
privacy leak on its own.

```text
ffmpeg -ss HEAD -to TAIL -i RAW.mov -an \
  -vf "setpts=0.5*PTS,scale=1512:-2,fps=60" \
  -c:v libx264 -preset slow -crf 20 -pix_fmt yuv420p -movflags +faststart OUT.mp4
```

`-an` drops the audio. `setpts=0.5*PTS` doubles the speed, which suits typing
and idle thinking. 60fps instead of 30 when the clip contains an animation
driven at around 32fps, since 30 judders against it. Half resolution is ample
for a feed. A 30MB 44 second capture becomes about 1.4MB at 21 seconds.

Find the trim points by sampling instead of scrubbing, cropping to the region
that tells you what you need:

```text
ffmpeg -v error -ss 30 -i RAW.mov -vf "fps=1,scale=380:-1" rev/t%02d.png -y
```

Prefer MP4 over GIF for anything going to a social feed. Feeds autoplay native
video, and the same content is roughly a tenth the size.

## Review every frame before it leaves the machine

A display capture holds whatever else was on screen. Before any recording is
published the agent samples it end to end and looks, which is a privacy review
instead of a quality one:

```text
ffmpeg -v error -i RAW.mov -vf "fps=1/3,scale=480:-1" rev/f%02d.png -y
```

Check for other windows, notification banners, and readable absolute paths. Then
delete the raw capture once the cut is made, since the raw is the copy that
still holds everything the crop removed.

## Verify the frames

Exit zero from `vhs` means a file was written, not that the file is legible. The
agent extracts a frame and looks at it before reporting a render as good:

```text
ffprobe -v error -show_entries format=duration -of csv=p=0 OUT.gif
ffmpeg -v error -ss SECONDS -i OUT.gif -vframes 1 frame.png -y
```

Wrapping, scrolled-off content, and mid-token `cut` damage are all invisible in
the exit code and obvious in the frame.

## Showing the result to a human

macOS Preview steps an animated GIF frame by frame instead of playing it, so
the agent opens rendered output in a browser. Several clips go on one local HTML
page instead of several tabs.
