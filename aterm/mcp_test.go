package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var mcpToolNames = []string{"ask_choice", "clear_session", "close_session", "list_agents", "read_messages", "send_message", "session_status"}

func fakeBackend() mcpBackend {
	return mcpBackend{
		listAgents: func() ([]sessionView, error) {
			return []sessionView{{Name: "eng-platform-beetle-ox", Role: "eng-platform"}}, nil
		},
		send: func(target, body string, opts sendOptions) (peerMessage, error) {
			if target == "nobody" {
				return peerMessage{}, errors.New("no live session answers to nobody")
			}
			state := "delivered"
			if body == "fail" {
				state = "failed"
			}
			return peerMessage{ID: "m1", Target: target, State: state, Reason: opts.Wait.String()}, nil
		},
		status: func(target string, lines int) (sessionStatus, error) {
			if target == "ox" {
				return sessionStatus{}, errors.New("status of " + target)
			}
			return sessionStatus{Name: target}, nil
		},
		clear: func(target string, force bool) (string, string, error) {
			return target, "/clear", nil
		},
		close: func(target string, force bool) (string, int, error) {
			return target, 0, nil
		},
		inbox: func(all bool) ([]inboxMessage, error) {
			if all {
				return nil, errors.New("inbox of all")
			}
			return []inboxMessage{{ID: "m9", From: "eng-platform Beetle-Ox", Text: "[from eng-platform Beetle-Ox] hi"}}, nil
		},
		ask: func(ask choiceAsk) (choiceAnswer, error) {
			if ask.Question == "cancel" {
				return choiceAnswer{State: "cancelled", Reason: "a client dismissed it"}, nil
			}
			return choiceAnswer{State: "answered", Picks: []int{0}, Labels: []string{ask.Options[0].Label}}, nil
		},
	}
}

// mcpClient connects the SDK's own client to newMCPServer over in-memory
// transports, so what a harness would see is what the test asserts.
func mcpClient(t *testing.T, backend mcpBackend) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	serverSide, clientSide := mcp.NewInMemoryTransports()
	serverSession, err := newMCPServer(backend).Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, arguments any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: a tool failure must be output, not a protocol error: %v", name, err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if block, ok := content.(*mcp.TextContent); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String(), result.IsError
}

