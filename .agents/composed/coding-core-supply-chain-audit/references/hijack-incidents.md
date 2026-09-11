# Hijack: signals 5-7, incidents, defense in depth

Companion to [hijack-patterns.md](hijack-patterns.md).

## Signal 5: Maintainer tweets / blogs about handing off the project

Real-life pattern: maintainer announces "I'm stepping away, [random handle] is taking over." Random handle then ships a malicious release.

When a project transitions ownership, treat the new maintainer as a fresh entity in this audit. Apply the full checklist to them. Don't inherit the trust score of the previous maintainer.

## Signal 6: Minified, vendored, or obfuscated code

Vendored dependencies, minified JS bundles, base64-encoded blobs, "compiled" generated code committed to source - all of these are places where a payload can hide. Audit the source-of-truth, not the artifact.

If a `dist/` folder is committed, look at `src/` and ensure the `dist/` is reproducible from source. If it's not (e.g. random build outputs of unknown provenance), that's a red flag for the project's hygiene at minimum and a potential payload site at worst.

## Signal 7: New maintainer adding a "helpful" telemetry endpoint

Real-world incident pattern: a new maintainer ships a release that includes a "phone-home" call ("we just want to count installs!"). Even if the intent is benign, telemetry endpoints in build scripts or runtime startup code are unacceptable for a library - they're consent-bypassing exfil channels.

Surface this even when intent looks benign. The dep should be added to your audit-rejected list and the user should be told.

## Real incidents to reference

When in doubt, search for these incident write-ups for pattern reminders:

- **event-stream** (npm, 2018) - minor maintainer added, then a cryptocurrency-stealer payload introduced.
- **ua-parser-js** (npm, 2021) - maintainer's npm account stolen, then a coinminer payload published.
- **xz-utils backdoor** (2024) - multi-year social-engineering campaign, with an obfuscated payload in release tarballs but not the git source.
- **rc** (npm, 2021) - typosquat / account takeover, with exfil to an attacker-controlled webhook.
- **colors / faker** (npm, 2022) - maintainer self-sabotage, not malicious in the security sense but still broke downstream.
- **PyPI mirroring attacks** - typosquatting `requets` for `requests`, etc. Don't just check the existence of the package, check the spelling carefully.

## Defense in depth (post-audit)

Even after a clean audit, add ongoing protection:

- `cargo audit` / `npm audit` / `pip-audit` in CI.
- Dependabot or Renovate enabled, with auto-merge restricted to patch-level updates of already-audited deps.
- Pin to exact versions in production. Use ranges only in libraries.
- For load-bearing deps, mirror the source to coilysiren as a fallback in case upstream is hijacked or yanked.
