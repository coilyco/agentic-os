"""Run the release actions against a fake API as GitHub and as Forgejo.

The fake records every request, so each case asserts the endpoint, the auth
header and the body one host receives. See docs/release.md.
"""
from __future__ import annotations

import base64
import json
import os
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

import pytest

ROOT = Path(__file__).resolve().parent.parent
ACTIONS = ROOT / "actions"
FORMULA = 'class Aos < Formula\n  url "https://old.example/x.git", tag: "v1", revision: "aaa"\n  version "1"\nend\n'


class FakeApi:
    """Answers from `routes[(method, path)]`, a list consumed one reply per call."""

    def __init__(self) -> None:
        self.routes: dict[tuple[str, str], list[tuple[int, object]]] = {}
        self.calls: list[dict] = []
        api = self

        class Handler(BaseHTTPRequestHandler):
            def handle_any(self) -> None:
                length = int(self.headers.get("Content-Length") or 0)
                parts = urlsplit(self.path)
                api.calls.append(
                    {
                        "method": self.command,
                        "path": parts.path,
                        "query": parts.query,
                        "headers": dict(self.headers),
                        "body": self.rfile.read(length),
                    }
                )
                replies = api.routes.get((self.command, parts.path), [])
                status, payload = replies.pop(0) if len(replies) > 1 else (replies[0] if replies else (404, {}))
                data = json.dumps(payload).encode()
                self.send_response(status)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = handle_any

            def log_message(self, *args: object) -> None:
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self.server.server_port}"

    def route(self, method: str, path: str, *replies: tuple[int, object]) -> None:
        self.routes[(method, path)] = list(replies)

    def sent(self, method: str, path: str) -> list[dict]:
        return [c for c in self.calls if c["method"] == method and c["path"] == path]


@pytest.fixture
def api():
    fake = FakeApi()
    yield fake
    fake.server.shutdown()


@pytest.fixture
def repo(tmp_path: Path) -> Path:
    """A clone with an origin, because tag-bump runs `git fetch --tags`."""
    origin = tmp_path / "origin.git"
    work = tmp_path / "work"
    for cmd in (
        ["git", "init", "-q", "--bare", str(origin)],
        ["git", "clone", "-q", str(origin), str(work)],
    ):
        subprocess.run(cmd, check=True, capture_output=True)
    git = ["git", "-C", str(work), "-c", "user.name=t", "-c", "user.email=t@example.com"]
    (work / "f").write_text("x")
    subprocess.run([*git, "add", "f"], check=True)
    subprocess.run([*git, "commit", "-q", "-m", "init", "--no-gpg-sign"], check=True)
    return work


def run_action(name: str, cwd: Path, **env: str) -> tuple[subprocess.CompletedProcess, dict[str, str]]:
    output = cwd / "github_output"
    output.write_text("")
    full = {
        **os.environ,
        "GITHUB_ACTION_PATH": str(ACTIONS / name),
        "GITHUB_OUTPUT": str(output),
        "REPO": "owner/name",
        **env,
    }
    done = subprocess.run(
        ["bash", str(ACTIONS / name / "run.sh")], cwd=cwd, env=full, capture_output=True, text=True, check=False
    )
    outputs = {}
    for line in output.read_text().splitlines():
        if "=" in line and not line.startswith("changelog"):
            key, _, value = line.partition("=")
            outputs[key] = value
    return done, outputs


def tag_env(api: FakeApi, host: str, **extra: str) -> dict[str, str]:
    return {
        "RELEASE_HOST_INPUT": host,
        "RELEASE_API_URL": api.url + ("/api/v1" if host == "forgejo" else ""),
        "FORGEJO_TOKEN": "job-token",
        "BUMP": "minor",
        "CREATE_TAG": "true",
        "TAG_NAME": "v1.2.0",
        "TAG_PREFIX": "v",
        "RELEASE_SHA": "abc123",
        **extra,
    }


def test_tag_bump_github_creates_a_git_ref(api: FakeApi, repo: Path) -> None:
    api.route("POST", "/repos/owner/name/git/refs", (201, {}))
    done, out = run_action("tag-bump", repo, **tag_env(api, "github", RELEASE_TOKEN_INPUT="app-token"))
    assert done.returncode == 0, done.stderr + done.stdout
    (call,) = api.sent("POST", "/repos/owner/name/git/refs")
    assert json.loads(call["body"]) == {"ref": "refs/tags/v1.2.0", "sha": "abc123"}
    assert call["headers"]["Authorization"] == "Bearer app-token"
    assert out["new_tag"] == "v1.2.0"


