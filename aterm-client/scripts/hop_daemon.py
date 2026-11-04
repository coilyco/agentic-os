"""A stand-in aterm daemon that fails at one chosen hop per port, on real sockets.

Each scenario listens on its own loopback port and breaks the connection at a
different hop, so a browser produces the real failure and not a scripted one.
Prints {"scenario": port} as one JSON line, then serves until killed.
Reference: tooling-aterm-client, reach.md. Standard library, plus the openssl CLI.
"""
import asyncio
import base64
import hashlib
import json
import socket
import ssl
import struct
import subprocess
import tempfile
import time
from pathlib import Path

# A daemon that is up for a few seconds, then drops every socket and refuses the upgrade.
# One clock per scenario, so each browser size starts against a daemon that is up.
FLAP_UP_SECONDS = 3
flap: dict[str, float] = {}
GUID = b"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
REFUSALS = {"refused-ownership": ("ownership", "that device belongs to another tailnet user"), "refused-name": ("name", "open this daemon by its tailnet name"), "refused-origin": ("origin", "this page may not open a session socket")}
# Scenarios that speak HTTPS, as the daemon's tailnet listener does.
TLS = {"ok", "flaps", "flaps-narrow", "http-500", "upgrade-refused", "opaque-origin", "welcome-error", "cut-after-open", "no-welcome", *REFUSALS}
# Scenarios whose breakage is the certificate, so the browser is not told to trust it.
PLAIN = {"tls-plain", "reset", "silent"}


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


def certificate(directory: Path, name: str) -> tuple[str, str]:
    crt, key = str(directory / f"{name}.crt"), str(directory / f"{name}.key")
    subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", crt, "-days", "1", "-subj", f"/CN={name}", "-addext", "subjectAltName=IP:127.0.0.1"], check=True, capture_output=True)
    return crt, key


async def head(reader: asyncio.StreamReader) -> tuple[str, dict[str, str]]:
    raw = await asyncio.wait_for(reader.readuntil(b"\r\n\r\n"), 5)
    lines = raw.decode("latin1").split("\r\n")
    return lines[0].split(" ")[1], {k.lower(): v for k, v in (line.split(": ", 1) for line in lines[1:] if ": " in line)}


def respond(writer: asyncio.StreamWriter, status: str, headers: dict[str, str], body: str = "") -> None:
    data = body.encode()
    lines = [f"HTTP/1.1 {status}", *(f"{k}: {v}" for k, v in headers.items()), f"Content-Length: {len(data)}", "Connection: close", "", ""]
    writer.write("\r\n".join(lines).encode() + data)


async def read_frame(reader: asyncio.StreamReader) -> tuple[int, bytes]:
    first, second = await reader.readexactly(2)
    size = second & 0x7F
    if size == 126:
        (size,) = struct.unpack(">H", await reader.readexactly(2))
    elif size == 127:
        (size,) = struct.unpack(">Q", await reader.readexactly(8))
    mask = await reader.readexactly(4)
    return first & 0x0F, bytes(byte ^ mask[i % 4] for i, byte in enumerate(await reader.readexactly(size)))


def frame(opcode: int, payload: bytes) -> bytes:
    return bytes([0x80 | opcode, len(payload)]) + payload


