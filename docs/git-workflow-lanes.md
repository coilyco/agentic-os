# Git workflow lanes

Every repo declares its landing lane once, as `ward.workflow` in the AGENTS.md frontmatter. A hand-written one-line stamp used to restate it without ever saying the lane **is** a standing authorization, so agents kept stopping to ask before a commit, a push, or a pull request, and a turn that stops there strands the work in a dirty worktree.

`generate-git-workflow` replaces it with a marker-delimited managed block rendered from that declared lane. The block names the one fleet lane, `pull-request-and-merge`, says which lane this repo declares, and states the pre-authorization in MUST / ALWAYS / NEVER terms, with `--no-verify` and force-push held closed. It also says outright that a slug names what the **agent** does: `pull-request-and-merge` carries the merge because the author merges its own PR, and `pull-request` drops it because the author stops there. Two drafts inverted that. A repo declaring no lane renders the `pull-request` variant, which neither pushes `main` nor merges.

`merge-remote-main` is retired: it allowed the direct push, and pushing straight to `main` ended fleet-wide. Dropping the slug from `LANES` makes it unrenderable, so a repo declaring it reads as undeclared and gets the guarded shape, and `pr-guard` stands down for no lane now. `coilysiren/coilysiren` keeps it deliberately, being GitHub-canonical with `.agentic-os-ignore`, no catalog hooks, and no managed block.

- **`git-workflow`** (pre-commit hook) regenerates the block offline and fails on drift, a missing block, a block that no longer matches the declared lane, or a legacy stamp beside it. Org-agnostic, no base repo exempt: a lane binds in the base as in a consumer.
- **`apply-git-workflow`** injects or refreshes the block in place, under `## Agent rules`. Idempotent: the lane comes from the file it rewrites.

Fleet rollout waits on a tagged `aos-precommit` release, then runs in order: `apply-git-workflow` lands the block, the rollout enables the hook. Reversed, every commit breaks.

Schema, rollout, and the applier live in [`agentic_os/generators/generate_git_workflow.py`](../agentic_os/generators/generate_git_workflow.py).
