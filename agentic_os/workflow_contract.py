"""The runner-label and host contract for ``.github/workflows`` files.

``workflow_contract.json`` is the one owner. The checker rejects against it and
the porting script translates with it, so neither restates a label. See
docs/dev-container.md.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

CONTRACT_PATH = Path(__file__).resolve().parent / "workflow_contract.json"


@dataclass(frozen=True)
class Contract:
    hosted: tuple[str, ...]
    forgejo_to_github: dict[str, str]
    needs_redesign: dict[str, str]
    forbidden_hosts: tuple[str, ...]
    setup_ci_uses: str
    setup_ci_ref: str

    @property
    def setup_ci(self) -> str:
        return f"{self.setup_ci_uses}@{self.setup_ci_ref}"


def load_contract(path: Path | None = None) -> Contract:
    data = json.loads((path or CONTRACT_PATH).read_text(encoding="utf-8"))
    return Contract(
        hosted=tuple(data["hosted"]),
        forgejo_to_github=dict(data["forgejo_to_github"]),
        needs_redesign=dict(data["needs_redesign"]),
        forbidden_hosts=tuple(data["forbidden_hosts"]),
        setup_ci_uses=data["setup_ci"]["uses"],
        setup_ci_ref=data["setup_ci"]["ref"],
    )
