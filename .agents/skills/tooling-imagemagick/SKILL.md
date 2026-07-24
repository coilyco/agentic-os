---
name: tooling-imagemagick
description: Inspect and transform raster images with ImageMagick. Use for conversion, resizing, cropping, rotation, composition, montage, annotation, metadata handling, or pixel comparison.
license: ImageMagick
compatibility: Requires ImageMagick 7 (`magick`) or ImageMagick 6 command tools.
metadata:
  source-url: https://imagemagick.org/command-line-processing/
---

# ImageMagick

The agent inspects each image before transforming it because geometry, page
count, orientation, alpha, colorspace, and profiles can change the correct
command. The agent prefers ImageMagick 7 syntax. On ImageMagick 6, the agent
uses the corresponding `identify`, `convert`, `compare`, or `montage` command.

## Workflow

1. The agent preserves every input and chooses a new output path.
2. The agent inspects the source:

   ```text
   magick identify -verbose INPUT
   ```

3. The agent applies only the operations the deliverable needs:

   ```text
   magick INPUT -auto-orient -resize 'WIDTHxHEIGHT>' OUTPUT
   magick INPUT -crop WIDTHxHEIGHT+X+Y +repage OUTPUT
   magick INPUT OUTPUT.ext
   magick montage INPUT... -geometry 'THUMBNAIL+GAP' OUTPUT
   ```

4. The agent uses parentheses for isolated image sequences and reads operations
   from left to right because ImageMagick command order changes the result.
5. The agent verifies dimensions, format, colorspace, alpha, frame count, and
   metadata with `magick identify`. The agent also inspects the rendered output
   with the harness's native image viewer.
6. The agent uses comparison metrics when exact or perceptual differences matter:

   ```text
   magick compare -metric AE EXPECTED ACTUAL DIFF.png
   ```

## Guardrails

* The agent avoids `mogrify` because it edits files in place.
* The agent uses `-strip` only when metadata removal is intentional. Profiles,
  orientation, timestamps, and provenance may be part of the deliverable.
* The agent selects frames deliberately for GIF, PDF, TIFF, and other
  multi-image inputs.
* The agent treats `compare` returning nonzero as a reported difference, not
  automatically as an execution failure.
* The agent does not loosen ImageMagick security policy to process an
  unsupported or untrusted input.
* The agent uses [`tooling-image-zoom`](../tooling-image-zoom/SKILL.md) when the
  task is close inspection rather than transformation.
