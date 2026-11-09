from __future__ import annotations

import pytest

from agentic_os import prod_install_ref
from agentic_os.prod_install_ref import ReleaseRefError, resolve_release_ref

API = "https://api.github.com/repos/coilyco"


def _ref(name: str, sha: str, kind: str = "commit") -> dict[str, object]:
    return {"ref": f"refs/tags/{name}", "object": {"type": kind, "sha": sha}}


def _fetcher(
    repo: str,
    branch_sha: str,
    refs: object,
    annotated: dict[str, str] | None = None,
):
    """A GitHub stand-in: the release branch, matching-refs, and tag objects."""
    seen: list[str] = []

    def fetch(url: str) -> object:
        seen.append(url)
        if url == f"{API}/{repo}/branches/release":
            return {"commit": {"sha": branch_sha}}
        if "/git/matching-refs/tags/" in url:
            return refs
        for tag_sha, commit in (annotated or {}).items():
            if url == f"{API}/{repo}/git/tags/{tag_sha}":
                return {"object": {"type": "commit", "sha": commit}}
        raise AssertionError(f"unexpected URL: {url}")

    fetch.seen = seen  # type: ignore[attr-defined]
    return fetch


def test_guard_uses_the_tag_on_release_not_a_newer_staging_tag() -> None:
    fetch = _fetcher(
        "umbra",
        "promoted",
        [_ref("v0.130.0", "staging"), _ref("v0.129.0", "promoted")],
    )

    assert resolve_release_ref("guard", fetch_json=fetch) == "v0.129.0"
    assert resolve_release_ref("umbra", fetch_json=fetch) == "v0.129.0"


def test_the_matching_tag_is_found_when_it_is_not_first_in_the_listing() -> None:
    others = [_ref(f"aos-v0.{n}.0", f"old{n}") for n in range(1, 40)]
    fetch = _fetcher("agentic-os", "promoted", others + [_ref("aos-v0.40.0", "promoted")])

    assert resolve_release_ref("aos", fetch_json=fetch) == "aos-v0.40.0"


def test_aos_asks_only_for_its_own_tag_prefix() -> None:
    fetch = _fetcher(
        "agentic-os",
        "promoted",
        [
            _ref("aos-precommit-v0.130.0", "promoted"),
            _ref("aos-v0.130.0", "promoted"),
        ],
    )

    assert resolve_release_ref("aos", fetch_json=fetch) == "aos-v0.130.0"
    assert any(url.endswith("/git/matching-refs/tags/aos-v") for url in fetch.seen)


def test_the_highest_version_wins_when_two_tags_share_the_commit() -> None:
    fetch = _fetcher(
        "umbra",
        "promoted",
        [_ref("v0.9.0", "promoted"), _ref("v0.10.0", "promoted")],
    )

    assert resolve_release_ref("guard", fetch_json=fetch) == "v0.10.0"


def test_an_annotated_tag_is_followed_to_its_commit() -> None:
    fetch = _fetcher(
        "umbra",
        "promoted",
        [_ref("v0.7.0", "tagobject", kind="tag")],
        annotated={"tagobject": "promoted"},
    )

    assert resolve_release_ref("guard", fetch_json=fetch) == "v0.7.0"


def test_no_tag_at_the_release_head_is_an_error_never_the_literal_release() -> None:
    fetch = _fetcher("umbra", "promoted", [_ref("v0.860.0", "other")])

    with pytest.raises(ReleaseRefError, match="promoted"):
        resolve_release_ref("guard", fetch_json=fetch)


@pytest.mark.parametrize(
    "refs",
    [{"message": "Not Found"}, [{"object": {}}], [_ref("v1.0.0", None)]],  # type: ignore[arg-type]
)
def test_a_malformed_listing_is_an_error(refs: object) -> None:
    fetch = _fetcher("umbra", "promoted", refs)

    with pytest.raises(ReleaseRefError):
        resolve_release_ref("guard", fetch_json=fetch)


def test_an_unreachable_api_is_an_error() -> None:
    def unavailable(url: str) -> object:
        raise OSError(url)

    with pytest.raises(ReleaseRefError):
        resolve_release_ref("aos", fetch_json=unavailable)


def test_the_command_exits_non_zero_and_prints_no_ref_on_failure(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    def unavailable(url: str) -> object:
        raise OSError(url)

    monkeypatch.setattr(prod_install_ref, "_get_json", unavailable)

    assert prod_install_ref.main(["aos"]) == 1
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "cannot read agentic-os release refs" in captured.err


def test_the_command_prints_the_tag_on_success(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    fetch = _fetcher("agentic-os", "promoted", [_ref("aos-v0.5.0", "promoted")])
    monkeypatch.setattr(prod_install_ref, "_get_json", fetch)

    assert prod_install_ref.main(["aos"]) == 0
    assert capsys.readouterr().out == "aos-v0.5.0\n"
