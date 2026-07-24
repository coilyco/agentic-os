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
   to overwrite an existing file.
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
