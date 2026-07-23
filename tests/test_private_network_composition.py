"""Tests for the AOS-owned private-network boundary."""
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent


def test_ward_specs_do_not_select_a_private_network_provider() -> None:
    ward_specs = REPO_ROOT / ".ward"
    assert not (ward_specs / "guardfile.tailscale.kdl").exists()
    roles = (ward_specs / "roles.kdl").read_text(encoding="utf-8").lower()
    for forbidden in ("tailscale", "proxy", "socks", "network"):
        assert forbidden not in roles
