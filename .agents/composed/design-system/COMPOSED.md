---
name: design-system
description: Generate or audit design systems, design tokens, component consistency, accessibility, and visual polish. Triggers - design system, design tokens, visual audit, styling review, UI consistency, accessibility review.
metadata:
  origin: ECC
  source: https://github.com/affaan-m/ECC/tree/591ab5cbd3f2f65860ea91c226e410b1502c8e2e/skills/design-system
  revision: 591ab5cbd3f2f65860ea91c226e410b1502c8e2e
  license: MIT
---

# Design System Generation and Audit

Use this workflow to make visual-system decisions explicit and testable. Read
the target repo's design conventions, product context, and command
surface before applying it.

## Generate a design system

1. Scan existing CSS, component styles, theme files, and design artifacts.
2. Extract colors, typography, spacing, radii, shadows, breakpoints, motion,
   and state treatments.
3. Identify intentional patterns, accidental one-offs, and accessibility gaps.
4. Research comparable products only when current external evidence will
   materially improve a decision.
5. Propose a compact token set and map existing values into it.
6. Define reusable component patterns, including focus, disabled, loading,
   empty, error, hover, and responsive states.
7. Produce the repo's expected design artifact, such as `DESIGN.md`,
   tokens, component documentation, or a self-contained preview.

Preserve established product character unless the task explicitly calls for a
new direction. Explain the reason for each deliberate change.

## Audit an existing interface

Score each dimension from 0 to 10, then cite concrete examples and the smallest
useful correction:

* **Color** - palette reuse, semantic roles, and contrast.
* **Typography** - hierarchy, legibility, and consistent text roles.
* **Spacing** - rhythm, density, alignment, and layout scales.
* **Components** - shared structure and complete interaction states.
* **Responsive behavior** - reflow, overflow, touch targets, and breakpoints.
* **Themes** - complete light, dark, and high-contrast treatment where present.
* **Motion** - purposeful feedback with reduced-motion support.
* **Accessibility** - keyboard access, focus visibility, semantics, and contrast.
* **Information density** - readable grouping without unnecessary decoration.
* **Polish** - coherent icons, borders, transitions, loading, and empty states.

Verify visual claims against rendered pages when the available tools permit it.
Report an unverified rendering gap instead of inferring pixels from source.

## Check for generic output

Flag patterns that appear without a product reason:

* default purple-to-blue gradients
* decorative glass effects
* uniform rounded cards around unrelated content
* excessive scroll animation
* interchangeable centered hero layouts
* typography with no deliberate hierarchy or voice

Treat these as prompts for inspection, not automatic defects. Keep a pattern
when it serves the product, content, or interaction.

## Provenance

Adapted from
[`affaan-m/ECC/skills/design-system`](https://github.com/affaan-m/ECC/tree/591ab5cbd3f2f65860ea91c226e410b1502c8e2e/skills/design-system)
at revision `591ab5cbd3f2f65860ea91c226e410b1502c8e2e`. The upstream source is
MIT licensed. See [`LICENSE`](LICENSE).
