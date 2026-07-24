---
name: tooling-image-zoom
description: Crop and re-examine small or ambiguous image regions. Use for dense screenshots, charts, scans, diagrams, or text that is unreadable at full-image scale.
license: MIT
compatibility: Requires Bash and ImageMagick (`magick` or `convert` plus `identify`).
metadata:
  source-url: https://github.com/anthropics/claude-cookbooks/blob/main/multimodal/crop_tool.ipynb
---

# Image zoom

Inspect the complete image before cropping so each detail keeps its surrounding
context. Use the bundled helper when a label, mark, control, or relationship is
too small or ambiguous at the original scale.

## Workflow

1. Read the complete image and identify the region that needs closer inspection.
2. Choose a bounding box as normalized `x1 y1 x2 y2` coordinates from `0` to `1`.
   `(0, 0)` is the top-left corner and `(1, 1)` is the bottom-right corner.
3. Resolve `scripts/crop-image` relative to this `SKILL.md`, then run:

   ```text
   scripts/crop-image INPUT OUTPUT X1 Y1 X2 Y2
   ```

4. Read the output image with the harness's native image-reading tool.
5. Repeat with a new output path when the first crop still contains unreadable
   detail. Widen the bounds when a tight crop loses necessary context.

## Guardrails

* Use a new output path because the helper refuses to overwrite an existing file.
* Treat unreadable detail as unresolved, not absent.
* Preserve the original image and report material uncertainty in the answer.