async def serve_http(scenario: str, reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
    path, headers = await head(reader)
    origin = headers.get("origin", "")
    cors = {"Access-Control-Allow-Origin": origin, "Access-Control-Expose-Headers": "X-Aterm-Refusal", "Vary": "Origin"} if origin and scenario != "opaque-origin" else {}
    if scenario == "http-500":
        respond(writer, "500 Internal Server Error", {**cors, "Content-Type": "text/plain"}, "not aterm")
    elif headers.get("upgrade", "").lower() == "websocket":
        flaps = scenario.startswith("flaps")
        since = flap.setdefault(scenario, time.monotonic())
        down = flaps and time.monotonic() - since > FLAP_UP_SECONDS
        if scenario == "upgrade-refused" or scenario == "opaque-origin":
            respond(writer, "403 Forbidden", {}, "no")
        elif down:
            respond(writer, "503 Service Unavailable", {}, "restarting")
        else:
            key = headers["sec-websocket-key"].encode()
            accept = base64.b64encode(hashlib.sha1(key + GUID).digest()).decode()
            writer.write(f"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: {accept}\r\n\r\n".encode())
            await writer.drain()
            if scenario == "no-welcome":
                await asyncio.sleep(30)
            await read_frame(reader)
            if scenario == "cut-after-open":
                writer.write(frame(8, struct.pack(">H", 1008) + b"policy"))
                await writer.drain()
                return
            welcome = {"type": "welcome", "format": "aterm.daemon.v1", "version": "mock"}
            if scenario == "welcome-error":
                welcome["error"] = "unsupported format aterm.daemon.v0"
            writer.write(frame(1, json.dumps(welcome).encode()))
            await writer.drain()
            # An up daemon stays up and answers the client's own probe, a list.
            while scenario == "ok" or flaps:
                left = since + FLAP_UP_SECONDS - time.monotonic() if flaps else 120
                opcode, payload = await asyncio.wait_for(read_frame(reader), max(left, 0.05))
                if opcode == 8:
                    return
                asked = json.loads(payload)
                if asked.get("type") == "list":
                    writer.write(frame(1, json.dumps({"type": "sessions", "id": asked.get("id"), "sessions": []}).encode()))
                    await writer.drain()
            await asyncio.sleep(1)
    elif path == "/_aterm/reach" and scenario in REFUSALS:
        layer, reason = REFUSALS[scenario]
        respond(writer, "403 Forbidden", {**cors, "X-Aterm-Refusal": layer, "Content-Type": "text/plain"}, f"aterm daemon: {reason}\n")
    elif path == "/_aterm/reach":
        respond(writer, "200 OK", {**cors, "Cache-Control": "no-store", "Content-Type": "application/json"}, '{"layer":"ok"}')
    else:
        respond(writer, "404 Not Found", cors, "")
    await writer.drain()


async def serve_plain(scenario: str, _reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
    if scenario == "reset":
        writer.transport.get_extra_info("socket").setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, struct.pack("ii", 1, 0))
        writer.transport.abort()
        return
    if scenario == "silent":
        await asyncio.sleep(60)
        return
    # tls-plain: a daemon that speaks HTTP where the browser asked for HTTPS.
    respond(writer, "200 OK", {"Content-Type": "text/plain"}, "plain http")
    await writer.drain()


async def main() -> None:
    directory = Path(tempfile.mkdtemp())
    crt, key = certificate(directory, "mock-daemon")
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(crt, key)
    ports: dict[str, int] = {}
    servers = []

    def handler(scenario: str, serve):
        async def run(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
            try:
                await serve(scenario, reader, writer)
            except (asyncio.TimeoutError, asyncio.IncompleteReadError, ConnectionError, ssl.SSLError):
                pass
            finally:
                writer.close()

        return run

    for scenario in sorted(TLS):
        ports[scenario] = free_port()
        servers.append(await asyncio.start_server(handler(scenario, serve_http), "127.0.0.1", ports[scenario], ssl=context))
    for scenario in sorted(PLAIN):
        ports[scenario] = free_port()
        servers.append(await asyncio.start_server(handler(scenario, serve_plain), "127.0.0.1", ports[scenario]))
    # Hops 1 and 2 need no server: an unresolvable name, and a port nothing listens on.
    ports["refused"] = free_port()
    ports["unresolved"] = free_port()
    # The same certificate, to a browser that trusts nothing: the TLS hop failing.
    ports["tls-selfsigned"] = free_port()
    servers.append(await asyncio.start_server(handler("ok", serve_http), "127.0.0.1", ports["tls-selfsigned"], ssl=context))
    print(json.dumps(ports), flush=True)
    await asyncio.gather(*(server.serve_forever() for server in servers))


if __name__ == "__main__":
    asyncio.run(main())
