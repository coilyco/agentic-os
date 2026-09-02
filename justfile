# Per-repo task manifest. Run `just` (or `just --list`) to see every verb.
#
# Recipes take trailing arguments directly: `just aos-say hello`, where the
# retired form was `ward exec aos-say -- hello`.
#
# One line of comment per recipe on purpose: just reads only the LAST comment
# line above a recipe, so a wrapped description silently truncates to its tail.
#
# `ward exec` is retired. `.ward/ward.yaml` survives carrying catalog metadata
# only, because the catalog hooks upstream in agentic-os pin that exact path.

set positional-arguments

# Default target: list every available recipe.
default:
    @just --list --unsorted

# Canonical short agent-id generator over the dictatable alphabet (lowercase 2-letter+2-digit, e.g. `ab85`). Bare invocation prints a fresh `secrets`-backed id (`-n 5` for more); `--seed <s>` prints the deterministic contract id for a seed, `--org <forgejo-org>` prints the short container token, `--emit-vectors` regenerates agent_id_vectors.json. The cross-repo naming primitive ward and umbra's Go port build against. See docs/build-output-is-not-content.md.
agent-id *ARGS:
    @uv run python -m agentic_os.agent_id "$@"

# Inventory AGENTS-family sources across the committed substrate and an infrastructure-supplied managed-repo manifest. Emits stable Markdown or JSON with repository provenance, paragraph hashes, clipping candidates, and active role/harness cascades without copying doctrine text. See docs/agents-context-inventory.md.
agents-context-inventory *ARGS:
    @uv run python -m agentic_os.agents_context_inventory "$@"

# Validate the shared kitty configuration against the installed kitty.
kitty-config-check *ARGS:
    @kitty --config kitty/kitty.conf +runpy 'raise SystemExit(0)' "$@"

# Compile the standalone Go `aos` container launcher.
aos-build *ARGS:
    @go build -C aos-cli ./... "$@"

# Remove the local standalone Go `aos` build artifact.
aos-clean *ARGS:
    @go clean -C aos-cli "$@"

# Render the always-composed, always-guarded Ward launch without starting Docker. Pass the Ward issue or freeform arguments as trailing arguments.
aos-composition-dry-run *ARGS:
    @./aos-cli/aos --agent codex --role platform --warded --dry-run -- "$@"

# Materialize the full local context bundle and render Ward's launch without starting an agent container. Pass the Ward issue or freeform arguments as trailing arguments.
aos-composition-smoke *ARGS:
    @bash scripts/aos-composition-smoke.sh "$@"

# Launch the full local image through `aos`, compose the platform Codex HOME, verify its container-boundary defaults, hydrate substrate, and print the in-container Codex version.
aos-container-smoke *ARGS:
    @go run -C aos-cli . --role platform --layout codex --image agentic-os:aos-local acompose -- sh testdata/codex-smoke.sh "$@"

# Launch the full local image as the frontend Codex variant and verify its projected AGENTS.md briefing, selected composed skill, entry-point promotion, and role isolation.
aos-frontend-smoke *ARGS:
    @./aos-cli/aos --role frontend --layout codex --image agentic-os:aos-local --auth=false --no-substrate acompose -- sh scripts/aos-frontend-smoke.sh "$@"

# Render the default standalone composed-container launch without starting Docker.
aos-dry-run *ARGS:
    @./aos-cli/aos --role platform --dry-run acompose -- codex --version "$@"

# Format the standalone Go `aos` launcher.
aos-fmt *ARGS:
    @gofmt -w aos-cli "$@"

# Build the five local language images and fan them into agentic-os:aos-local for standalone `aos` smoke testing.
aos-image-build *ARGS:
    @env TAG=aos-local docker buildx bake --progress=plain --file docker/dev-base/docker-bake.hcl "$@"

# Install the standalone Go `aos` launcher into GOBIN.
aos-install *ARGS:
    @go install -C aos-cli . "$@"

# Run Go static analysis for the standalone `aos` launcher.
aos-lint *ARGS:
    @go vet -C aos-cli ./... "$@"

# Cross-compile version-stamped `aos` release binaries and checksums from the owning target manifest.
aos-release-build *ARGS:
    @sh scripts/aos-release-build.sh "$@"

# Verify `aos` release checksums, package metadata, and the native version-stamped binary.
aos-release-check *ARGS:
    @sh scripts/check-aos-release.sh "$@"

# Render the `aos` Homebrew formula and Scoop manifest from release binaries.
aos-release-package *ARGS:
    @sh scripts/render-aos-packaging.sh "$@"

