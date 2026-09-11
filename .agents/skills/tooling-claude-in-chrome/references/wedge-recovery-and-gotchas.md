# Wedge recovery, gotchas, anti-signals

## When the session wedges anyway

Symptoms: every call returns `Cannot access a chrome-extension:// URL of different extension` even though `tabs_context_mcp` works.

In order of cost:

1. `select_browser` with the deviceId from `list_connected_browsers` - sometimes nudges the session back. Cheap, try first.
2. Ask the human to click on the target tab to refocus it, and dismiss any extension popup.
3. Ask the human to **Cmd+Q Chrome** and relaunch, then reattach the Claude in Chrome extension by clicking its toolbar icon. The Bash path to quit Chrome (`osascript ... quit`) is in the harness deny list.

Don't loop on a wedged session - it stays wedged until human intervention.

## Other gotchas

* `browser_batch` is significantly faster than serial calls. Use it whenever 2+ steps have no inter-dependency. The runtime nags about this.
* `tabs_close_mcp` can hang ("did not respond in time") if a Chrome dialog is up. Resolve the dialog first.
* Repo lists, dropdowns, and async-loaded UIs need explicit waits after the trigger event. `await`/promises inside `javascript_tool` return their resolved value, so polling with `setTimeout` + `Promise` works.

## Anti-signals

* "Just click the button" - if the button is part of a React controlled form, click via JS.
* "It worked once with the mouse" - one success doesn't mean the path is reliable. The wedge is non-deterministic and triggered by extension focus shifts you can't observe.
* "I'll navigate away and come back" - if the form is dirty, the Leave site modal will fire.
