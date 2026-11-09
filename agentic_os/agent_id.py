"""Canonical short agent-id generator over the dictatable alphabet.

Two lowercase letters then two digits (`ab85`), over an alphabet that drops the
visually and phonetically ambiguous characters. The ward naming rewrite and the
umbra Go port build against this module, so the alphabet, the shape and the
seeded variant are a cross-language contract, mirrored byte for byte in
agent_id_vectors.json. Lowercase is the one divergence from the archived source.
"""

from __future__ import annotations

import hashlib
import json
import secrets
from pathlib import Path

# Dictatable alphabet, lowercased: confusable/homophone characters dropped
# (i l n o, 0 1 2 3). Ground truth + rationale in docs/build-output-is-not-content.md.
ID_LETTERS = "abcdefghjkmpqrstuvwxyz"
ID_DIGITS = "456789"
ID_ALPHABET = ID_LETTERS + ID_DIGITS
ID_LEN = 4
ID_LETTER_LEN = 2

_VECTORS_PATH = Path(__file__).with_name("agent_id_vectors.json")
_ORG_SHORTNAMES_PATH = Path(__file__).with_name("org_shortnames.json")


def new_id() -> str:
    """Return a fresh random id: 2 dictatable letters then 2 dictatable digits.

    `secrets`-backed and uniform over the allowed alphabet - safe to name an
    agent with. Not reproducible; use `seeded_id` for the parity contract.
    """
    letters = "".join(secrets.choice(ID_LETTERS) for _ in range(ID_LETTER_LEN))
    digits = "".join(secrets.choice(ID_DIGITS) for _ in range(ID_LEN - ID_LETTER_LEN))
    return letters + digits


def is_valid(raw: str) -> bool:
    """True when `raw` is already a canonical id (lowercase, correct shape)."""
    return (
        len(raw) == ID_LEN
        and all(c in ID_LETTERS for c in raw[:ID_LETTER_LEN])
        and all(c in ID_DIGITS for c in raw[ID_LETTER_LEN:])
    )


def normalize(raw: str) -> str:
    """Canonicalize a spoken/typed id (trim + lowercase), or raise ValueError.

    The dictated form may arrive upper- or mixed-case with surrounding
    whitespace; the canonical stored form is lowercase.
    """
    cid = raw.strip().lower()
    if not is_valid(cid):
        raise ValueError(f"not a canonical agent id: {raw!r}")
    return cid


def seeded_id(seed: str) -> str:
    """Deterministic id from a string seed - the cross-language parity anchor.

    Algorithm (portable byte-for-byte to Go and any language with sha256):
    take sha256(utf-8(seed)), then index the alphabet by the first four digest
    bytes modulo each sub-alphabet's length - bytes 0,1 into the letters,
    bytes 2,3 into the digits. This is NOT for real ids (use `new_id`); it
    exists only so `agent_id_vectors.json` pins a reproducible seed->id map a
    port can assert against.
    """
    digest = hashlib.sha256(seed.encode("utf-8")).digest()
    return (
        ID_LETTERS[digest[0] % len(ID_LETTERS)]
        + ID_LETTERS[digest[1] % len(ID_LETTERS)]
        + ID_DIGITS[digest[2] % len(ID_DIGITS)]
        + ID_DIGITS[digest[3] % len(ID_DIGITS)]
    )


def load_org_shortnames() -> dict:
    """Return the parsed org-shortname contract (`org_shortnames.json`)."""
    return json.loads(_ORG_SHORTNAMES_PATH.read_text(encoding="utf-8"))


def org_shortname(org: str) -> str:
    """Map a long org to its short container token.

    Explicit mappings win; an unmapped org falls back to its last `-` segment
    (`some-long-org` -> `org`), the documented default that keeps container and
    dozzle names short without a hardcoded entry per org.
    """
    data = load_org_shortnames()
    mapped = data.get("map", {}).get(org)
    if mapped is not None:
        return mapped
    return org.rsplit("-", 1)[-1]


def build_vectors() -> dict:
    """Build the shared test-vector object from this module's ground truth.

    The committed `agent_id_vectors.json` is exactly this, pretty-printed. The
    drift test regenerates and compares, so an alphabet or algorithm change that
    forgets to re-emit the file fails CI.
    """
    seeds = [str(n) for n in range(20)] + [
        "kai-server",
        "seed-contract",
        "the-quick-brown-fox",
        "",
    ]
    return {
        "note": (
            "Cross-language contract for the canonical agent-id generator. "
            "Ports must reproduce every seed->id below and the alphabets above, byte-for-byte. "
            "Regenerate with "
            "`python -m agentic_os.agent_id --emit-vectors`."
        ),
        "id_letters": ID_LETTERS,
        "id_digits": ID_DIGITS,
        "id_len": ID_LEN,
        "id_letter_len": ID_LETTER_LEN,
        "seed_algorithm": (
            "digest = sha256(utf8(seed)); "
            "id = id_letters[digest[0] % 22] + id_letters[digest[1] % 22] "
            "+ id_digits[digest[2] % 6] + id_digits[digest[3] % 6]"
        ),
        "vectors": [{"seed": s, "id": seeded_id(s)} for s in seeds],
    }


def _dumped_vectors() -> str:
    return json.dumps(build_vectors(), indent=2, ensure_ascii=False) + "\n"


def main(argv: list[str] | None = None) -> int:
    import argparse

    parser = argparse.ArgumentParser(description="Canonical short agent-id generator.")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--seed", help="print the deterministic id for SEED and exit")
    group.add_argument(
        "--org", help="print the short container token for an org and exit"
    )
    group.add_argument(
        "--emit-vectors",
        action="store_true",
        help="write agent_id_vectors.json from this module and exit",
    )
    parser.add_argument(
        "-n", type=int, default=1, help="how many fresh ids to print (default 1)"
    )
    args = parser.parse_args(argv)

    if args.emit_vectors:
        _VECTORS_PATH.write_text(_dumped_vectors(), encoding="utf-8")
        print(f"wrote {_VECTORS_PATH}")
        return 0
    if args.seed is not None:
        print(seeded_id(args.seed))
        return 0
    if args.org is not None:
        print(org_shortname(args.org))
        return 0
    for _ in range(max(1, args.n)):
        print(new_id())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
