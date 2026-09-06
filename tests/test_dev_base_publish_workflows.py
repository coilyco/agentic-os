"""Tests for the independent dev-base publish workflow surface."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PUBLISH = ROOT / ".forgejo" / "workflows" / "dev-base-publish.yml"
RELEASE = ROOT / ".forgejo" / "workflows" / "release.yml"
PUBLISH_ACTION = ROOT / "actions" / "publish-dev-base" / "action.yml"
PUBLISH_IMAGE = (
    ROOT / "actions" / "publish-dev-base" / "scripts" / "publish-image.sh"
)
CHECKPOINT = (
    ROOT / "actions" / "publish-dev-base" / "scripts" / "checkpoint.sh"
)
INSTALL_DOCKER = (
    ROOT
    / "actions"
    / "publish-dev-base"
    / "scripts"
    / "install-docker.sh"
)
VERIFY_FULL = (
    ROOT
    / "actions"
    / "publish-dev-base"
    / "scripts"
    / "verify-full.sh"
)


def _run_checkpoint(tmp_path: Path, **overrides: str) -> tuple[subprocess.CompletedProcess[str], str]:
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    docker = bin_dir / "docker"
    docker.write_text(
        """#!/usr/bin/env bash
ref=${@: -1}
case "$ref" in
  *:lang-node-missing) exit 1 ;;
  *:lang-node-existing|*:release) printf 'Digest: sha256:target\\n' ;;
  *:draft-source) printf 'Digest: sha256:source\\n' ;;
  *) exit 1 ;;
