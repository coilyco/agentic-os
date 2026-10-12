"""Tests for scripts/apply-shell-links.py."""
from __future__ import annotations

import importlib.util
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "apply-shell-links.py"


def _load_script():
    spec = importlib.util.spec_from_file_location("apply_shell_links", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _make_repo(root: Path) -> None:
    (root / "shell").mkdir(parents=True)
    (root / "scripts").mkdir()
    (root / "kitty").mkdir()
    (root / "kitty" / "kitty.conf").write_text("font_size 14\n", encoding="utf-8")
    (root / "shell" / "zshrc").write_text("zsh\n", encoding="utf-8")
    (root / "shell" / "bashrc").write_text("bash\n", encoding="utf-8")
    (root / "scripts" / "gpg-ssm").write_text("gpg\n", encoding="utf-8")
    (root / "scripts" / "gpg-ssm.cmd").write_text("gpg\n", encoding="utf-8")
    (root / "scripts" / "ssm-get").write_text("ssm\n", encoding="utf-8")
    (root / "scripts" / "git-credential-forgejo-ssm.sh").write_text("git\n", encoding="utf-8")
    (root / "scripts" / "git-credential-forgejo-ssm.cmd").write_text("git\n", encoding="utf-8")
    (root / "scripts" / "docker-credential-forgejo-ssm").write_text(
        "docker\n", encoding="utf-8"
    )
    (root / "scripts" / "docker-credential-forgejo-ssm.cmd").write_text(
        "docker\n", encoding="utf-8"
    )


def test_repoints_stale_symlink(tmp_path: Path) -> None:
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    home.mkdir()
    _make_repo(repo)

    stale = repo / "zsh" / "zshrc"
    (repo / "zsh").mkdir()
    stale.write_text("old\n", encoding="utf-8")
    (home / ".zshrc").symlink_to(stale)

    spec = script.LinkSpec("zshrc", repo / "shell" / "zshrc", home / ".zshrc")
    action, _ = script.apply_link(spec, dry_run=False)

    assert action == "repointed"
    assert (home / ".zshrc").resolve() == (repo / "shell" / "zshrc").resolve()


def test_backs_up_regular_file_before_linking(tmp_path: Path) -> None:
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    home.mkdir()
    _make_repo(repo)
    (home / ".bashrc").write_text("custom\n", encoding="utf-8")

    spec = script.LinkSpec("bashrc", repo / "shell" / "bashrc", home / ".bashrc")
    action, _ = script.apply_link(spec, dry_run=False)

    assert action == "backed-up"
    assert (home / ".bashrc").is_symlink()
    assert (home / ".bashrc").resolve() == (repo / "shell" / "bashrc").resolve()
    assert (home / ".bashrc.bak").read_text(encoding="utf-8") == "custom\n"


def test_current_link_is_ok_on_second_apply(tmp_path: Path) -> None:
    # Pins the Windows readlink() \\?\-prefix case: a current link must
    # compare equal to its source, not get re-pointed on every run.
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    home.mkdir()
    _make_repo(repo)

    spec = script.LinkSpec("zshrc", repo / "shell" / "zshrc", home / ".zshrc")
    assert script.apply_link(spec, dry_run=False)[0] == "linked"
    assert script.apply_link(spec, dry_run=False)[0] == "ok"


def test_check_reports_drift(tmp_path: Path) -> None:
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    home.mkdir()
    _make_repo(repo)
    specs = script.link_specs(home, repo)

    action, _ = script.apply_link(specs[0], dry_run=True)

    assert action == "would-link"


def test_kitty_config_links_from_a_stock_install(monkeypatch, tmp_path: Path) -> None:
    # The stock kitty.conf a fresh kitty writes is a regular file. It is backed
    # up and replaced, and the tracked file is what kitty then reads.
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    (home / ".config" / "kitty").mkdir(parents=True)
    _make_repo(repo)
    stock = home / ".config" / "kitty" / "kitty.conf"
    stock.write_text("# stock\n", encoding="utf-8")
    monkeypatch.setattr(script.os, "name", "posix")

    spec = next(s for s in script.link_specs(home, repo) if s.name == "kitty")
    action, _ = script.apply_link(spec, dry_run=False)

    assert action == "backed-up"
    assert stock.resolve() == (repo / "kitty" / "kitty.conf").resolve()
    assert (stock.parent / "kitty.conf.bak").read_text(encoding="utf-8") == "# stock\n"


def test_windows_skips_kitty(monkeypatch, tmp_path: Path) -> None:
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    home.mkdir()
    _make_repo(repo)
    monkeypatch.setattr(script.os, "name", "nt")

    assert "kitty" not in [spec.name for spec in script.link_specs(home, repo)]


def test_windows_skips_bashrc(monkeypatch, tmp_path: Path) -> None:
    script = _load_script()
    home = tmp_path / "home"
    repo = tmp_path / "repo"
    home.mkdir()
    _make_repo(repo)
    monkeypatch.setattr(script.os, "name", "nt")

    names = [spec.name for spec in script.link_specs(home, repo)]

    assert names == [
        "zshrc",
        "gpg-ssm",
        "ssm-get",
        "git-credential-forgejo-ssm",
        "docker-credential-forgejo-ssm",
        "gpg-ssm-bash",
        "git-credential-forgejo-ssm-bash",
        "docker-credential-forgejo-ssm-bash",
    ]


def _session_home(tmp_path: Path) -> tuple[Path, Path]:
    """A native session home mirroring a real one, as aos-cli stages it."""
    real = tmp_path / "Users" / "kai"
    (real / ".local" / "bin").mkdir(parents=True)
    (real / ".gitconfig").write_text("", encoding="utf-8")
    session = tmp_path / "aos" / "native" / "pp94" / "home"
    session.mkdir(parents=True)
    for name in (".gitconfig", ".local"):
        (session / name).symlink_to(real / name)
    return real, session


def test_native_session_root_detects_session_paths(tmp_path: Path) -> None:
    script = _load_script()
    session = tmp_path / "aos" / "native" / "20260806t051414z" / "home" / ".local"

    assert script.native_session_root(session) == tmp_path / "aos" / "native"
    assert script.native_session_root(tmp_path / "Users" / "kai") is None


def test_canonical_home_resolves_a_session_home(tmp_path: Path) -> None:
    script = _load_script()
    real, session = _session_home(tmp_path)

    assert script.canonical_home(session) == real
    assert script.canonical_home(real) == real


def test_config_spec_names_the_durable_home(tmp_path: Path) -> None:
    script = _load_script()
    repo = tmp_path / "repo"
    _make_repo(repo)
    real, session = _session_home(tmp_path)

    (spec,) = script.config_specs(session, repo)

    assert spec.key == "gpg.program"
    assert spec.value == str(real / ".local" / "bin" / "gpg-ssm")
    assert spec.config_path == session / ".gitconfig"


def test_config_repoints_a_purged_session_signer(tmp_path: Path) -> None:
    script = _load_script()
    repo = tmp_path / "repo"
    _make_repo(repo)
    real, session = _session_home(tmp_path)
    signer = real / ".local" / "bin" / "gpg-ssm"
    signer.symlink_to(repo / "scripts" / "gpg-ssm")
    purged = tmp_path / "aos" / "native" / "20260806t051414z" / "home" / ".local" / "bin" / "gpg-ssm"
    (session / ".gitconfig").write_text(
        f"[gpg]\n\tprogram = {purged}\n", encoding="utf-8"
    )
    (spec,) = script.config_specs(session, repo)

    assert script.apply_config(spec, dry_run=True) == (
        "would-repoint",
        f"purged session path {purged} -> {signer}",
    )
    action, _ = script.apply_config(spec, dry_run=False)

    assert action == "repointed"
    assert script._git_config_get(spec) == str(signer)
    assert script.apply_config(spec, dry_run=False)[0] == "ok"


def test_config_fails_when_the_signer_is_absent(tmp_path: Path) -> None:
    script = _load_script()
    repo = tmp_path / "repo"
    _make_repo(repo)
    _, session = _session_home(tmp_path)

    (spec,) = script.config_specs(session, repo)
    action, detail = script.apply_config(spec, dry_run=True)

    assert action == "failed"
    assert "link it first" in detail


def test_main_refuses_a_session_checkout(tmp_path: Path, capsys) -> None:
    script = _load_script()
    repo = tmp_path / "aos" / "native" / "pp94" / "projects" / "repo"
    _make_repo(repo)
    _, session = _session_home(tmp_path)

    code = script.main(["--check", "--home", str(session), "--repo-root", str(repo)])

    assert code == 1
    assert "Refusing to apply" in capsys.readouterr().err
