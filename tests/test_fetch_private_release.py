"""Behavior of docker/dev-base/fetch-private-release.sh against fake GitHub servers.

The script fetches named assets of a private release with a read token and checks them
against the release's SHA256SUMS. These tests run it for real: an API server that
redirects each asset to a second server, which stands in for GitHub's storage host.
"""

import hashlib
import json
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parents[1] / "docker" / "dev-base" / "fetch-private-release.sh"
ASSET = b"agent-compose bytes"


class Servers:
    """An API server and a storage server, each recording the Authorization it saw."""

    def __init__(self, sums_for: bytes):
        self.api_auth: list[str | None] = []
        self.store_auth: list[str | None] = []
        blobs = {"/blob/a": ASSET, "/blob/sums": b"%s  agent-compose-linux-amd64\n" % hashlib.sha256(sums_for).hexdigest().encode()}
        outer = self

        class Store(BaseHTTPRequestHandler):
            def do_GET(self):
                outer.store_auth.append(self.headers.get("Authorization"))
                body = blobs.get(self.path)
                self.send_response(200 if body is not None else 404)
                self.end_headers()
                self.wfile.write(body or b"")

            def log_message(self, *args):
                pass

        self.store = HTTPServer(("127.0.0.1", 0), Store)
        store_port = self.store.server_port

        class Api(BaseHTTPRequestHandler):
            def do_GET(self):
                outer.api_auth.append(self.headers.get("Authorization"))
                if self.path == "/repos/o/r/releases/tags/v1":
                    body = json.dumps({"assets": [{"name": "agent-compose-linux-amd64", "id": 1}, {"name": "SHA256SUMS", "id": 2}]}).encode()
                    self.send_response(200)
                    self.end_headers()
                    self.wfile.write(body)
                elif self.path in ("/repos/o/r/releases/assets/1", "/repos/o/r/releases/assets/2"):
                    self.send_response(302)
                    self.send_header("Location", f"http://127.0.0.1:{store_port}/blob/{'a' if self.path.endswith('1') else 'sums'}")
                    self.end_headers()
                else:
                    self.send_response(404)
                    self.end_headers()

            def log_message(self, *args):
                pass

        self.api = HTTPServer(("127.0.0.1", 0), Api)
        for server in (self.api, self.store):
            threading.Thread(target=server.serve_forever, daemon=True).start()

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self.api.server_port}"

    def close(self):
        for server in (self.api, self.store):
            server.shutdown()
            server.server_close()


def run(tmp_path: Path, servers: Servers, token: str):
    token_file = tmp_path / "token"
    token_file.write_text(token)
    return subprocess.run(
        ["bash", str(SCRIPT), str(token_file), "o/r", "v1", str(tmp_path / "out"), "agent-compose-linux-amd64"],
        env={"PATH": "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin", "GITHUB_API_URL": servers.url},
        capture_output=True,
        text=True,
    )


@pytest.fixture
def good(tmp_path):
    servers = Servers(sums_for=ASSET)
    yield servers
    servers.close()


def test_the_token_reaches_the_api_and_never_the_storage_host(tmp_path, good):
    result = run(tmp_path, good, "tok-123\n")
    assert result.returncode == 0, result.stderr
    assert (tmp_path / "out" / "agent-compose-linux-amd64").read_bytes() == ASSET
    assert "Bearer tok-123" in good.api_auth
    assert good.store_auth and all(value is None for value in good.store_auth)
    assert "tok-123" not in result.stdout + result.stderr


def test_a_checksum_mismatch_fails_the_fetch(tmp_path):
    servers = Servers(sums_for=b"some other bytes")
    try:
        result = run(tmp_path, servers, "tok-123")
    finally:
        servers.close()
    assert result.returncode != 0
    assert "is not" in result.stderr


def test_an_empty_token_file_fails_before_any_request(tmp_path, good):
    result = run(tmp_path, good, "")
    assert result.returncode == 1
    assert "pass the read token as a BuildKit secret" in result.stderr
    assert good.api_auth == []