esac
""",
        encoding="utf-8",
    )
    docker.chmod(0o755)
    output = tmp_path / "github-output"
    env = os.environ | {
        "PATH": f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
        "GITHUB_OUTPUT": str(output),
        "IMAGE_BASE": "registry.example.invalid/org/image",
        "MODE": "build",
        "TIER": "lang-node",
        "TAG": "missing",
        "ALIAS": "",
        "EXTRA_ALIASES": "",
        "SOURCE_TAG": "",
        **overrides,
    }
    completed = subprocess.run(
        ["bash", str(CHECKPOINT)],
        check=False,
        capture_output=True,
        env=env,
        text=True,
    )
    return completed, output.read_text(encoding="utf-8") if output.exists() else ""


def _assert_alert_steps_are_non_blocking(text: str) -> None:
    lines = text.splitlines()
    alert_indexes = [
        index
        for index, line in enumerate(lines)
        if line.strip().startswith("- name: Alert Telegram on")
    ]
    assert alert_indexes
    for index in alert_indexes:
        assert any(
            "continue-on-error: true" in line for line in lines[index : index + 8]
        )


def test_workflows_publish_parallel_payloads_then_full_and_release_only_full() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")
    language_tiers = {
        line.strip().removeprefix("- ")
        for line in publish.splitlines()
        if line.strip().startswith("- lang-")
    }

    assert "publish-language-payloads:" in publish
    assert "publish-full:" in publish
    assert "retag-languages:" not in release
    assert "retag-full:" in release
    assert language_tiers
    for tier in language_tiers:
        assert f"          - {tier}" not in release
    # Derived, not restated: every language tier fans out at once, so a sixth
    # language must widen the matrix rather than silently queue behind the rest.
    assert f"max-parallel: {len(language_tiers)}" in publish
    assert "tier: ${{ matrix.tier }}" in publish
    assert "tier: ${{ matrix.tier }}" not in release
    assert "needs: [plan-draft, publish-language-payloads]" in publish
    assert "needs: [plan-release, retag-full]" in release
    # Two payload attempts (agentic-os#987), the full image, the payload
    # existence check, and the post-release verify (agentic-os#1032).
    assert publish.count("uses: ./actions/publish-dev-base") == 5
    assert release.count("uses: ./actions/publish-dev-base") == 1


def test_language_payload_caches_are_stable_across_commits_and_runners() -> None:
    script = PUBLISH_IMAGE.read_text(encoding="utf-8")

    assert 'cache_ref="${IMAGE_BASE}:$(tier_tag "$TIER" buildcache)"' in script
    assert '--cache-from "type=registry,ref=${cache_ref}"' in script
    assert '--cache-to "type=registry,ref=${cache_ref},mode=max,ignore-error=true"' in script
    assert "${TAG}" not in script.split('cache_ref=', 1)[1].split("\n", 1)[0]
    assert 'if [[ "$TIER" = lang-* ]]' in script


def test_promotion_fails_closed_for_language_payloads() -> None:
    for path in (PUBLISH_IMAGE, CHECKPOINT):
        text = path.read_text(encoding="utf-8")
        assert '"$TIER" != full' in text
        assert "only the full dev-base image is a promotable release product" in text


def test_checkpoint_treats_an_absent_draft_manifest_as_a_build_miss(tmp_path: Path) -> None:
    completed, output = _run_checkpoint(tmp_path)

    assert completed.returncode == 0, completed.stderr
    assert output == "skip_publish=false\n"


def test_checkpoint_skips_an_existing_build_manifest(tmp_path: Path) -> None:
    completed, output = _run_checkpoint(tmp_path, TAG="existing")

    assert completed.returncode == 0, completed.stderr
    assert output == "skip_publish=true\n"


def test_checkpoint_rebuilds_a_mismatched_promotion_target(tmp_path: Path) -> None:
    completed, output = _run_checkpoint(
        tmp_path,
        MODE="promote",
        TIER="full",
        TAG="release",
        SOURCE_TAG="draft-source",
    )

    assert completed.returncode == 0, completed.stderr
    assert output == "skip_publish=false\n"


def test_publish_workflow_keeps_resume_inputs_and_non_blocking_alerts() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    for needle in ("sha:", "tag:", "continue-on-error: true"):
        assert needle in publish
    for needle in ("sha:", "tag:", "source-tag:", "continue-on-error: true"):
        assert needle in release
    assert "github.event.inputs.tier == 'all'" in publish
    assert "github.event.inputs.tier == 'full'" in publish
    _assert_alert_steps_are_non_blocking(publish)
    _assert_alert_steps_are_non_blocking(release)


def test_full_image_build_uses_the_dedicated_runner() -> None:
    publish = PUBLISH.read_text(encoding="utf-8")

    # The verify job needs the same runner: it reads the registry through docker.
    # plan-draft joined them, so four rather than three.
    assert publish.count("runs-on: docker-build") == 4
    assert "publish-language-payloads:" in publish
    assert "publish-full:" in publish
    assert "needs: [plan-draft, publish-language-payloads]" in publish
    plan_draft = publish.split("\n  plan-draft:\n", 1)[1].split(
        "\n  publish-language-payloads:\n", 1
    )[0]
    # Exact, not a substring of docker-build, and no container: a container here
    # made the publish graph depend on the image it publishes.
    assert "runs-on: docker-build" in plan_draft
    assert "container:" not in plan_draft


def test_shared_docker_bootstrap_retries_downloads_without_partial_files() -> None:
    script = INSTALL_DOCKER.read_text(encoding="utf-8")

    assert script.count("curl \\") == 1
    for option in (
        "--retry ",
        "--retry-all-errors",
        "--retry-delay ",
        "--remove-on-error",
        "-fsSL",
    ):
        assert option in script
    for source in (
        "download.docker.com/linux/static/stable/x86_64/",
        "download.docker.com/linux/static/stable/x86_64/docker-${docker_cli_ver}.tgz",
        "github.com/docker/buildx/releases/download/${BUILDX_VERSION}/",
    ):
        assert source in script
    assert "actions/install-docker-buildx" not in PUBLISH_ACTION.read_text(
        encoding="utf-8"
    )


def test_promote_verifies_the_draft_before_the_release_tags_move() -> None:
    action = PUBLISH_ACTION.read_text(encoding="utf-8")
    steps = [
        block.splitlines()[0].strip()
        for block in action.split("\n    - name: ")[1:]
    ]
    promote_verify = steps.index(
        "Verify the promotion source before the release tags move"
    )
    promote_binfmt = steps.index(
        "Install binfmt so promote can verify the foreign architecture"
    )
    publish = steps.index("Publish or promote the image")

    # Promotion is the last thing the job does, so a failed verification never
    # leaves release and latest on an image that did not pass.
    assert promote_binfmt < promote_verify < publish
    promote_block = action.split(
        "\n    - name: Verify the promotion source before the release tags move", 1
    )[1].split("\n    - name:", 1)[0]
    assert "TAG: ${{ inputs.source-tag }}" in promote_block


def test_publish_action_verifies_every_toolchain_and_aosguard() -> None:
    action = PUBLISH_ACTION.read_text(encoding="utf-8")
    script = VERIFY_FULL.read_text(encoding="utf-8")

    assert "inputs:\n  tier:" in action
    assert "required: true" in action
    assert "Verify the full development surface on both architectures" in action
    assert "if: ${{ inputs.tier == 'full' && inputs.mode == 'build' }}" in action
    assert "if: ${{ inputs.tier == 'full' && inputs.mode == 'promote' }}" in action
    assert "scripts/verify-full.sh" in action
    assert 'image="${IMAGE_BASE}:${TAG}"' in script
    assert "PLATFORMS:-linux/amd64,linux/arm64" in script
    assert "PLATFORMS: ${{ inputs.platforms }}" in action
    for command in (
        "node --version",
        "go version",
        "dotnet --list-sdks",
        "cargo --version",
        "python --version",
        "aosguard --version",
        "test -s /opt/agentic-os/aosguard-skill/aosguard/SKILL.md",
        "test -s /opt/agentic-os/aosguard-skill/aosguard/references/commands.yaml",
    ):
        assert command in script


def test_a_payload_gets_one_bounded_retry() -> None:
    # One transient payload failure used to skip the whole release. agentic-os#987.
    text = PUBLISH.read_text(encoding="utf-8")
    payload = text[text.index("publish-language-payloads:") : text.index("publish-full:")]

    assert "id: payload" in payload
    assert "continue-on-error: true" in payload
    assert "steps.payload.outcome == 'failure'" in payload
    # Two attempts and one check. A third attempt would turn a real defect into
    # a slow failure, and the check is what keeps two attempts honest.
    assert payload.count("uses: ./actions/publish-dev-base") == 3
    assert "mode: verify" in payload


def test_the_retry_cannot_turn_a_loud_failure_into_a_silent_one() -> None:
    # continue-on-error swallows attempt one, so a retry that never fires would
    # leave the job green with nothing published. The registry settles it.
    text = PUBLISH.read_text(encoding="utf-8")
    payload = text[text.index("publish-language-payloads:") : text.index("publish-full:")]
    check = payload[payload.index("Assert the payload actually landed") :]

    # Unconditional on the payload steps' outcomes: it runs whether attempt one
    # passed, the retry rescued it, or the retry never fired at all.
    assert "steps.payload" not in check.split("- name: Alert")[0]
    assert "continue-on-error" not in check.split("- name: Alert")[0]


def test_the_retry_does_not_swallow_a_second_failure() -> None:
    # continue-on-error keeps attempt one from failing the job. The retry has no
    # such guard, so its failure still fails the job and still alerts.
    text = PUBLISH.read_text(encoding="utf-8")
    payload = text[text.index("publish-language-payloads:") : text.index("publish-full:")]
    retry = payload[payload.index("name: Retry the payload once") :]
    assert "continue-on-error" not in retry.split("- name: Alert")[0]
    assert "failure() && github.ref == 'refs/heads/release'" in payload


VERIFY_ALIAS = ROOT / "actions" / "publish-dev-base" / "scripts" / "verify-alias.sh"


def _run_verify_alias(tmp_path: Path, digests: dict[str, str], **env: str):
    """Run verify-alias.sh against a docker stub with a fixed tag -> digest map."""
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    cases = "\n".join(
        f'  *:{tag}) printf "Digest: {digest}\\\\n" ;;' for tag, digest in digests.items()
    )
    docker = bin_dir / "docker"
    docker.write_text(
        f'#!/usr/bin/env bash\nref=${{@: -1}}\ncase "$ref" in\n{cases}\n  *) exit 1 ;;\nesac\n',
        encoding="utf-8",
    )
    docker.chmod(0o755)
    return subprocess.run(
        ["bash", str(VERIFY_ALIAS)],
        check=False,
        capture_output=True,
        text=True,
        env=os.environ
        | {
            "PATH": f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
            "IMAGE_BASE": "registry.example.invalid/org/image",
            "TIER": "full",
            "TAG": "release",
            "ALIAS": "",
            "EXTRA_ALIASES": "latest",
            "SOURCE_TAG": "draft-abc",
            **env,
        },
    )


def test_a_moved_alias_passes() -> None:
    import tempfile

    with tempfile.TemporaryDirectory() as d:
        got = _run_verify_alias(
            Path(d),
            {"draft-abc": "sha256:built", "release": "sha256:built", "latest": "sha256:built"},
        )
    assert got.returncode == 0, got.stderr


def test_a_stale_alias_fails_loudly() -> None:
    # The defect: every job green, `release` still on the previous digest.
    # See agentic-os#1032.
    import tempfile

    with tempfile.TemporaryDirectory() as d:
        got = _run_verify_alias(
            Path(d),
            {"draft-abc": "sha256:built", "release": "sha256:previous", "latest": "sha256:built"},
        )
    assert got.returncode == 1
    assert "expected sha256:built" in got.stderr


def test_an_alias_that_does_not_resolve_fails() -> None:
    import tempfile

    with tempfile.TemporaryDirectory() as d:
        got = _run_verify_alias(Path(d), {"draft-abc": "sha256:built"})
    assert got.returncode == 1
    assert "The promotion did not happen" in got.stderr


def test_a_missing_source_is_an_error_not_a_pass() -> None:
    # Comparing against nothing must never read as agreement.
    import tempfile

    with tempfile.TemporaryDirectory() as d:
        got = _run_verify_alias(Path(d), {"release": "sha256:anything"})
    assert got.returncode == 1
    assert "nothing this run built" in got.stderr


def test_promote_asserts_and_the_caller_asserts_again() -> None:
    action = PUBLISH_ACTION.read_text(encoding="utf-8")
    publish = PUBLISH.read_text(encoding="utf-8")

    assert "inputs.mode == 'promote' || inputs.mode == 'verify'" in action
    # A reusable-workflow call reported success without running, so the caller
    # checks the registry rather than the job colour. See agentic-os#1032.
    assert "assert-release-moved:" in publish
    assert "needs: [plan-draft, publish-release]" in publish
    assert "mode: verify" in publish


def test_verifying_nothing_is_not_a_pass() -> None:
    # An empty tag list once exited 0 having checked nothing, which is the same
    # shape the script already refuses for an unresolvable source.
    import tempfile

    with tempfile.TemporaryDirectory() as d:
        got = _run_verify_alias(
            Path(d), {"draft-abc": "sha256:built"}, TAG="", ALIAS="", EXTRA_ALIASES=""
        )
    assert got.returncode == 2
    assert "Verifying nothing is not a pass" in got.stderr


def test_the_dispatch_guard_has_one_spelling() -> None:
    # Two spellings of the same condition lived in this file. A backstop must
    # not be guarded differently from the job it backstops.
    publish = PUBLISH.read_text(encoding="utf-8")
    assert "|| inputs.tier ==" not in publish
    # One per guarded step or job, derived rather than restated: adding a
    # guarded step must not be able to reintroduce the other spelling.
    assert publish.count("github.event.inputs.tier == 'all'") == publish.count(
        "github.event_name != 'workflow_dispatch'"
    )