func TestMCPListsTheSevenToolsWithTypedSchemas(t *testing.T) {
	listed, err := mcpClient(t, fakeBackend()).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	required := map[string][]string{}
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
		schema, _ := json.Marshal(tool.InputSchema)
		var decoded struct {
			Required             []string       `json:"required"`
			Properties           map[string]any `json:"properties"`
			AdditionalProperties any            `json:"additionalProperties"`
		}
		if err := json.Unmarshal(schema, &decoded); err != nil {
			t.Fatalf("%s schema: %v", tool.Name, err)
		}
		required[tool.Name] = decoded.Required
		if tool.Name == "ask_choice" {
			for _, field := range []string{"question", "questions", "options", "header", "allow_other", "multi"} {
				if _, ok := decoded.Properties[field]; !ok {
					t.Errorf("ask_choice schema lacks %q: %s", field, schema)
				}
			}
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(mcpToolNames, ",") {
		t.Fatalf("tools = %v, want %v", names, mcpToolNames)
	}
	if strings.Join(required["send_message"], ",") != "to,message" {
		t.Fatalf("send_message requires %v", required["send_message"])
	}
}

func TestMCPCallsAllSevenTools(t *testing.T) {
	session := mcpClient(t, fakeBackend())
	cases := []struct {
		tool      string
		arguments map[string]any
		want      string
	}{
		{"list_agents", map[string]any{}, `"name": "eng-platform-beetle-ox"`},
		{"send_message", map[string]any{"to": "ox", "message": "hi", "wait_seconds": 2}, "delivered m1 to ox: 2s"},
		{"session_status", map[string]any{"to": "beetle", "lines": 5}, `"beetle"`},
		{"read_messages", map[string]any{}, `"text": "[from eng-platform Beetle-Ox] hi"`},
		{"clear_session", map[string]any{"to": "ox"}, "typed /clear into ox"},
		{"close_session", map[string]any{"to": "ox", "force": true}, "closed ox (exit 0)"},
		{"ask_choice", map[string]any{"question": "Ship?", "options": []map[string]any{{"label": "yes"}}},
			`{"labels":["yes"],"picks":[0],"text":""}`},
	}
	for _, c := range cases {
		out, failed := callTool(t, session, c.tool, c.arguments)
		if failed || !strings.Contains(out, c.want) {
			t.Errorf("%s: got %q failed=%v, want %q", c.tool, out, failed, c.want)
		}
	}
}

func TestMCPReportsToolFailuresAsOutputNotProtocolErrors(t *testing.T) {
	session := mcpClient(t, fakeBackend())
	cases := []struct {
		name      string
		tool      string
		arguments map[string]any
		want      string
	}{
		{"backend error", "send_message", map[string]any{"to": "nobody", "message": "hi"}, "no live session answers to nobody"},
		{"failed delivery", "send_message", map[string]any{"to": "ox", "message": "fail"}, "failed m1 to ox"},
		{"inbox error", "read_messages", map[string]any{"all": true}, "inbox of all"},
		{"status error", "session_status", map[string]any{"to": "ox"}, "status of ox"},
		{"cancelled ask", "ask_choice", map[string]any{"question": "cancel", "options": []map[string]any{{"label": "a"}}},
			"the ask was cancelled: a client dismissed it"},
		{"both ask shapes", "ask_choice", map[string]any{"question": "x", "options": []map[string]any{{"label": "a"}},
			"questions": []map[string]any{{"question": "y", "options": []map[string]any{{"label": "b"}}}}},
			"give either question or questions, not both"},
		{"missing required field", "send_message", map[string]any{"message": "hi"}, "to"},
		{"wrong type", "session_status", map[string]any{"to": "ox", "lines": "many"}, "lines"},
	}
	for _, c := range cases {
		out, failed := callTool(t, session, c.tool, c.arguments)
		if !failed || !strings.Contains(out, c.want) {
			t.Errorf("%s: got %q failed=%v, want an isError result containing %q", c.name, out, failed, c.want)
		}
	}
}

func TestMCPUnknownToolIsAProtocolError(t *testing.T) {
	_, err := mcpClient(t, fakeBackend()).CallTool(context.Background(), &mcp.CallToolParams{Name: "nope"})
	if err == nil {
		t.Fatal("a tool that does not exist has no result to carry isError")
	}
}

// TestServeMCPSpeaksNewlineDelimitedJSONOnStdio drives serveMCP with raw lines,
// the way a harness's stdio launcher does, and holds stdout to JSON-RPC only.
func TestServeMCPSpeaksNewlineDelimitedJSONOnStdio(t *testing.T) {
	toServer, fromHarness := io.Pipe()
	toHarness, fromServer := io.Pipe()
	served := make(chan error, 1)
	go func() {
		served <- serveMCP(context.Background(), toServer, fromServer)
		_ = fromServer.Close()
	}()
	replies := bufio.NewScanner(toHarness)
	replies.Buffer(make([]byte, 64<<10), maxFrame)
	send := func(line string) {
		if _, err := io.WriteString(fromHarness, line+"\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	read := func() map[string]json.RawMessage {
		t.Helper()
		if !replies.Scan() {
			t.Fatalf("server closed early: %v", replies.Err())
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(replies.Bytes(), &frame); err != nil || string(frame["jsonrpc"]) != `"2.0"` {
			t.Fatalf("stdout carried something other than JSON-RPC: %q", replies.Text())
		}
		return frame
	}

	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	initialize := read()
	if !strings.Contains(string(initialize["result"]), `"protocolVersion":"2025-03-26"`) ||
		!strings.Contains(string(initialize["result"]), `"name":"aterm"`) {
		t.Fatalf("initialize should negotiate the client's revision and name aterm: %s", initialize["result"])
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	list := read()
	for _, tool := range mcpToolNames {
		if !strings.Contains(string(list["result"]), `"name":"`+tool+`"`) {
			t.Fatalf("tools/list is missing %s: %s", tool, list["result"])
		}
	}
	send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"send_message","arguments":{"message":"hi"}}}`)
	call := read()
	if call["error"] != nil || !strings.Contains(string(call["result"]), `"isError":true`) {
		t.Fatalf("a bad call is an isError result, not a JSON-RPC error: %v", call)
	}

	_ = fromHarness.Close()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("closing stdin is a clean end, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveMCP did not return after its input closed")
	}
}
