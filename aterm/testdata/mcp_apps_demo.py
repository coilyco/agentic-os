"""A demo MCP server with one view, and a driver that fronts it through a daemon.

serve              stdio MCP server: disk_usage returns a ui:// chart, refresh app-only.
drive SOCKET PORT  spawn an opted-in session on the daemon at SOCKET, front this server
                   through its gateway, and call disk_usage on 127.0.0.1:PORT.

Run with `python3 -I`. See the tooling-aterm-client skill, mcp-apps-gateway.md.
"""

import base64
import http.client
import json
import socket
import sys

MIME = "text/html;profile=mcp-app"
URI = "ui://demo/disk"

PAGE = """<!doctype html><html><head><meta charset="utf-8"><style>
body{font:14px system-ui;margin:12px;color:#222;background:#fff}
h1{font-size:15px;margin:0 0 8px}.row{display:flex;align-items:center;gap:8px;margin:6px 0}
.row span{width:70px}.bar{height:14px;background:#4a8;border-radius:3px}
button{margin-top:8px;font:inherit;padding:4px 10px}#note{color:#666;margin-top:6px}
</style></head><body><h1>Disk usage (demo view)</h1><div id="bars">waiting for the tool result</div>
<button id="refresh">Refresh through the host</button><div id="note"></div><script>
let next = 0; const pending = {};
const send = (m) => parent.postMessage(Object.assign({jsonrpc: "2.0"}, m), "*");
const rpc = (method, params) => new Promise((resolve) => { const id = ++next; pending[id] = resolve; send({id, method, params}); });
function draw(data) {
  document.getElementById("bars").innerHTML = data.volumes.map((v) =>
    `<div class="row"><span>${v.name}</span><div class="bar" style="width:${v.percent * 2}px"></div>${v.percent}%</div>`).join("");
  document.getElementById("note").textContent = "reading " + data.reading;
}
addEventListener("message", (event) => {
  const m = event.data || {};
  if (m.id !== undefined && m.method === undefined) { (pending[m.id] || (() => {}))(m); return; }
  if (m.method === "ui/notifications/tool-result") draw(m.params.structuredContent);
});
document.getElementById("refresh").onclick = () =>
  rpc("tools/call", {name: "refresh", arguments: {}}).then((r) => draw(r.result.structuredContent));
rpc("ui/initialize", {protocolVersion: "2026-01-26", appInfo: {name: "demo", version: "1"}, appCapabilities: {}})
  .then(() => send({method: "ui/notifications/initialized", params: {}}));
</script></body></html>"""


def reading(count):
    return {"reading": count, "volumes": [
        {"name": "system", "percent": 40 + count},
        {"name": "data", "percent": 62 + 3 * count},
        {"name": "backups", "percent": 30 + 7 * count},
    ]}


def reply(method, params, state):
    if method == "initialize":
        return {"protocolVersion": params.get("protocolVersion", "2025-11-25"), "capabilities": {"tools": {}, "resources": {}},
                "serverInfo": {"name": "demo", "version": "1"}}
    if method == "tools/list":
        return {"tools": [
            {"name": "disk_usage", "description": "Disk usage as a bar chart view.", "inputSchema": {"type": "object"},
             "_meta": {"ui": {"resourceUri": URI}}},
            {"name": "refresh", "description": "Read the volumes again.", "inputSchema": {"type": "object"},
             "_meta": {"ui": {"visibility": ["app"]}}},
        ]}
    if method == "tools/call":
        state["count"] += 1
        data = reading(state["count"])
        return {"content": [{"type": "text", "text": json.dumps(data["volumes"])}], "structuredContent": data}
    if method == "resources/read":
        return {"contents": [{"uri": URI, "mimeType": MIME, "text": PAGE,
                              "_meta": {"ui": {"prefersBorder": True}}}]}
    return None


def serve():
    state = {"count": 0}
    for line in sys.stdin:
        request = json.loads(line)
        if "id" not in request:
            continue
        result = reply(request["method"], request.get("params") or {}, state)
        body = ({"result": result} if result is not None
                else {"error": {"code": -32601, "message": "no method " + request["method"]}})
        print(json.dumps({"jsonrpc": "2.0", "id": request["id"], **body}), flush=True)


def frames(sock):
    for line in sock.makefile("r"):
        yield json.loads(line)


def drive(socket_path, port):
    sock = socket.socket(socket.AF_UNIX)
    sock.connect(socket_path)
    out = sock.makefile("w")
    incoming = frames(sock)

    def send(message):
        out.write(json.dumps(message) + "\n")
        out.flush()

    send({"type": "hello", "format": "aterm.daemon.v1", "version": "demo"})
    next(incoming)
    send({"type": "spawn", "id": "1", "session": "demo-gateway", "role": "demo", "identity": "gateway", "seat": "shell",
          "argv": ["/bin/sh", "-c", "echo TOKEN=$ATERM_SESSION_TOKEN; exec cat"],
          "env": ["TERM=xterm-256color", "PATH=/usr/bin:/bin", "ATERM_MCP_APPS=1"], "cwd": "/tmp", "rows": 24, "cols": 100})
    seen, token = "", None
    for message in incoming:
        if message["type"] == "output":
            seen += base64.b64decode(message["data"]).decode(errors="replace")
            if "TOKEN=" in seen and "\n" in seen.split("TOKEN=", 1)[1]:
                token = seen.split("TOKEN=", 1)[1].split("\n", 1)[0].strip()
                break
    send({"type": "gateway_add", "id": "2", "token": token, "server": "demo",
          "gateway": {"command": sys.executable, "args": ["-I", __file__, "serve"]}})
    added = next(m for m in incoming if m["type"] in ("gateway_added", "error"))
    print(json.dumps(added))
    path = added["gateway"]["path"]

    session = {}

    def post(method, params=None, notify=False):
        headers = {"Content-Type": "application/json", "Accept": "application/json, text/event-stream", **session}
        message = {"jsonrpc": "2.0", "method": method, "params": params or {}}
        if not notify:
            message["id"] = 1
        conn = http.client.HTTPConnection("127.0.0.1", int(port))
        conn.request("POST", path, json.dumps(message), headers)
        response = conn.getresponse()
        body = response.read().decode()
        if response.getheader("Mcp-Session-Id"):
            session["Mcp-Session-Id"] = response.getheader("Mcp-Session-Id")
        if notify:
            return None
        if "text/event-stream" in (response.getheader("Content-Type") or ""):
            body = next(line[5:] for line in body.splitlines() if line.startswith("data:"))
        return json.loads(body)

    post("initialize", {"protocolVersion": "2025-11-25", "capabilities": {},
                        "clientInfo": {"name": "demo-harness", "version": "1"}})
    session["MCP-Protocol-Version"] = "2025-11-25"
    post("notifications/initialized", notify=True)
    print("tools the agent sees:", [t["name"] for t in post("tools/list")["result"]["tools"]])
    print("tools/call disk_usage:", json.dumps(post("tools/call", {"name": "disk_usage", "arguments": {}})["result"]["content"]))


if __name__ == "__main__":
    if sys.argv[1:] == ["serve"]:
        serve()
    elif len(sys.argv) == 4 and sys.argv[1] == "drive":
        drive(sys.argv[2], sys.argv[3])
    else:
        sys.exit(__doc__)
