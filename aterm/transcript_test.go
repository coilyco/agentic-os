package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures in testdata/transcript are real file structure, one per harness, with the
// text, paths and ids replaced. Each has a golden reply a client's mock can replay.

func transcriptFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "transcript", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func transcriptTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// golden compares a reply's items to the checked-in answer. UPDATE_GOLDEN=1 rewrites it.
func transcriptGolden(t *testing.T, name string, reply transcriptReply) {
	t.Helper()
	shown := struct {
		Reader string           `json:"reader"`
		Items  []transcriptItem `json:"items"`
		More   bool             `json:"more"`
	}{reply.Reader, reply.Items, reply.More}
	raw, err := json.MarshalIndent(shown, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "transcript", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with UPDATE_GOLDEN=1 to write it)", err)
	}
	if strings.TrimSpace(string(want)) != strings.TrimSpace(string(raw)) {
		t.Fatalf("%s differs from the reply:\n%s", name, raw)
	}
}

func itemsByID(items []transcriptItem) map[string]transcriptItem {
	out := map[string]transcriptItem{}
	for _, item := range items {
		out[item.ID] = item
	}
	return out
}

func itemKinds(items []transcriptItem) string {
	var out []string
	for _, item := range items {
		out = append(out, item.Kind)
	}
	return strings.Join(out, " ")
}

var transcriptLedger = []ledgerEntry{
	{Name: "sysadmin-senior-turtle-ox", Role: "sysadmin-senior", Identity: "Turtle-Ox"},
	{Name: "prod-director-griffin-goose", Role: "prod-director", Identity: "Griffin-Goose"},
}

func TestClaudeTranscriptReadsPromptsRepliesToolsAndPeers(t *testing.T) {
	path := transcriptTempFile(t, transcriptFixture(t, "claude.jsonl"))
	reply, err := claudeTranscript(path, transcriptRequest{Limit: 200}, transcriptLedger)
	if err != nil {
		t.Fatal(err)
	}
	transcriptGolden(t, "claude.golden.json", reply)
	items := itemsByID(reply.Items)

	// A failed shell call, an edit with its diffstat, and a passing call.
	if tool := items["toolu_1"].Tool; tool == nil || tool.OK == nil || *tool.OK || tool.Summary != "Ran just aterm-client-gate" || !strings.Contains(tool.Output, "3 failed") {
		t.Fatalf("failed shell = %+v", tool)
	}
	if tool := items["toolu_2"].Tool; tool == nil || tool.Class != "edit" || tool.Diff == nil || tool.Diff.Added != 2 || tool.Diff.Removed != 1 || tool.Summary != "Edited Terminal.svelte +2 -1" || tool.Output != "" {
		t.Fatalf("edit = %+v", tool)
	}
	if tool := items["toolu_3"].Tool; tool == nil || tool.OK == nil || !*tool.OK || tool.Output != "847 passed" {
		t.Fatalf("shell = %+v", tool)
	}
	// One content block is one line, and a reply's blocks merge into one item.
	if reply := items["msg_A"]; reply.Kind != "reply" || reply.Text != "I will run the gate first." || reply.Model != "claude-sonnet-5-5" {
		t.Fatalf("merged reply = %+v", reply)
	}
	if items["msg_D"].Complete == nil || !*items["msg_D"].Complete {
		t.Fatal("an end_turn reply is complete")
	}
	// Thinking, a subagent, a system reminder and api-request lines are not conversation.
	for _, id := range []string{"00000000-0000-4000-8000-000000000020", "00000000-0000-4000-8000-000000000021"} {
		if _, ok := items[id]; ok {
			t.Fatalf("%s is not conversation", id)
		}
	}
}

