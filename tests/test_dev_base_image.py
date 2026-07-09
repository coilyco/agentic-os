"""Tests for the dev-base image contract."""
from __future__ import annotations

from pathlib import Path


DOCKERFILE = Path(__file__).resolve().parent.parent / "docker" / "dev-base" / "Dockerfile"


def test_dev_base_seeds_default_ward_home() -> None:
    text = DOCKERFILE.read_text(encoding="utf-8")

    assert "ARG WARD_AGENT_UID=1000" in text
    assert "ARG WARD_AGENT_GID=1000" in text
    assert "install -d -o \"${WARD_AGENT_UID}\" -g \"${WARD_AGENT_GID}\" -m 0700 /home/ubuntu/.ward" in text
