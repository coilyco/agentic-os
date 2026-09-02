from __future__ import annotations

from agentic_os.prod_install_ref import resolve_release_ref


def _fetcher(branch_sha: str, tags: list[dict[str, object]]):
    def fetch(url: str) -> object:
        if url.endswith("/branches/release"):
            return {"commit": {"id": branch_sha}}
        if "/tags?" in url:
            return tags
        raise AssertionError(f"unexpected URL: {url}")

    return fetch


def test_guard_uses_the_tag_on_release_not_a_newer_staging_tag() -> None:
    fetch = _fetcher(
        "promoted",
        [
            {"name": "v0.130.0", "commit": {"sha": "staging"}},
            {"name": "v0.129.0", "commit": {"sha": "promoted"}},
        ],
    )

    assert resolve_release_ref("guard", fetch_json=fetch) == "v0.129.0"
    assert resolve_release_ref("umbra", fetch_json=fetch) == "v0.129.0"


def test_guard_uses_the_generated_tag_on_release() -> None:
    fetch = _fetcher(
        "promoted",
        [{"name": "v0.860.0", "commit": {"sha": "promoted"}}],
    )

    assert resolve_release_ref("guard", fetch_json=fetch) == "v0.860.0"


def test_aos_uses_its_independent_generated_tag() -> None:
    fetch = _fetcher(
        "promoted",
        [
            {"name": "v0.262.0", "commit": {"sha": "promoted"}},
            {"name": "aos-v0.130.0", "commit": {"sha": "promoted"}},
        ],
    )

    assert resolve_release_ref("aos", fetch_json=fetch) == "aos-v0.130.0"


def test_literal_release_is_the_fallback_when_no_tag_is_available() -> None:
    fetch = _fetcher(
        "promoted",
        [{"name": "v0.860.0", "commit": {"sha": "other"}}],
    )

    assert resolve_release_ref("guard", fetch_json=fetch) == "release"


def test_literal_release_is_the_fallback_when_forgejo_is_unavailable() -> None:
    def unavailable(url: str) -> object:
        raise OSError(url)

    assert resolve_release_ref("aos", fetch_json=unavailable) == "release"