func TestClaudeTranscriptTagsEveryPathAPeerMessageTakes(t *testing.T) {
	reply, err := claudeTranscript(transcriptTempFile(t, transcriptFixture(t, "claude.jsonl")), transcriptRequest{Limit: 200}, transcriptLedger)
	if err != nil {
		t.Fatal(err)
	}
	items := itemsByID(reply.Items)
	const stamped, queued, native = "00000000-0000-4000-8000-000000000011", "00000000-0000-4000-8000-000000000015", "00000000-0000-4000-8000-000000000018"

	// aterm send types a stamped line into the prompt: a user line, and its answer is for
	// it.
	if peer := items[stamped]; peer.Kind != "peer" || peer.From == nil || peer.From.Role != "prod-director" || peer.From.Identity != "Griffin-Goose" || peer.Text != "Please add a note to COI-3532." {
		t.Fatalf("aterm send peer = %+v", peer)
	}
	if reply := items["msg_E"]; reply.For != stamped || reply.Shared {
		t.Fatalf("the answer to an idle peer = %+v", reply)
	}
	// A message queued while a turn ran lands in that turn, so its answers are shared.
	if peer := items[queued]; peer.Kind != "peer" || peer.From == nil || peer.From.Role != "prod-director" || peer.Text != "Reminder: merge on green." {
		t.Fatalf("queued peer = %+v", peer)
	}
	if reply := items["msg_G"]; reply.For != queued || !reply.Shared {
		t.Fatalf("the answer to a mid-turn peer must be for it and shared: %+v", reply)
	}
	// What was already running before it landed answers Kai, not the peer.
	if tool := items["toolu_4"]; tool.For != "" || tool.Shared {
		t.Fatalf("a call made before the peer landed is not for it: %+v", tool)
	}
	// Claude's own cross-session message carries a structured origin, resolved to a role.
	if peer := items[native]; peer.Kind != "peer" || peer.From == nil || peer.From.Role != "sysadmin-senior" || peer.From.Identity != "Turtle-Ox" {
		t.Fatalf("cross-session peer = %+v", peer)
	}
	if reply := items["msg_H"]; reply.For != native || reply.Shared {
		t.Fatalf("the answer to a cross-session peer = %+v", reply)
	}
	// Kai's own turns stay prompts and answer no peer.
	for _, id := range []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000013"} {
		if items[id].Kind != "prompt" {
			t.Fatalf("%s should be Kai's prompt: %+v", id, items[id])
		}
	}
	for _, id := range []string{"msg_A", "msg_B", "msg_C", "msg_D", "toolu_1", "toolu_2"} {
		if items[id].For != "" {
			t.Fatalf("%s answers Kai and must carry no for: %+v", id, items[id])
		}
	}
	// A cross-session sender the ledger does not know keeps no role.
	unknown, err := claudeTranscript(transcriptTempFile(t, transcriptFixture(t, "claude.jsonl")), transcriptRequest{Limit: 200}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if peer := itemsByID(unknown.Items)[native]; peer.From == nil || peer.From.Role != "" || peer.From.Identity != "sysadmin-senior-turtle-ox" {
		t.Fatalf("an unknown sender = %+v", peer.From)
	}
}

