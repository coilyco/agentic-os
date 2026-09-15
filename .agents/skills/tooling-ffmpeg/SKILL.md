---
name: tooling-ffmpeg
description: Inspect and transform audio or video with FFmpeg and ffprobe. Use for transcoding, trimming, remuxing, frame or audio extraction, filters, subtitles, thumbnails, and media diagnostics.
license: LGPL-2.1-or-later
compatibility: Requires `ffmpeg` and `ffprobe`.
metadata:
  source-url: https://ffmpeg.org/documentation.html
---

# FFmpeg

The agent probes media before changing it because extensions do not reveal the
actual streams, codecs, time bases, or metadata. The installed binaries remain
the authority for available encoders, filters, and license-affecting build
options.

## Workflow

1. The agent preserves every input and chooses a new output path.
2. The agent inspects the input before selecting codecs or filters:

   ```text
   ffprobe -v error -show_format -show_streams -of json INPUT
   ```

3. The agent inspects local capabilities when an operation depends on them:

   ```text
   ffmpeg -hide_banner -buildconf
   ffmpeg -hide_banner -encoders
   ffmpeg -hide_banner -filters
   ```

4. The agent starts each write with `ffmpeg -hide_banner -n` so FFmpeg refuses
   to overwrite an existing file. `-n` guards only the path the agent never
   meant to overwrite. When the deliverable must land where a file already
   sits, the agent writes a new sibling and renames it into place on success,
   because FFmpeg 5.x truncates the output during option parsing, before the
   filter graph initialises. A run that fails on a bad filter argument has
   destroyed that file already, and a cleanup keyed on the output existing
   after a failure then deletes a file it never created.
5. The agent maps streams explicitly, keeps optional streams optional, and uses
   stream copy when the task changes only the container or timing:

   ```text
   ffmpeg -hide_banner -n -i INPUT -map 0 -c copy OUTPUT
   ```

6. The agent uses an encoder confirmed by the local build when pixels or samples
   must change:

   ```text
   ffmpeg -hide_banner -n -i INPUT -map 0:v:0 -map 0:a? -c:v VIDEO_ENCODER -c:a AUDIO_ENCODER OUTPUT
   ffmpeg -hide_banner -n -ss TIMESTAMP -i INPUT -frames:v 1 OUTPUT.png
   ffmpeg -hide_banner -n -i INPUT -vn -c:a AUDIO_ENCODER OUTPUT
   ```

7. The agent probes the output, compares its streams and duration with the
   intended result, and previews representative video frames or audio.
   Anything that renders text or composites an image is verified by looking at
   a frame. libass and drawtext draw an empty box for every character they have
   no glyph for, and FFmpeg still exits 0, so no stream summary carries it.

## Guardrails

* The agent treats stream-copy trims as keyframe-bound operations. The agent
  re-encodes when frame-accurate boundaries matter.
* The agent preserves subtitles, chapters, attachments, color metadata, and
  rotation only when the deliverable needs them. Explicit `-map` choices make
  omissions visible.
* The agent quotes paths and filter graphs and avoids shell-expanded globs when
  ordering matters.
* The agent never assumes a named codec exists. The installed FFmpeg build can
  omit encoders or enable GPL and nonfree components.
* The agent treats a successful exit as necessary but insufficient. A valid
  container can still contain the wrong streams, duration, dimensions, or
  channel layout.
* The agent reads `r_frame_rate` against `avg_frame_rate`. When they disagree
  the source is variable frame rate, which is ordinary for phone and screen
  captures. A stream-copy cut there lands on a frozen or wrong frame while
  reporting success, and a re-encode needs `-fps_mode cfr` at a rate the agent
  chose, because a dropped-frame average arrives at values like 23.4.
* The agent reads the transfer and primaries before it picks an encoder.
  Pushing a BT.2020, PQ, HLG, or Dolby Vision source through an SDR path
  flattens the colours and reports nothing, and tagging BT.709 onto BT.2020
  pixels is wrong twice over. The agent keeps the source tags or tonemaps on
  purpose.
* The agent brings the frame to its delivery size before burning text into it.
  Text burned before a crop or resize lands off-frame, and text burned at an
  intermediate size and upscaled later comes out soft, because a 1280x720
  source fit to 9:16 is 406x720 until something scales it.
* The agent rounds width and height to even values for `yuv420p`, and reads the
  rotation tag phone footage carries before computing the output frame rather
  than after the result looks sideways.
* The agent checks true peak alongside integrated loudness, because a file
  normalised to a LUFS target still clips. A clip measuring at or below
  -40 LUFS is room tone, wind, or nothing, and raising it to a speech target
  raises the noise, so the agent says so instead.
* The agent collapses work that can be one filter graph into one filter graph.
  Three re-encodes chained by hand cost three generations of quality for one
  result.