def test_tag_bump_github_treats_an_existing_ref_as_done_but_not_other_422s(api: FakeApi, repo: Path) -> None:
    api.route("POST", "/repos/owner/name/git/refs", (422, {"message": "Reference already exists"}))
    done, _ = run_action("tag-bump", repo, **tag_env(api, "github"))
    assert done.returncode == 0
    assert "tag already exists" in done.stdout
    api.route("POST", "/repos/owner/name/git/refs", (422, {"message": "Object does not exist"}))
    done, _ = run_action("tag-bump", repo, **tag_env(api, "github"))
    assert done.returncode == 1
    assert "HTTP 422" in done.stdout


def test_tag_bump_forgejo_keeps_the_tags_endpoint_and_token_header(api: FakeApi, repo: Path) -> None:
    api.route("POST", "/api/v1/repos/owner/name/tags", (409, {}))
    done, out = run_action("tag-bump", repo, **tag_env(api, "forgejo"))
    assert done.returncode == 0, done.stderr + done.stdout
    (call,) = api.sent("POST", "/api/v1/repos/owner/name/tags")
    assert json.loads(call["body"]) == {"tag_name": "v1.2.0", "target": "abc123"}
    assert call["headers"]["Authorization"] == "token job-token"
    assert out["new_tag"] == "v1.2.0"


def test_tag_bump_compute_only_needs_no_api(api: FakeApi, repo: Path) -> None:
    done, out = run_action("tag-bump", repo, **tag_env(api, "github", CREATE_TAG="false", TAG_NAME=""))
    assert done.returncode == 0, done.stderr
    assert out["new_tag"] == "v0.1.0"
    assert api.calls == []


def test_tag_bump_changelog_keeps_the_newest_100_commits(api: FakeApi, repo: Path) -> None:
    # One fast-import stream, since 129 separate commits take a minute here.
    def git_out(*args: str) -> str:
        return subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True, text=True).stdout.strip()

    branch, tip = git_out("symbolic-ref", "--short", "HEAD"), git_out("rev-parse", "HEAD")
    stream = "".join(
        f"commit refs/heads/{branch}\ncommitter t <t@example.com> {1_700_000_000 + n} +0000\n"
        f"data {len(f'c{n}')}\nc{n}\n" + ("" if n else f"from {tip}\n") + "\n"
        for n in range(129)
    )
    subprocess.run(["git", "-C", str(repo), "fast-import", "--quiet"], input=stream, text=True, check=True)
    git_out("reset", "-q", "--hard", branch)
    done, _ = run_action("tag-bump", repo, **tag_env(api, "github", CREATE_TAG="false", TAG_NAME=""))
    assert done.returncode == 0, done.stderr
    lines = (repo / "github_output").read_text().split("changelog<<EOF\n")[1].split("\nEOF")[0].splitlines()
    assert lines[0].startswith("- c128 (")
    assert lines[99].startswith("- c29 (")
    assert lines[100] == "- ... and 30 older commits"
    assert len(lines) == 101


def test_host_auto_reads_the_runner_server_url(api: FakeApi, repo: Path) -> None:
    api.route("POST", "/repos/owner/name/git/refs", (201, {}))
    env = tag_env(api, "auto", GITHUB_SERVER_URL="https://github.com")
    done, _ = run_action("tag-bump", repo, **env)
    assert done.returncode == 0, done.stderr + done.stdout
    assert api.sent("POST", "/repos/owner/name/git/refs")
    api.route("POST", "/api/v1/repos/owner/name/tags", (201, {}))
    env = tag_env(api, "auto", GITHUB_SERVER_URL="https://forgejo.example", RELEASE_API_URL="", FORGEJO_BASE_URL=api.url)
    done, _ = run_action("tag-bump", repo, **env)
    assert done.returncode == 0, done.stderr + done.stdout
    assert api.sent("POST", "/api/v1/repos/owner/name/tags")


def test_missing_token_fails_before_any_call(api: FakeApi, repo: Path) -> None:
    done, _ = run_action("tag-bump", repo, **tag_env(api, "github", FORGEJO_TOKEN=""))
    assert done.returncode == 1
    assert "no API token" in done.stderr
    assert api.calls == []


def release_env(api: FakeApi, host: str, **extra: str) -> dict[str, str]:
    return {**tag_env(api, host), "TAG": "v1.2.0", "NAME": "", "BODY": "notes", **extra}