func TestTranscriptPagesOldestFirstWithOlderAndSinceCursors(t *testing.T) {
	path := transcriptTempFile(t, transcriptFixture(t, "claude.jsonl"))
	newest, err := claudeTranscript(path, transcriptRequest{Limit: 3}, transcriptLedger)
	if err != nil || len(newest.Items) != 3 || !newest.More || newest.Older == "" {
		t.Fatalf("newest page = %+v err = %v", newest, err)
	}
	all, _ := claudeTranscript(path, transcriptRequest{Limit: 200}, transcriptLedger)
	if got, want := newest.Items[0].ID, all.Items[len(all.Items)-3].ID; got != want {
		t.Fatalf("the newest page is the last three, oldest first: starts at %s, want %s", got, want)
	}
	older, err := claudeTranscript(path, transcriptRequest{Limit: 3, Before: newest.Older}, transcriptLedger)
	if err != nil || len(older.Items) != 3 || older.Items[len(older.Items)-1].ID == newest.Items[0].ID {
		t.Fatalf("older page = %+v err = %v", older, err)
	}
	if older.Items[len(older.Items)-1].pos >= newest.Items[0].pos {
		t.Fatal("an older page ends before the newer page begins")
	}
	// Nothing new since the cursor answers without items.
	quiet, err := claudeTranscript(path, transcriptRequest{Since: all.Cursor}, transcriptLedger)
	if err != nil || len(quiet.Items) != 0 || quiet.Cursor != all.Cursor {
		t.Fatalf("an unchanged file = %+v err = %v", quiet, err)
	}
	// A line appended after the cursor is the only thing returned.
	file, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	file.WriteString(`{"type":"assistant","uuid":"x1","parentUuid":"00000000-0000-4000-8000-000000000019","timestamp":"2026-11-10T20:50:00.000Z","message":{"id":"msg_Z","model":"m","stop_reason":"end_turn","content":[{"type":"text","text":"One more."}]}}` + "\n")
	file.Close()
	fresh, err := claudeTranscript(path, transcriptRequest{Since: all.Cursor}, transcriptLedger)
	if err != nil || len(fresh.Items) != 1 || fresh.Items[0].ID != "msg_Z" || fresh.Cursor == all.Cursor {
		t.Fatalf("since = %+v err = %v", fresh, err)
	}
}

func TestTranscriptHoldsTheCursorBackToARunningToolAndUpdatesItInPlace(t *testing.T) {
	var kept []string
	for _, line := range strings.Split(strings.TrimSpace(transcriptFixture(t, "claude.jsonl")), "\n") {
		if strings.Contains(line, `"toolu_4"`) && strings.Contains(line, "tool_result") {
			continue
		}
		kept = append(kept, line)
	}
	path := transcriptTempFile(t, strings.Join(kept, "\n")+"\n")
	first, err := claudeTranscript(path, transcriptRequest{Limit: 200}, transcriptLedger)
	if err != nil {
		t.Fatal(err)
	}
	running := itemsByID(first.Items)["toolu_4"]
	if running.Tool == nil || running.Tool.OK != nil {
		t.Fatalf("a call with no result has no ok yet: %+v", running.Tool)
	}
	file, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	file.WriteString(`{"type":"user","uuid":"r4","parentUuid":"00000000-0000-4000-8000-000000000014","timestamp":"2026-11-10T20:51:00.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_4","content":"done"}]}}` + "\n")
	file.Close()
	later, err := claudeTranscript(path, transcriptRequest{Since: first.Cursor}, transcriptLedger)
	if err != nil {
		t.Fatal(err)
	}
	updated := itemsByID(later.Items)["toolu_4"]
	if updated.Tool == nil || updated.Tool.OK == nil || !*updated.Tool.OK || updated.Tool.Output != "done" || updated.Tool.Summary != running.Tool.Summary {
		t.Fatalf("the finished tool replaces the running one by id: %+v", updated)
	}
}

func TestTranscriptWidensItsWindowUntilAPageFills(t *testing.T) {
	saved := transcriptWindow
	transcriptWindow = 300
	defer func() { transcriptWindow = saved }()
	path := transcriptTempFile(t, transcriptFixture(t, "claude.jsonl"))
	page, err := claudeTranscript(path, transcriptRequest{Limit: 200}, transcriptLedger)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := claudeTranscript(path, transcriptRequest{Limit: 200}, transcriptLedger)
	if len(page.Items) != len(full.Items) || page.More {
		t.Fatalf("a narrow window still returns everything: %d items, more=%v", len(page.Items), page.More)
	}
}

func TestTranscriptRefusesACursorItDidNotIssue(t *testing.T) {
	path := transcriptTempFile(t, transcriptFixture(t, "claude.jsonl"))
	for _, request := range []transcriptRequest{{Since: "nonsense"}, {Since: "f:999999999"}, {Before: "f:-1"}, {Before: "t:1:x"}} {
		_, err := claudeTranscript(path, request, nil)
		if reasonFor(err) != reasonBadCursor || !errors.Is(err, errBadCursor) {
			t.Fatalf("%+v: %v (reason %q)", request, err, reasonFor(err))
		}
	}
}

