import { spawn } from "node:child_process";
import { createServer } from "node:http";

const PORT = Number(process.env.PORT || 8787);
const SERVER = process.env.MCP_SERVER || "@modelcontextprotocol/server-system-monitor";

// One long-lived stdio session, so a widget's second button press lands on the
// same server process as its first.
const srv = spawn("npx", ["-y", SERVER, "--stdio"], {
  stdio: ["pipe", "pipe", "inherit"],
});
let buf = "";
const pending = new Map();
srv.stdout.on("data", (d) => {
  buf += d.toString();
  let i;
  while ((i = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, i).trim();
    buf = buf.slice(i + 1);
    if (!line) continue;
    let m;
    try { m = JSON.parse(line); } catch { continue; }
    if (m.id !== undefined && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  }
});
let nextId = 1;
const rpc = (method, params) =>
  new Promise((res) => {
    const id = nextId++;
    pending.set(id, res);
    srv.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
  });

await rpc("initialize", {
  protocolVersion: "2026-01-26",
  capabilities: {},
  clientInfo: { name: "kitty-terminal-host", version: "0.0.1" },
});
srv.stdin.write(JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }) + "\n");

const toolList = (await rpc("tools/list", {})).result;
const appTool = toolList.tools.find((t) => t._meta?.ui?.resourceUri);
const uiUri = appTool?._meta?.ui?.resourceUri;
const widget = (await rpc("resources/read", { uri: uiUri })).result.contents[0];
const instantiating = (await rpc("tools/call", { name: appTool.name, arguments: {} })).result;

console.log(`${SERVER}: tool ${appTool.name}, widget ${uiUri}, ${widget.text.length} bytes`);

// --- the frame log --------------------------------------------------------
const frames = [];
const logFrame = (dir, method, note) => {
  const row = `${dir} ${method}${note ? "   " + note : ""}`;
  frames.push(row);
  console.log(row);
};

// The tools/call branch is the seam: it forwards to the live session above
// rather than replaying a captured result.
const page = `<!doctype html><meta charset="utf-8">
<style>html,body{margin:0;background:#12101a}iframe{border:0;width:100vw;height:100vh}</style>
<iframe id="w" src="/widget.html" sandbox="${process.env.SANDBOX || 'allow-scripts'}"></iframe>
<script>
const fr = document.getElementById('w');
const TOOL_RESULT = ${JSON.stringify(instantiating)};
const log = (d, m, n) => fetch('/log', {method:'POST', body: JSON.stringify({d, m, n})});
const post = (m) => fr.contentWindow.postMessage(m, '*');
let openLinkSeq = 0;
// postMessage is structured clone, not JSON, so an undefined-valued key
// survives the hop and reads as a present error member. Send one or the other.
const proxy = (m) => fetch('/rpc', {method:'POST', body: JSON.stringify({method:m.method, params:m.params})})
  .then(r => r.json())
  .then(j => {
    const reply = {jsonrpc:"2.0", id:m.id};
    if (j.error) { reply.error = j.error; } else { reply.result = j.result; }
    post(reply);
    log('->', j.error ? 'error' : 'result', m.method + ' live');
  });

window.addEventListener('message', (e) => {
  const m = e.data;
  if (!m || m.jsonrpc !== "2.0") return;
  log('<-', m.method || 'result', m.id !== undefined ? 'id:' + m.id : '');
  if (m.method === "ui/initialize") {
    post({jsonrpc:"2.0", id:m.id, result:{
      protocolVersion:"2026-01-26",
      hostInfo:{name:"kitty-terminal-host", version:"0.0.1"},
      hostCapabilities:{openLinks:{}, serverTools:{listChanged:false},
                        serverResources:{listChanged:false}},
      hostContext:{theme:"dark", displayMode:"inline",
                   availableDisplayModes:["inline"],
                   containerDimensions:{width:innerWidth, height:innerHeight}}}});
    log('->', 'result', 'ui/initialize');
  } else if (m.method === "ui/notifications/initialized") {
    post({jsonrpc:"2.0", method:"ui/notifications/tool-result", params:TOOL_RESULT});
    log('->', 'ui/notifications/tool-result', 'instantiating');
  } else if (m.method === "ui/open-link") {
    openLinkSeq++;
    log('<-', 'ui/open-link', JSON.stringify(m.params) + ' seq:' + openLinkSeq);
    if (openLinkSeq === 1) {
      post({jsonrpc:"2.0", id:m.id, error:{code:-32601, message:"not implemented: ui/open-link"}});
      log('->', 'error -32601', 'ui/open-link');
    } else if (openLinkSeq === 2) {
      post({jsonrpc:"2.0", id:m.id, result:{isError:true}});
      log('->', 'result isError:true', 'ui/open-link refused');
    } else {
      post({jsonrpc:"2.0", id:m.id, result:{isError:false}});
      log('->', 'result isError:false', 'ui/open-link accepted');
    }
  } else if (m.method === "tools/call" || m.method === "tools/list" ||
             m.method === "resources/read" || m.method === "resources/list") {
    proxy(m);
  } else if (m.id !== undefined && m.method) {
    post({jsonrpc:"2.0", id:m.id, error:{code:-32601, message:"not implemented: " + m.method}});
    log('->', 'error -32601', m.method);
  }
});
</script>`;

createServer(async (req, res) => {
  const url = new URL(req.url, "http://127.0.0.1");
  if (url.pathname === "/") {
    res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
    return res.end(page);
  }
  if (url.pathname === "/widget.html") {
    res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
    return res.end(widget.text);
  }
  if (url.pathname === "/frames") {
    res.writeHead(200, { "content-type": "text/plain" });
    return res.end(frames.join("\n"));
  }
  let body = "";
  for await (const c of req) body += c;
  if (url.pathname === "/log") {
    const { d, m, n } = JSON.parse(body);
    logFrame(d, m, n);
    res.writeHead(204);
    return res.end();
  }
  if (url.pathname === "/rpc") {
    const { method, params } = JSON.parse(body);
    const out = await rpc(method, params);
    logFrame("  ", "live " + method, JSON.stringify(params?.name ?? ""));
    res.writeHead(200, { "content-type": "application/json" });
    return res.end(JSON.stringify({ result: out.result, error: out.error }));
  }
  res.writeHead(404);
  res.end();
}).listen(PORT, "127.0.0.1", () => console.log(`host on http://127.0.0.1:${PORT}`));