def test_create_release_github_and_idempotent_reuse(api: FakeApi, tmp_path: Path) -> None:
    api.route("POST", "/repos/owner/name/releases", (201, {"id": 41}))
    done, out = run_action("create-release", tmp_path, **release_env(api, "github"))
    assert done.returncode == 0, done.stderr + done.stdout
    assert out["release_id"] == "41"
    (call,) = api.sent("POST", "/repos/owner/name/releases")
    assert json.loads(call["body"])["tag_name"] == "v1.2.0"
    assert call["headers"]["Accept"] == "application/vnd.github+json"

    exists = {"message": "Validation Failed", "errors": [{"code": "already_exists", "field": "tag_name"}]}
    api.route("POST", "/repos/owner/name/releases", (422, exists))
    api.route("GET", "/repos/owner/name/releases/tags/v1.2.0", (200, {"id": 41}))
    done, out = run_action("create-release", tmp_path, **release_env(api, "github"))
    assert done.returncode == 0, done.stderr + done.stdout
    assert out["release_id"] == "41"


def test_create_release_github_sends_make_latest_on_create(api: FakeApi, tmp_path: Path) -> None:
    api.route("POST", "/repos/owner/name/releases", (201, {"id": 41}))
    done, _ = run_action("create-release", tmp_path, **release_env(api, "github", MAKE_LATEST="false"))
    assert done.returncode == 0, done.stderr + done.stdout
    (call,) = api.sent("POST", "/repos/owner/name/releases")
    assert json.loads(call["body"])["make_latest"] == "false"
    assert api.sent("PATCH", "/repos/owner/name/releases/41") == []


def test_create_release_github_reapplies_make_latest_to_an_existing_release(api: FakeApi, tmp_path: Path) -> None:
    exists = {"message": "Validation Failed", "errors": [{"code": "already_exists", "field": "tag_name"}]}
    api.route("POST", "/repos/owner/name/releases", (422, exists))
    api.route("GET", "/repos/owner/name/releases/tags/v1.2.0", (200, {"id": 41}))
    api.route("PATCH", "/repos/owner/name/releases/41", (200, {"id": 41}))
    done, out = run_action("create-release", tmp_path, **release_env(api, "github", MAKE_LATEST="true"))
    assert done.returncode == 0, done.stderr + done.stdout
    assert out["release_id"] == "41"
    (call,) = api.sent("PATCH", "/repos/owner/name/releases/41")
    assert json.loads(call["body"]) == {"make_latest": "true"}


def test_create_release_github_fails_when_the_make_latest_update_is_refused(api: FakeApi, tmp_path: Path) -> None:
    exists = {"message": "Validation Failed", "errors": [{"code": "already_exists", "field": "tag_name"}]}
    api.route("POST", "/repos/owner/name/releases", (422, exists))
    api.route("GET", "/repos/owner/name/releases/tags/v1.2.0", (200, {"id": 41}))
    api.route("PATCH", "/repos/owner/name/releases/41", (403, {"message": "Forbidden"}))
    done, _ = run_action("create-release", tmp_path, **release_env(api, "github", MAKE_LATEST="true"))
    assert done.returncode == 1
    assert "HTTP 403" in done.stdout


def test_create_release_rejects_a_make_latest_value_before_any_call(api: FakeApi, tmp_path: Path) -> None:
    done, _ = run_action("create-release", tmp_path, **release_env(api, "github", MAKE_LATEST="yes"))
    assert done.returncode == 1
    assert "make_latest must be true|false" in done.stderr
    assert api.calls == []


def test_create_release_forgejo_does_not_send_make_latest(api: FakeApi, tmp_path: Path) -> None:
    api.route("POST", "/api/v1/repos/owner/name/releases", (201, {"id": 7}))
    done, _ = run_action("create-release", tmp_path, **release_env(api, "forgejo", MAKE_LATEST="true"))
    assert done.returncode == 0, done.stderr + done.stdout
    (call,) = api.sent("POST", "/api/v1/repos/owner/name/releases")
    assert "make_latest" not in json.loads(call["body"])


def test_create_release_github_fails_on_other_422(api: FakeApi, tmp_path: Path) -> None:
    api.route("POST", "/repos/owner/name/releases", (422, {"message": "Validation Failed", "errors": []}))
    done, _ = run_action("create-release", tmp_path, **release_env(api, "github"))
    assert done.returncode == 1
    assert "HTTP 422" in done.stdout


def test_create_release_forgejo_keeps_409_reuse(api: FakeApi, tmp_path: Path) -> None:
    api.route("POST", "/api/v1/repos/owner/name/releases", (409, {}))
    api.route("GET", "/api/v1/repos/owner/name/releases/tags/v1.2.0", (200, {"id": 7}))
    done, out = run_action("create-release", tmp_path, **release_env(api, "forgejo"))
    assert done.returncode == 0, done.stderr + done.stdout
    assert out["release_id"] == "7"


def upload_env(api: FakeApi, host: str, asset: Path, **extra: str) -> dict[str, str]:
    return {**tag_env(api, host), "RELEASE_ID": "41", "FILE": str(asset), "NAME": "", **extra}


