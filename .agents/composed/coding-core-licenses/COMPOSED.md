---
name: coding-core-licenses
description: Kai's licensing policy - MIT for shareable, AGPL-3.0 for deployment-of-one, proprietary for private. Triggers - license, LICENSE, MIT, AGPL, GPL, copyright, SPDX, what license.
---

# coding-core-licenses

How to pick a license for a `coilysiren/*` repo. Choose by **what the repo is for**, not by habit. Three tiers, most to least permissive.

## Tier 1 - MIT (things meant to be shared)

Anything Kai wants other people to pick up, reuse, fork, and build on. Generic tools, libraries, helper and derivative scripts, substrate tooling that is published for reuse.

- **License:** MIT.
- **SPDX:** `MIT`.
- **Action:** top-level `LICENSE` file, MIT text, Kai's copyright line.
- This is the **default** when a repo's intent is unclear and it is plausibly shareable. Don't sub in Apache-2.0 / BSD / GPL without asking.

## Tier 2 - AGPL-3.0 (deployment-of-one repos)

A repo that is a deploy of one - software Kai runs as a single-tenant deploy for herself, not a library others embed. AGPL is the right tier because it closes the network-service loophole: anyone who redeploys it, modified or not, as a service must offer the source to its users. That keeps a personal deploy from being silently swallowed into someone else's closed product.

- **License:** GNU Affero General Public License v3.0.
- **SPDX:** `AGPL-3.0-or-later` (use the full SPDX identifier, not a bare `AGPL`).
- **Action:** top-level `LICENSE` file with the full AGPL-3.0 text, Kai's copyright line. Set the SPDX id in package metadata (`Cargo.toml` `license`, `package.json` `license`, `pyproject.toml` `license`).
- Existing example: `galaxy-gen`.

## Tier 3 - proprietary, All Rights Reserved (private personal repos)

The stricter-than-AGPL case: a personal repo where the intent is not "share on terms" but "this is mine, do not surface it." The honest finding is that **no named open-source-style license does this** - copyright licenses govern copying, modifying, and distributing, not whether someone may acknowledge the work exists. "Don't cite me, don't reference me, pretend you do not see me" is a privacy posture, not a licensable term. So tier 3 is two separate layers:

- **Legal layer - proprietary, All Rights Reserved.** A repo with no license is already "All Rights Reserved" by default copyright. Make it explicit with a short proprietary `LICENSE` notice (no grant of rights, copyright Kai, all rights reserved). SPDX has no open identifier for this - use `LicenseRef-Proprietary` or omit the SPDX field. Keep the repo private.
- **Posture layer - no-reference.** "Pretend you do not see me" is an agent-behavior rule, not a license. Treat tier-3 repos under leak-discipline: do not cite the repo, its name, or its contents in commit messages, PR text, public issues, blog posts, skill bodies, or cross-repo docs. The exact wiring (a denylist entry, a leak-check rule, a per-repo `AGENTS.md` line) is tracked, not yet built - see the tracking issue.

## When this skill is active

Creating a repo, adding or changing a `LICENSE`, setting a `license` field in package metadata, or answering "what license should this be". Pick the tier from intent, then act.

## See also

- `kai-tech-prefs` - other repo conventions.
