# The guarded `gh`

GitHub is canonical for every coilyco repo (COI-3083), so a pull request is a GitHub pull request and `gh` is the verb surface for it.

## Status

* **The source is gone.** `.umbra/shims/gh/gh.kdl` and `.umbra/shims-seat/gh/gh.kdl` were deleted on `main` by two commits titled `Delete .umbra/shims/gh/gh.kdl` and `Delete .umbra/shims-seat/gh/gh.kdl`. The `gh-shim-build`, `gh-shim-build-seat` and `gh-seat-guardfile` verbs in the `justfile` still name them, and that code cleanup is COI-2931.
* **An installed copy can outlive the source.** A shim built from the old guardfile can still sit first on an agent session's PATH. It refuses pull-request writes outside its old per-repo allowlist, and it stays until COI-3077 merges and the host converge removes it. A refusal from it is that stale allowlist, not a repo rule. Report it and keep the branch pushed, and never retry the verb in another shape.
* **After removal** `gh` is GitHub's own binary. The rest of this page describes what a seat runs against it.

## The seat pull-request lifecycle on GitHub (COI-2933)

Every verb takes `--repo coilyco/<repo>`, and `gh pr` verbs take a number or branch. These were run through the old guard on 2026-10-27, each exiting 0.

* **Open** - `gh pr create --repo coilyco/<repo> --head BRANCH --base main --title T --body-file FILE`. Pass text by `--body-file`, since the old guard refused `(`, `{` and a backslash in any argument.
* **Update from main** - `gh pr update-branch N --repo coilyco/<repo>`. It merges base into the head branch and never forces.
* **Merge** - `gh pr merge N --repo coilyco/<repo> --squash --delete-branch`.
* **Read a failing run** - `gh run list --repo coilyco/<repo>`, then `gh run view ID --repo coilyco/<repo> --log-failed`. A run past GitHub's retention answers HTTP 410.
* **Rerun** - `gh run rerun ID --repo coilyco/<repo> --failed`.
* **Behind base** - GitHub refuses a merge as behind only when branch protection requires an up-to-date branch. The one repo checked had none, and a behind branch merged cleanly. The remedy is **inferred and not observed**: on a refusal, run `gh pr update-branch`, wait for checks, then merge.
* **Push** - over the SSH remote. An HTTPS clone has no credential helper.

## The token broker (COI-2933)

`aterm github-token --repo coilyco/NAME` prints one coilyco-ops App installation token and nothing else. The aterm daemon mints it, so a seat never reads the App key: it comes from the daemon's `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY_BASE64`, else SSM through `aws`. The `github_token` frame (feature `github-token`) names the caller by `ATERM_SESSION_TOKEN`, and a browser client or an unknown token is refused.

* **One owner, one repo.** Only `coilyco`, and the mint requests `repositories: [NAME]`, so a leaked token reaches one repo for about an hour. A token is cached until five minutes before expiry.
* **Permissions** default to contents write, pull_requests write, metadata read, as infrastructure's `github_app_token.py`. `ATERM_GITHUB_PERMISSIONS=actions=write,contents=write,...` widens them, and GitHub refuses the whole mint if the installation holds less, so widen only after the App grants it.
* **`aterm gh-exec <gh args>`** takes the last `--repo` (else a `gh api repos/OWNER/NAME` path, else the one repo every github.com remote names), asks the broker, and runs the real `gh` with `GH_TOKEN` for that repo and an empty private `GH_CONFIG_DIR`. Any other `GH_*` token variable is dropped. No repo, another owner, or a broker failure refuses before `gh` starts and never falls back to the host login.
* **Outside coilyco it is anonymous and read-only.** A repo under another owner, or a call naming no repo, runs `gh` with no token for `pr`, `issue`, `run`, `workflow`, `repo` and `release` read verbs, `search` and `api`. Anything else refuses and says why.
* **Not a floor.** It leaves the real `gh` and the host login reachable (COI-3013). Narrowing the host token is COI-2935.

The seam is `githubBroker.token` in `aterm/github_token.go`, and `ATERM_GITHUB_API` points it at a stand-in API for tests.

## What a PATH shim never bought

`shell/common.sh` prepends `~/.local/umbra/shims` only when `AOS_NATIVE_SESSION` is set, so an agent session met the guarded `gh` and an ordinary interactive shell kept the real one. umbra's own doc states the limit: **a PATH shim is not an enforcement floor.** A same-user agent that spells `/opt/homebrew/bin/gh` walks around it. The floor would be ownership plus no passwordless sudo, which belongs to host convergence.

`UMBRA_IDENTIFY=1 gh` is the only surface that says what stands on the name while a shim is installed.

`gh issue create` forwards. The GitHub issue queue is for external contributors and fleet work goes to Linear.
