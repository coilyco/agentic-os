"""Tests for agentic_os.pre_commit.check_brand_case.

The brand name is lowercase in prose, sentence-initial included. Identifiers
are not prose, so slugs, hostnames, code spans, fenced blocks and link targets
carry no opinion and must not be flagged. Half of these are negative controls,
because a case checker that matches nothing passes every file silently.
"""
from __future__ import annotations

from pathlib import Path

from agentic_os.pre_commit.check_brand_case import scan_text

DOC = Path("docs/x.md")


def test_flags_title_case() -> None:
    found = scan_text(DOC, "Coilyco ships the thing.")
    assert len(found) == 1
    assert found[0].found == "Coilyco"


def test_flags_camel_case() -> None:
    assert len(scan_text(DOC, "the CoilyCo house style")) == 1


def test_flags_upper_case() -> None:
    assert len(scan_text(DOC, "COILYCO INTERNAL")) == 1


def test_flags_sentence_initial() -> None:
    """The whole point: ordinary sentence casing is what breaks the name."""
    assert len(scan_text(DOC, "Coilyco is a platform.")) == 1


def test_accepts_the_canonical_form() -> None:
    assert scan_text(DOC, "coilyco ships the thing. coilyco again.") == []


def test_ignores_a_slug() -> None:
    assert scan_text(DOC, "the coilyco-flight-deck org and coilyco-bridge") == []


def test_ignores_a_hostname() -> None:
    assert scan_text(DOC, "hosted at https://coilyco.ai/Coilyco and elsewhere") == []


def test_ignores_a_link_target() -> None:
    assert scan_text(DOC, "see [the deck](https://x.test/Coilyco) for more") == []


def test_ignores_a_code_span() -> None:
    assert scan_text(DOC, "the `Coilyco Teable` connector is addressed by name") == []


def test_ignores_a_fenced_block() -> None:
    text = "\n".join(["prose here", "```", "CN=Coilyco Code Signing", "```", "more prose"])
    assert scan_text(DOC, text) == []


def test_fence_reopens_so_later_prose_is_still_checked() -> None:
    text = "\n".join(["```", "Coilyco inside", "```", "Coilyco outside"])
    found = scan_text(DOC, text)
    assert len(found) == 1
    assert found[0].line == 4


def test_allowlist_exempts_a_literal_identifier() -> None:
    line = "the cert subject is CN=Coilyco Code Signing, RSA 4096"
    assert len(scan_text(DOC, line)) == 1
    assert scan_text(DOC, line, frozenset({"CN=Coilyco Code Signing"})) == []


def test_reports_line_and_column() -> None:
    found = scan_text(DOC, "\n".join(["fine here", "and then Coilyco lands"]))
    assert (found[0].line, found[0].column) == (2, 10)


def test_a_file_without_the_name_is_untouched() -> None:
    assert scan_text(DOC, "nothing to see, no brand at all") == []