# Ask one composed role a real question through cloud Codex or Goose with a local model, enforce a bounded run, and require the response to confirm its loaded role. Usage: `just aos-role-question cloud|local ROLE [MODEL]`.
aos-role-question *ARGS:
    @sh scripts/aos-role-question.sh "$@"

# Speak short status text with the Mac-integrated `aos-say` client. On Darwin it calls `/usr/bin/say` directly, and on other hosts it forwards one JSON request to the configured relay. Use `just aos-say relay` for the relay entrypoint.
aos-say *ARGS:
    @go run -C aos-say . "$@"

# Run the `aos-say` Go test suite.
aos-say-test *ARGS:
    @go test -C aos-say ./... "$@"

# Launch the selected agent in one standalone AOS container with composed context and the aosguard skill, then print the agent version.
aos-standalone-composition-smoke *ARGS:
    @./aos-cli/aos --agent codex --role platform --image agentic-os:aos-local --auth=false -- --version "$@"

# Run the standalone Go `aos` launcher test suite.
aos-test *ARGS:
    @go test -C aos-cli -timeout 30m ./... "$@"

# Reconcile the standalone Go `aos` launcher's module dependencies.
aos-tidy *ARGS:
    @go mod tidy -C aos-cli "$@"

# Materialize the standalone aosguard operator CLI and native generated skill from the independent .umbra snapshot.
aosguard-build *ARGS:
    @umbra --project-root .umbra/guardfiles --skills-out dist/skills build --out dist/aosguard "$@"
    @uv run python -m agentic_os.generators.generate_aosguard_skills --skills-root dist/skills

# Refresh aosguard's vendored API snapshot and frozen umbra dependency graph with the packaged umbra driver. Pass umbra lock flags as trailing arguments.
aosguard-lock *ARGS:
    @sh scripts/aosguard-lock.sh "$@"

# Format the native AOSguard release wrapper.
aosguard-release-fmt *ARGS:
    @gofmt -w aosguard-release "$@"

# Materialize and run aosguard from the independent .umbra snapshot, passing arguments through.
aosguard-run *ARGS:
    @umbra --project-root .umbra/guardfiles run -- "$@"

# Launch one composed agent session in its own branded kitty window. Bare invocation picks a role; pass role and seat positionally, and harness arguments after `--`. `--list` prints the live roster, `--dry-run` inspects without opening a window. See docs/aterm.md.
aterm *ARGS:
    @go run -C aterm . "$@"

# Write one macOS .app launcher per live role into ~/Applications, so a role opens from Spotlight or the Dock. `--dry-run` prints the bundles and their launcher scripts without writing. Pass `--icon <file.icns>` to give every bundle the same icon. See docs/aterm.md.
aterm-bundles *ARGS:
    @go run -C aterm . bundles "$@"

# Compile the branded session launcher.
aterm-build *ARGS:
    @go build -C aterm ./... "$@"

# Format the branded session launcher.
aterm-fmt *ARGS:
    @gofmt -w aterm "$@"

# Run Go static analysis for the branded session launcher.
aterm-lint *ARGS:
    @go vet -C aterm ./... "$@"

# Re-render the shipped sound-mark samples from the live Agent Compose roster into aterm/sounds/. Commit the result: each sample is meant to be auditioned and rejected by ear before it ships. See docs/aterm.md.
aterm-sounds *ARGS:
    @cd aterm && go run ./soundgen sounds "$@"

# Split the current aterm session's kitty window beside a command, or put it back. `pane on -- <cmd>` splits and moves the role creature clear, `pane off` restores it and closes the pane. Needs a kitty with remote control listening. See docs/aterm-pane.md.
aterm-pane *ARGS:
    @go run -C aterm . pane "$@"

# Walk the live Agent Compose roster and assert aterm still fits it: every launchable seat resolves, every unlaunchable one refuses, no shipped overlay field is discarded, and every timbre has a sample. Fails rather than skips when agent-compose is missing. See docs/aterm.md.
aterm-contract *ARGS:
    @ATERM_LIVE_ROSTER=1 go test -C aterm -run TestLive -v . "$@"

# Run the branded session launcher tests.
aterm-test *ARGS:
    @go test -C aterm ./... "$@"

# Reconcile the branded session launcher's Go module metadata.
aterm-tidy *ARGS:
    @go mod tidy -C aterm "$@"

# Roll out the canonical pre-commit hook block to every consumer repo under ~/projects/<org>/* (all org dirs). Idempotent.
apply-agentic-os-hooks *ARGS:
    @uv run python scripts/apply-agentic-os-hooks.py "$@"