def test_upload_github_replaces_an_asset_on_a_later_page_and_posts_the_raw_body(api: FakeApi, tmp_path: Path) -> None:
    asset = tmp_path / "aos.tar.gz"
    asset.write_bytes(b"payload")
    page = [{"id": n, "name": f"other-{n}"} for n in range(100)]
    api.route("GET", "/repos/owner/name/releases/41/assets", (200, page), (200, [{"id": 900, "name": "aos.tar.gz"}]))
    api.route("DELETE", "/repos/owner/name/releases/assets/900", (204, {}))
    api.route("POST", "/repos/owner/name/releases/41/assets", (201, {"browser_download_url": "https://x/aos.tar.gz"}))
    done, out = run_action("upload-release-asset", tmp_path, **upload_env(api, "github", asset))
    assert done.returncode == 0, done.stderr + done.stdout
    assert [c["query"] for c in api.sent("GET", "/repos/owner/name/releases/41/assets")] == [
        "per_page=100&page=1",
        "per_page=100&page=2",
    ]
    assert api.sent("DELETE", "/repos/owner/name/releases/assets/900")
    (up,) = api.sent("POST", "/repos/owner/name/releases/41/assets")
    assert up["query"] == "name=aos.tar.gz"
    assert up["headers"]["Content-Type"] == "application/octet-stream"
    assert up["body"] == b"payload"
    assert out["asset_url"] == "https://x/aos.tar.gz"


def test_upload_forgejo_keeps_multipart_and_release_scoped_delete(api: FakeApi, tmp_path: Path) -> None:
    asset = tmp_path / "aos.tar.gz"
    asset.write_bytes(b"payload")
    base = "/api/v1/repos/owner/name/releases/41/assets"
    api.route("GET", base, (200, [{"id": 5, "name": "aos.tar.gz"}]))
    api.route("DELETE", f"{base}/5", (204, {}))
    api.route("POST", base, (201, {"browser_download_url": "https://f/aos.tar.gz"}))
    done, _ = run_action("upload-release-asset", tmp_path, **upload_env(api, "forgejo", asset))
    assert done.returncode == 0, done.stderr + done.stdout
    (up,) = api.sent("POST", base)
    assert up["headers"]["Content-Type"].startswith("multipart/form-data")
    assert api.sent("GET", base)[0]["query"] == ""


def bump_env(api: FakeApi, host: str, **extra: str) -> dict[str, str]:
    return {
        **tag_env(api, host),
        "FORMULA_PATH": "Formula/aos.rb",
        "TAG": "v2.0.0",
        "NEW_REVISION": "deadbeef",
        "URL_TEMPLATE": "",
        "BRANCH": "main",
        "MSG_TEMPLATE": "chore(formula): bump to {TAG} [skip ci]",
        "FORGEJO_BASE_URL": "https://forgejo.example",
        **extra,
    }


def put_back(call: dict) -> tuple[dict, str]:
    body = json.loads(call["body"])
    return body, base64.b64decode(body["content"]).decode()


@pytest.mark.parametrize(
    ("host", "prefix", "clone"),
    [("github", "", "https://github.com/owner/name.git"), ("forgejo", "/api/v1", "https://forgejo.example/owner/name.git")],
)
def test_bump_formula_writes_the_clone_url_of_the_host(api: FakeApi, tmp_path: Path, host: str, prefix: str, clone: str) -> None:
    path = f"{prefix}/repos/owner/name/contents/Formula/aos.rb"
    api.route("GET", path, (200, {"sha": "blobsha", "content": base64.encodebytes(FORMULA.encode()).decode()}))
    api.route("PUT", path, (200, {}))
    done, _ = run_action("bump-formula", tmp_path, **bump_env(api, host))
    assert done.returncode == 0, done.stderr + done.stdout
    body, text = put_back(api.sent("PUT", path)[0])
    assert f'  url "{clone}", tag: "v2.0.0", revision: "deadbeef"' in text
    assert 'version "1"' in text
    assert body["sha"] == "blobsha"
    assert body["message"] == "chore(formula): bump to v2.0.0 [skip ci]"
    assert body["branch"] == "main"


def test_bump_formula_explicit_template_wins(api: FakeApi, tmp_path: Path) -> None:
    path = "/repos/owner/name/contents/Formula/aos.rb"
    api.route("GET", path, (200, {"sha": "s", "content": base64.b64encode(FORMULA.encode()).decode()}))
    api.route("PUT", path, (201, {}))
    done, _ = run_action("bump-formula", tmp_path, **bump_env(api, "github", URL_TEMPLATE="git@host:{REPO}.git"))
    assert done.returncode == 0, done.stderr + done.stdout
    assert 'url "git@host:owner/name.git"' in put_back(api.sent("PUT", path)[0])[1]
