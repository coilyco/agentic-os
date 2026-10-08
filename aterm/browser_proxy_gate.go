package main

import (
	"encoding/json"
	"strings"
)

// personHasControl is the CDP error an agent gets while a person drives the browser,
// which Playwright surfaces as the tool's error.
const personHasControl = "a person has control of this browser (aterm); wait for it to be handed back"

// personHolds is whether a person has taken the browser from the agent.
func (sb *sharedBrowser) personHolds() bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.driver == "person"
}

// passiveWhilePersonDrives is what an agent may still send while a person holds control:
// the bookkeeping a Playwright connection needs to attach and keep its page model.
func passiveWhilePersonDrives(method string) bool {
	return strings.HasSuffix(method, ".enable") || method == "Runtime.runIfWaitingForDebugger" ||
		method == "Browser.getVersion" || attachMethods[method]
}

// attachMethods are the attach calls and page init Playwright 0.0.78 sends beside the
// enables, found against a real Chromium. Refused, a page attached mid-control breaks.
var attachMethods = map[string]bool{
	"Target.setAutoAttach":                  true,
	"Target.attachToTarget":                 true,
	"Target.detachFromTarget":               true,
	"Target.getTargetInfo":                  true,
	"Target.getTargets":                     true,
	"Page.getFrameTree":                     true,
	"Page.setLifecycleEventsEnabled":        true,
	"Page.addScriptToEvaluateOnNewDocument": true,
	"Page.createIsolatedWorld":              true,
	"Page.setFontFamilies":                  true,
	"Page.setInterceptFileChooserDialog":    true,
}

// gate refuses the agent's command while a person holds control. Events keep flowing
// to the agent, so its page model follows the person, and handing back needs no replay.
func (a *agentConn) gate(request agentRequest) *cdpErrorBody {
	if !a.sb.personHolds() || passiveWhilePersonDrives(request.Method) {
		return nil
	}
	return &cdpErrorBody{Code: -32000, Message: personHasControl}
}

// refuse answers a gated command. Playwright hides a protocol error from an evaluation as
// "Execution context was destroyed" but reports a thrown one as sent, so it gets that.
func (a *agentConn) refuse(request agentRequest, refusal *cdpErrorBody) {
	if request.Method != "Runtime.evaluate" && request.Method != "Runtime.callFunctionOn" {
		a.reply(request, nil, refusal)
		return
	}
	thrown := map[string]any{"type": "object", "subtype": "error", "className": "Error", "description": refusal.Message}
	result, _ := json.Marshal(map[string]any{
		"result":           thrown,
		"exceptionDetails": map[string]any{"exceptionId": 1, "text": "Uncaught", "lineNumber": 0, "columnNumber": 0, "exception": thrown},
	})
	a.reply(request, result, nil)
}