# Inject or refresh the managed workspace-pointer block in each managed repo's AGENTS.md. Org-aware, idempotent. --dry-run to preview.
apply-agents-pointer *ARGS:
    @uv run python scripts/apply-agents-pointer.py "$@"

# Converge public-safe Claude Code preferences without clobbering local settings.
apply-base-claude-settings *ARGS:
    @python3 scripts/apply-base-claude-settings.py "$@"

# Repoint this host's shell entry symlinks (~/.zshrc, ~/.bashrc, gpg-ssm) and the git settings naming them (gpg.program) at the canonical agentic-os files. Idempotent; pass -- --check to fail on drift. Refuses to run from a native-session checkout.
apply-shell-links *ARGS:
    @uv run python scripts/apply-shell-links.py "$@"

# Audit every repo under ~/projects/<org>/* for presence of the canonical agentic-os hook block.
audit-pre-commit-coverage *ARGS:
    @uv run python scripts/audit-pre-commit-coverage.py "$@"

# Lint ~/.aws/config (or $AWS_CONFIG_FILE) for the [profile default] trap the AWS SDK silently ignores, which surfaces later as a cryptic NoRegion from SSM/S3. No-op when the file is absent. A machine-env check, not repo content, so it is ward-fenced rather than a commit-path hook.
aws-config *ARGS:
    @uv run python -m agentic_os.pre_commit.check_aws_config "$@"

# Report installed harness load points or capture one harness-neutral role bundle with `--role ROLE --snapshot FILE`; add repeatable `--additional-provider ID=PATH` inputs or `--compare FILE` for a deterministic component delta. `--immediate REPO` / `--peripheral REPO` walk working-dir/reference clones as reachable tiers. On-demand by design, not a pre-commit hook. See docs/context-budget.md.
context-budget *ARGS:
    @uv run python -m agentic_os.pre_commit.check_context_budget "$@"

# Build the parallel Ubuntu language payloads and full fan-in locally, loading agentic-os:dev-base-local. Language outputs remain cache-only build artifacts.
dev-base-build *ARGS:
    @env TAG=dev-base-local docker buildx bake --progress=plain --file docker/dev-base/docker-bake.hcl "$@"

# Run Docker buildx static checks against every language payload and the full fan-in image without building or publishing layers.
dev-base-check *ARGS:
    @bash scripts/dev-base-check.sh "$@"

# Fetch exact Forgejo 16 Actions log bytes: `just forgejo-actions-logs <owner> <repo> <run> [job] [attempt] [--max-bytes N]`. Omit job for a whole-run ZIP. Visible run/job indexes, explicit `id:<n>`, and exact job names are supported. Needs FORGEJO_TOKEN.
forgejo-actions-logs *ARGS:
    @uv run python -m agentic_os.forgejo_actions_logs "$@"

# Render the managed AGENTS.md workspace-pointer block for an org (--org) or the cwd repo. Single source of truth for the block shape.
gen-agents-pointer *ARGS:
    @uv run python -m agentic_os.generators.generate_agents_pointer "$@"

# Regenerate docs/catalog-caps-reference.md from the code-comments / documentation-layout validator constants. Run after changing a cap constant; check-caps-reference-drift fails on staleness. Docs and skills point at this render instead of restating a number.
gen-caps-reference *ARGS:
    @uv run python -m agentic_os.generators.generate_caps_reference "$@"

# Generate a repo-<name> pointer skill from repository metadata. Pipe `aosguard ops forgejo repo view --repo coilysiren/<name> --json` into it with `<name> --from-json -`, or pass --description/--topic. See docs/features-agents.md.
gen-repo-pointer-skill *ARGS:
    @uv run python -m agentic_os.generators.generate_repo_pointer_skill "$@"

# Regenerate agentic_os/seed_skills_data.py from seed: frontmatter on the composed coding-<lang> sources. Run after editing a seed block; check-seed-skills-drift fails on staleness. See docs/skill-discipline-authoring.md.
gen-seed-skills *ARGS:
    @uv run python -m agentic_os.generators.generate_seed_skills "$@"

# Run all pre-commit hooks against all files in the current repo.
pre-commit-all *ARGS:
    @pre-commit run --all-files "$@"

# Bump every third-party hook repo in .pre-commit-config.yaml to its latest tagged rev. Local hooks are unaffected (no rev).
pre-commit-autoupdate *ARGS:
    @pre-commit autoupdate "$@"

# Run only the code-comments hook across all files. Check comment-cap discipline at author time instead of waiting for the full commit gate.
pre-commit-code-comments *ARGS:
    @pre-commit run code-comments --all-files "$@"

