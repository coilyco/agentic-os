# Build file headers

Why each workflow and build-input file is shaped the way it is. This lived in
each file's own header until the two-line comment cap reached YAML (#1119).

## `.github/workflows/ci.yml`

Pull-request CI gate for director-merge repos. Branch protection can require the resulting `ci / gate` status context on PR-gated workflows. `release` is the promoted last-known-good branch (ward#1117 / aos#469) and never re-gates: promote.yml already ran the suite on the exact sha it fast-forwarded, so a flaky rerun cannot fail a vouched promotion (Kai's call, 2026-07-12).

AOS CLI and Python package tests stay on every build. The separate dev-base-pr workflow is path-filtered to Docker changes.

uv builds the local packages through PEP 517, which resolves setuptools from PyPI whenever the uv cache is cold. That egress flakes on this runner, so the jobs cache ~/.cache/uv and raise uv's 30s HTTP timeout.

## `.github/workflows/models-check.yml`

Live check of per-role model profiles against the Anthropic model list, so a retired model or unsupported effort turns red before a seat launch hits it. It runs daily and on dispatch. The key is operational-only: it lives only in the `ANTHROPIC_MODELS_API_KEY` secret, and every harness launch strips that name ([aos auth](aos-auth.md), Kai's rule of 2026-09-28). Until the secret exists, a scheduled run skips with a warning and a dispatched run fails (`teable:coilyco-flight-deck/agentic-os#7838`). Contract: [native harness configuration](native-harness-config.md).

## `.github/workflows/promote.yml`

Promote main to release after the same suite ci.yml runs. ci.yml never re-gates release precisely because this gate already vouched for the exact sha. promote calls ci.yml as its gate, so it cannot run a narrower suite and promote a red main. test_pull_request_ci_workflow.py holds that. Draft dev-base image publishing runs in a separate workflow keyed by the promoted SHA, so a transient registry or build failure cannot stall the release branch.

uv builds the local packages through PEP 517, which resolves setuptools from PyPI whenever the uv cache is cold. That egress flakes on this runner, so the jobs cache ~/.cache/uv and raise uv's 30s HTTP timeout.

## `.github/workflows/release.yml`

Full-image release publication on GHCR. dev-base-publish.yml calls this workflow after the draft graph succeeds. It passes `make_latest: "false"`, since `aos-cli-release.yml` owns the latest flag. Manual dispatch reuses the same jobs for retries and explicit version overrides.

## `.github/workflows/dev-base-publish.yml`

Publish commit-scoped language payloads and the full image after release has advanced, then publish the root minor release from the verified full draft. Registry or build failures do not block branch promotion. Jobs run on `ubuntu-latest` and push to `ghcr.io/coilyco/agentic-os` with the workflow token. No job uses a self-hosted runner (COI-2967, COI-3042).

## `docker/dev-base/fleet-precommit-hooks.yaml`

Hook environments warmed into the image at build time so CI never pays the cold-cache install. Stale pins here cost speed, never correctness: pre-commit falls back to installing whatever this misses.

Carries the externally-hosted pins the fleet shares.

## `AGENTS.md`, the git-workflow block

Every repo declares its landing lane once, as `ward.workflow` in the AGENTS.md frontmatter. A hand-written one-line stamp used to restate it without ever saying the lane **is** a standing authorization, so agents kept stopping to ask before a commit, a push, or a pull request, and a turn that stops there strands the work in a dirty worktree.

`generate-git-workflow` replaces it with a marker-delimited managed block rendered from that declared lane. The block names the one fleet lane, `pull-request-and-merge`, says which lane this repo declares, and states the pre-authorization in MUST / ALWAYS / NEVER terms, with `--no-verify` and force-push held closed. It also says outright that a slug names what the **agent** does: `pull-request-and-merge` carries the merge because the author merges its own PR, and `pull-request` drops it because the author stops there. Two drafts inverted that. A repo declaring no lane renders the `pull-request` variant, which neither pushes `main` nor merges.

`merge-remote-main` is retired: it allowed the direct push, and pushing straight to `main` ended fleet-wide. Dropping the slug from `LANES` makes it unrenderable, so a repo declaring it reads as undeclared and gets the guarded shape, and `pr-guard` stands down for no lane now. `coilysiren/coilysiren` keeps it deliberately, being GitHub-canonical with `.agentic-os-ignore`, no catalog hooks, and no managed block.

- **`git-workflow`** (pre-commit hook) regenerates the block offline and fails on drift, a missing block, a block that no longer matches the declared lane, or a legacy stamp beside it. Org-agnostic, no base repo exempt: a lane binds in the base as in a consumer.
- **`apply-git-workflow`** injects or refreshes the block in place, under `## Agent rules`. Idempotent: the lane comes from the file it rewrites.

**Forge.** GitHub is the only wording (COI-3083). The block names a GitHub pull request, and the full body names the update and merge verbs, which are the `gh pr` verbs. There is no `forge` option, so a leftover `forge` key in `[tool.agentic-os.git-workflow]` is ignored, and the hook fails a repo still carrying the old Forgejo block. The pointer body names no verbs.

Fleet rollout waits on a tagged `aos-precommit` release, then runs in order: `apply-git-workflow` lands the block, the rollout enables the hook. Reversed, every commit breaks.

Schema, rollout, and the applier live in [`agentic_os/generators/generate_git_workflow.py`](../agentic_os/generators/generate_git_workflow.py).