func TestCodexTranscriptUsesTheTypedItemsAndTurnsOrderAPeer(t *testing.T) {
	reply, err := fileTranscript("codex", transcriptTempFile(t, transcriptFixture(t, "codex.jsonl")), transcriptRequest{Limit: 200}, parseCodex)
	if err != nil {
		t.Fatal(err)
	}
	transcriptGolden(t, "codex.golden.json", reply)
	items := itemsByID(reply.Items)
	if tool := items["exec-1"].Tool; tool == nil || tool.OK == nil || *tool.OK || tool.Summary != "Ran just test" || !strings.Contains(tool.Output, "2 failed") {
		t.Fatalf("failed command = %+v", tool)
	}
	if tool := items["fc-1"].Tool; tool == nil || tool.Class != "edit" || tool.Diff == nil || tool.Diff.Added != 2 || tool.Diff.Removed != 1 || tool.Summary != "Edited table.go +2 -1" {
		t.Fatalf("file change = %+v", tool)
	}
	if tool := items["mcp-1"].Tool; tool == nil || tool.Class != "mcp" || tool.Summary != "Called linear.get_issue" || tool.Output != "COI-1 is open" {
		t.Fatalf("mcp call = %+v", tool)
	}
	if items["a2"].Model != "gpt-test" || items["a2"].Complete == nil || !*items["a2"].Complete {
		t.Fatalf("a final answer carries the turn's model and is complete: %+v", items["a2"])
	}
	// Reasoning is not sent, and a raw response_item is never read twice.
	if _, ok := items["rs1"]; ok {
		t.Fatal("reasoning is left out")
	}
	// A peer that starts its own turn is exact. One that lands mid-turn is shared.
	if peer := items["u2"]; peer.Kind != "peer" || peer.From == nil || peer.From.Identity != "Weevil-Ox" {
		t.Fatalf("peer = %+v", peer)
	}
	if reply := items["a3"]; reply.For != "u2" || reply.Shared {
		t.Fatalf("the answer to a peer that began its turn = %+v", reply)
	}
	if reply := items["a4"]; reply.For != "" {
		t.Fatalf("narration before a mid-turn peer answers Kai: %+v", reply)
	}
	if reply := items["a5"]; reply.For != "u4" || !reply.Shared {
		t.Fatalf("the answer after a mid-turn peer is for it and shared: %+v", reply)
	}
	if items["u3"].Kind != "prompt" || items["cc1"].Kind != "note" {
		t.Fatalf("prompt and note kinds: %+v %+v", items["u3"], items["cc1"])
	}
}

func opencodeFixtureDB(t *testing.T) ledgerEntry {
	t.Helper()
	needSqlite(t)
	home := t.TempDir()
	makeOpencodeDB(t, home, transcriptFixture(t, "opencode.sql"))
	return ledgerEntry{Name: "eng-platform-a-1111", Seat: "opencode", Cwd: "/work/demo", Home: home, Started: time.UnixMilli(1794000000000 - 1000)}
}