# Run only the documentation-layout hook across all files. Check doc placement/size at author time instead of waiting for the full commit gate.
pre-commit-documentation-layout *ARGS:
    @pre-commit run documentation-layout --all-files "$@"

# Run only the issue-reference-guard hook across all files. Check issue-reference cleanup at author time instead of waiting for the full commit gate.
pre-commit-issue-reference-guard *ARGS:
    @pre-commit run issue-reference-guard --all-files --hook-stage manual "$@"

# Run only the ruff-check hook across all files. Check-only (no --fix).
pre-commit-ruff *ARGS:
    @pre-commit run ruff-check --all-files "$@"

# Run only the shellcheck hook across all shell files. Check-only.
pre-commit-shellcheck *ARGS:
    @pre-commit run shellcheck --all-files "$@"

# Run only the shellcheck hook against paths passed as trailing arguments, for focused validation when unrelated platform-specific files block the all-files hook.
pre-commit-shellcheck-files *ARGS:
    @pre-commit run shellcheck --files "$@"

# Resolve guard, Ward, or AOS to the immutable generated tag at its production release branch, falling back to the literal `release` ref.
prod-install-ref *ARGS:
    @python3 -m agentic_os.prod_install_ref "$@"

# Hand-cut an aos-precommit release: bump pyproject + FALLBACK_REV floor, create a signed aos-precommit-v* tag, and push. --bump major|minor|patch to override; --dry-run to preview.
release *ARGS:
    @uv run python scripts/release.py "$@"

# Remint the two-stage promote PAT on the coilyco-ops bot (write:repository + read:user - without read:user Forgejo drops the release enqueue, ward#1117) and overwrite SSM /forgejo/coilyco-ops/ci-release-token. Values stay in-process. `--dry-run` previews. Follow with `just sync-actions-secrets`.
remint-ci-release-token *ARGS:
    @uv run --with boto3 python scripts/remint-ci-release-token.py "$@"

# Remint the coilyco-ops read:package token, verify it against the Forgejo registry, and overwrite SSM /forgejo/coilyco-ops/registry-read-token. Values stay in-process. `--dry-run` previews.
remint-registry-read-token *ARGS:
    @uv run --with boto3 python scripts/remint-registry-read-token.py "$@"

# Run the exact repository CI gate, bootstrapping the pinned umbra release before pytest and the full pre-commit suite.
repo-test-gate *ARGS:
    @bash scripts/ci-command.sh bash scripts/ci/repo-test-gate.sh "$@"

# Validate SSM parameter paths against the /<org>/<repo>/<tier>/<tail> schema. `just ssm-path /coilysiren/backend/write/ts-authkey [more...]`, or `--stdin` for one path per line. A CLI validator (paths arrive as args, not repo files), so it is ward-fenced rather than a commit-path hook. See docs/release.md.
ssm-path *ARGS:
    @uv run python -m agentic_os.pre_commit.check_ssm_path "$@"

# Read-only sweep: run the documentation-layout check across every workspace git repo and report passing/failing/disabled plus per-repo violation detail.
sweep-documentation-layout *ARGS:
    @uv run python scripts/sweep-documentation-layout.py "$@"

# Run the full pre-commit hook set (pre-commit run --all-files) across every workspace git repo and report per-repo Passed/Failed plus failing hook ids. Reverts autofix mutations so trees stay clean.
sweep-precommit *ARGS:
    @uv run python scripts/sweep-precommit.py "$@"

# Sync Forgejo Actions secrets (Telegram alert creds, release PATs, package-repository writers, housecast's PyPI token, and deploy's pin-reconciler pair) from their SSM sources of truth. Entries are keyed `owner/repo`, so the mapping spans orgs: aos + ward + umbra + housecast under coilyco-flight-deck, plus coilyco-bridge/deploy. Values flow SSM -> Forgejo API in-process, never disk or argv. An attended operator supplies `FORGEJO_ADMIN_TOKEN`; `--dry-run` previews the mapping. See docs/release.md.
sync-actions-secrets *ARGS:
    @uv run python scripts/sync-actions-secrets.py "$@"

# Run the agentic_os Python test suite (pytest).
test *ARGS:
    @uv run pytest "$@"

# On Windows, update every installed app from the Flight Deck Scoop bucket. Ward self-updates through its audited path; pass -- -WhatIf to preview.
update-flight-deck-scoop *ARGS:
    @pwsh -NoProfile -File scripts/update-flight-deck-scoop.ps1 "$@"
