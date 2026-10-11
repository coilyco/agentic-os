"""Tests for the rm, find and mv fs guard stubs.

The guard refuses a destructive call whose path sits outside the session shadow
and the scratchpad. The shadow root comes from AOS_NATIVE_SESSION_PROJECTS, so
each case builds its own. See COI-2724.
"""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
INSTALL = ROOT / "scripts" / "fs-shims" / "install.sh"


@pytest.fixture()
def world(tmp_path: Path):
    """A shadow with a projects dir, a decoy outside it, and the installed stubs."""
    shadow = tmp_path / "private" / "tmp" / "u1" / "sess"
    (shadow / "projects").mkdir(parents=True)
    outside = tmp_path / "private" / "tmp" / "elsewhere"
    outside.mkdir(parents=True)
    shims = tmp_path / "shims"
    subprocess.run(["sh", str(INSTALL), str(shims)], check=True, capture_output=True)
    env = {
        **os.environ,
        "AOS_NATIVE_SESSION_PROJECTS": str(shadow / "projects"),
        "PATH": f"{shims}{os.pathsep}{os.environ['PATH']}",
    }

    def run(*argv: str, cwd: Path = shadow):
        return subprocess.run(
            [str(shims / argv[0]), *argv[1:]], cwd=cwd, env=env,
            capture_output=True, text=True,
        )

    return shadow, outside, run


def test_rm_inside_the_shadow_runs(world) -> None:
    shadow, _, run = world
    victim = shadow / "projects" / "tree"
    (victim / "sub").mkdir(parents=True)
    assert run("rm", "-rf", str(victim)).returncode == 0
    assert not victim.exists()


def test_rm_outside_the_shadow_is_refused_and_leaves_the_decoy(world) -> None:
    shadow, outside, run = world
    decoy = outside / "decoy"
    decoy.mkdir()
    result = run("rm", "-rf", str(decoy))
    assert result.returncode == 2
    assert "outside the session shadow" in result.stderr
    assert decoy.exists()


def test_rm_relative_escape_is_refused(world) -> None:
    shadow, outside, run = world
    decoy = outside / "decoy"
    decoy.mkdir()
    rel = os.path.relpath(decoy, shadow / "projects")
    assert run("rm", "-rf", rel, cwd=shadow / "projects").returncode == 2
    assert decoy.exists()


def test_rm_through_a_symlink_parent_is_refused(world) -> None:
    shadow, outside, run = world
    (outside / "real").mkdir()
    (shadow / "projects" / "link").symlink_to(outside)
    assert run("rm", "-rf", str(shadow / "projects" / "link" / "real")).returncode == 2
    assert (outside / "real").exists()


def test_rm_of_a_symlink_inside_the_shadow_removes_the_link_only(world) -> None:
    shadow, outside, run = world
    link = shadow / "projects" / "link"
    link.symlink_to(outside)
    assert run("rm", str(link)).returncode == 0
    assert not link.is_symlink() and outside.exists()


def test_double_dash_hides_no_operand(world) -> None:
    _, outside, run = world
    decoy = outside / "-rf"
    decoy.write_text("x")
    assert run("rm", "--", str(decoy)).returncode == 2
    assert decoy.exists()


def test_find_delete_outside_is_refused_and_read_only_find_passes(world) -> None:
    _, outside, run = world
    (outside / "a.tmp").write_text("x")
    assert run("find", str(outside), "-name", "*.tmp", "-delete").returncode == 2
    assert (outside / "a.tmp").exists()
    listing = run("find", str(outside), "-name", "*.tmp")
    assert listing.returncode == 0 and "a.tmp" in listing.stdout


def test_find_exec_outside_is_refused(world) -> None:
    _, outside, run = world
    (outside / "a.tmp").write_text("x")
    assert run("find", str(outside), "-exec", "rm", "{}", ";").returncode == 2
    assert (outside / "a.tmp").exists()


def test_mv_needs_both_ends_inside(world) -> None:
    shadow, outside, run = world
    src = shadow / "projects" / "a"
    src.write_text("x")
    assert run("mv", str(src), str(outside / "a")).returncode == 2
    assert src.exists()
    assert run("mv", str(src), str(shadow / "projects" / "b")).returncode == 0
    (outside / "keep").write_text("x")
    assert run("mv", "-t", str(shadow / "projects"), str(outside / "keep")).returncode == 2


def test_an_unset_shadow_refuses_everything(world, tmp_path: Path) -> None:
    shadow, _, run = world
    victim = shadow / "projects" / "v"
    victim.mkdir()
    shims = tmp_path / "shims"
    env = {k: v for k, v in os.environ.items() if k != "AOS_NATIVE_SESSION_PROJECTS"}
    env["PATH"] = f"{shims}{os.pathsep}{env['PATH']}"
    result = subprocess.run([str(shims / "rm"), "-rf", str(victim)], env=env,
                            capture_output=True, text=True)
    assert result.returncode == 2 and victim.exists()