func TestOpencodeTranscriptReadsPartsAndTagsPeersByParent(t *testing.T) {
	entry := opencodeFixtureDB(t)
	reply, err := opencodeTranscript(entry, nil, transcriptRequest{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	transcriptGolden(t, "opencode.golden.json", reply)
	items := itemsByID(reply.Items)
	if tool := items["prt_b1"].Tool; tool == nil || tool.OK == nil || *tool.OK || tool.Summary != "Ran just test" || !strings.Contains(tool.Output, "2 failed") {
		t.Fatalf("a bash exit 1 is not ok: %+v", tool)
	}
	if tool := items["prt_e1"].Tool; tool == nil || tool.Class != "edit" || tool.Diff == nil || tool.Diff.Added != 2 || tool.Diff.Removed != 1 || tool.Summary != "Edited table.go +2 -1" {
		t.Fatalf("edit = %+v", tool)
	}
	if tool := items["prt_x1"].Tool; tool == nil || tool.OK == nil || *tool.OK || tool.Output != "Ripgrep JSON record exceeded 65536 bytes" {
		t.Fatalf("an error part is not ok and carries its error: %+v", tool)
	}
	if tool := items["prt_m1"].Tool; tool == nil || tool.Class != "mcp" || tool.Summary != "Called linear_get_issue" {
		t.Fatalf("an unknown tool name is an MCP server's: %+v", tool)
	}
	if tool := items["prt_r1"].Tool; tool == nil || tool.OK != nil {
		t.Fatalf("a running tool has no ok: %+v", tool)
	}
	if items["prt_t2"].Model != "model-test" || items["prt_t2"].Complete == nil || !*items["prt_t2"].Complete {
		t.Fatalf("a finished assistant reply = %+v", items["prt_t2"])
	}
	// Peers are told by the stamp. The idle one is exact, the queued one is shared.
	if peer := items["msg_u2"]; peer.Kind != "peer" || peer.From == nil || peer.From.Role != "eng-platform" {
		t.Fatalf("peer = %+v", peer)
	}
	if reply := items["prt_t3"]; reply.For != "msg_u2" || reply.Shared {
		t.Fatalf("answer to an idle peer = %+v", reply)
	}
	if reply := items["prt_t4"]; reply.For != "msg_u4" || !reply.Shared {
		t.Fatalf("answer to a mid-turn peer = %+v", reply)
	}
	if items["msg_u3"].Kind != "prompt" || items["prt_c1"].Kind != "note" {
		t.Fatalf("prompt and note: %+v %+v", items["msg_u3"], items["prt_c1"])
	}
	// Steps and reasoning parts are not conversation.
	for _, id := range []string{"prt_s1", "prt_f1"} {
		if _, ok := items[id]; ok {
			t.Fatalf("%s is not conversation", id)
		}
	}
}

func TestOpencodeTranscriptPagesAndHoldsTheCursorBeforeARunningTool(t *testing.T) {
	entry := opencodeFixtureDB(t)
	full, err := opencodeTranscript(entry, nil, transcriptRequest{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if got := itemsByID(full.Items)["prt_r1"]; got.Tool == nil || got.Tool.OK != nil {
		t.Fatal("the fixture ends with a tool still running")
	}
	// The cursor stops before the running tool, so a since read returns it again once it
	// ends.
	again, err := opencodeTranscript(entry, nil, transcriptRequest{Since: full.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := itemsByID(again.Items)["prt_r1"]; !ok {
		t.Fatalf("since must return the running tool again: %+v", itemKindsOf(again))
	}
	page, err := opencodeTranscript(entry, nil, transcriptRequest{Limit: 4})
	if err != nil || len(page.Items) != 4 || !page.More || page.Older == "" {
		t.Fatalf("page = %+v err = %v", page, err)
	}
	older, err := opencodeTranscript(entry, nil, transcriptRequest{Limit: 4, Before: page.Older})
	if err != nil || len(older.Items) == 0 || older.Items[len(older.Items)-1].ID == page.Items[0].ID {
		t.Fatalf("older = %+v err = %v", older, err)
	}
	if _, err := opencodeTranscript(entry, nil, transcriptRequest{Since: "t:1:x'; DROP TABLE part; --"}); reasonFor(err) != reasonBadCursor {
		t.Fatalf("a cursor that is not ours never reaches the query: %v", err)
	}
}

func itemKindsOf(reply transcriptReply) string { return itemKinds(reply.Items) }

func TestTranscriptNamesWhyItCannotRead(t *testing.T) {
	cases := map[string]struct {
		entry  ledgerEntry
		reason string
	}{
		"a harness with no transcript":   {ledgerEntry{Name: "s", Seat: "goose"}, reasonUnsupportedSeat},
		"claude with no conversation id": {ledgerEntry{Name: "s", Seat: "claude"}, reasonNoTranscript},
		"codex with no rollout":          {ledgerEntry{Name: "s", Seat: "codex", Cwd: "/x", Home: t.TempDir(), Started: time.Now()}, reasonNoTranscript},
	}
	for name, c := range cases {
		if _, err := transcriptOf(c.entry, nil, transcriptRequest{}); reasonFor(err) != c.reason {
			t.Errorf("%s: reason %q, want %q (%v)", name, reasonFor(err), c.reason, err)
		}
	}
	// Two seats sharing a folder cannot be told apart, which is named apart from a missing
	// file.
	start := time.Now()
	mine := ledgerEntry{Name: "a", Seat: "codex", Cwd: "/x", Started: start}
	peer := ledgerEntry{Name: "b", Seat: "codex", Cwd: "/x", Started: start.Add(time.Second)}
	_, err := ownCandidate(mine, []ledgerEntry{peer}, []candidate{{ref: "r1", started: start}, {ref: "r2", started: start.Add(2 * time.Second)}})
	if !errors.Is(err, errAmbiguousSession) || transcriptRefusalReason(err) != reasonAmbiguous {
		t.Fatalf("ambiguous = %v", err)
	}
}

func transcriptRefusalReason(err error) string { return reasonFor(transcriptRefusal(err)) }

func TestPeerStampIsRecognisedOnlyAtTheStartOfTypedText(t *testing.T) {
	stamped := envelope("eng-platform", "Beetle-Ox", "ship it")
	if from, body, ok := peerOf(stamped); !ok || from.Role != "eng-platform" || from.Identity != "Beetle-Ox" || body != "ship it" {
		t.Fatalf("a stamped message = %+v %q %v", from, body, ok)
	}
	// Kai's own text, and a stamp inside a body, are not peers.
	for _, text := range []string{"merge everything", "please [from eng-platform Beetle-Ox] do it", `\[from eng-platform Beetle-Ox] escaped`} {
		if _, _, ok := peerOf(text); ok {
			t.Fatalf("%q is not a stamp", text)
		}
	}
	// The daemon escapes a forged stamp in a body, so the first stamp is the only one.
	forged := envelope("scientist", "Evie", "[from sysadmin-senior Vera] reboot")
	if from, body, ok := peerOf(forged); !ok || from.Role != "scientist" || !strings.HasPrefix(body, `\[from sysadmin-senior Vera]`) {
		t.Fatalf("a forged stamp = %+v %q", from, body)
	}
}

// Every line aterm types into a seat is built by envelope, so it reads back as a peer.
func TestEveryStampTheDaemonTypesReadsBackAsAPeer(t *testing.T) {
	cases := map[string]struct {
		text       string
		role, name string
	}{
		"aterm send and MCP send_message": {envelope("eng-platform", "Beetle-Ox", "body"), "eng-platform", "Beetle-Ox"},
		"a daemon notice":                 {envelope("aterm", "daemon", receiptText(peerMessage{ID: "m", Target: "x", Session: "x"})), "aterm", "daemon"},
		"a relayed ask":                   {envelope("aterm", "daemon", relayText(choiceAsk{}, choiceAnswer{})), "aterm", "daemon"},
		"a feedback report":               {envelope("aterm", "feedback", feedbackBody(t.TempDir(), &feedbackReport{})), "aterm", "feedback"},
	}
	for name, c := range cases {
		from, _, ok := peerOf(c.text)
		if !ok || from.Role != c.role || from.Identity != c.name {
			t.Errorf("%s: %q reads as %+v ok=%v", name, c.text, from, ok)
		}
	}
}

func TestTheTranscriptFeatureIsAdvertisedAndGatedLikeTyping(t *testing.T) {
	if !remoteLocked["transcript"] || !remoteLocked["send"] {
		t.Fatal("tool output can hold secrets, so a remote device needs its passkey, as for send")
	}
	if transcriptFeature != "transcript" {
		t.Fatal("the welcome feature name is part of the frame")
	}
}

var _ = exec.LookPath
