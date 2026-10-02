package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfave/cli/v3"
)

// mcpProtocol is the revision this server answers with when a client asks for
// one it does not name. Tools are all it offers, so older clients fit too.
const mcpProtocol = "2025-06-18"

func newMCPCommand() *cli.Command {
	return &cli.Command{
		Name:  "mcp",
		Usage: "serve list_agents, send_message, session_status, clear_session, close_session and ask_choice over MCP stdio",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return serveMCP(os.Stdin, cmd.Root().Writer)
		},
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var mcpTools = []map[string]any{
	{
		"name": "ask_choice",
		"description": "Ask Kai a multiple-choice question on her aterm client and wait for the pick. " +
			"aterm stamps your seat on it. Returns the picked labels and any free text. A cancelled " +
			"or timed-out ask returns an error saying which. Give one question, or `questions` for up " +
			"to four, which Kai answers one card at a time and which return an answer each.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questions": map[string]any{
					"type":        "array",
					"description": "up to four questions, each shaped like the single-question fields here, instead of them",
					"items":       map[string]any{"type": "object"},
				},
				"question":    map[string]any{"type": "string"},
				"header":      map[string]any{"type": "string", "description": "a short label above the question"},
				"allow_other": map[string]any{"type": "boolean", "description": "let Kai type an answer instead"},
				"multi":       map[string]any{"type": "boolean", "description": "allow more than one pick"},
				"options": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"label"},
						"properties": map[string]any{
							"label":       map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	},
	{
		"name": "list_agents",
		"description": "List the live agent sessions on this host that send_message can reach. " +
			"Each carries its session name, role, identity, and harness. `self` is the caller.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name": "send_message",
		"description": "Type a message into another live agent session. The recipient sees it prefixed " +
			"`[from <your role> <your identity>]`, which aterm stamps and you cannot change. " +
			"Address it by role slug, identity, harness, or session name from list_agents. Returns " +
			"queued, held (with the reason), delivered, or failed. A message still queued or held when " +
			"this returns is followed by a `[from aterm daemon]` receipt typed into your own session " +
			"when it lands or fails, so do not poll. `notify_when_idle` adds one `[from aterm daemon]` line " +
			"when the recipient next goes idle, or ends.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"to", "message"},
			"properties": map[string]any{
				"to":      map[string]any{"type": "string", "description": "role slug, identity, harness, or session name"},
				"message": map[string]any{"type": "string"},
				"launch":  map[string]any{"type": "boolean", "description": "open the role when no session answers, and deliver into it"},
				"wait_seconds": map[string]any{"type": "integer", "description": "hold the answer up to this long, " +
					"at most 120, for delivered or failed, instead of 3 seconds"},
				"notify_when_idle": map[string]any{"type": "boolean", "description": "type one line into your session " +
					"when the recipient next goes idle after the message lands"},
				"new": map[string]any{"type": "boolean", "description": "open a new instance of the role even when one is live, " +
					"deliver into it, and return its session name. `to` must be a role slug"},
			},
		},
	},
	{
		"name": "session_status",
		"description": "Read another live agent session without typing into it: its state (starting, prompt, " +
			"busy, idle), whether it sits on a permission or choice prompt and the prompt text, seconds since " +
			"it last wrote and since anyone typed, whether Kai has a draft, and the last rows of its screen. " +
			"`to` resolves as in send_message. Use it to tell a seat waiting on Kai from one still working.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"to"},
			"properties": map[string]any{
				"to":    map[string]any{"type": "string", "description": "session name from list_agents, or role, identity, or harness"},
				"lines": map[string]any{"type": "integer", "description": "screen rows to return, default 30, at most 200"},
			},
		},
	},
	{
		"name": "clear_session",
		"description": "Start another live agent session over by typing its harness's clear command, which " +
			"discards its context. Only the director role may call it. `to` resolves as in send_message. " +
			"It refuses your own session, a session on a permission or choice prompt, and unless force is " +
			"set one that is busy, holds Kai's unsent draft, or has undelivered messages.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"to"},
			"properties": map[string]any{
				"to":    map[string]any{"type": "string", "description": "session name from list_agents, or role, identity, or harness"},
				"force": map[string]any{"type": "boolean", "description": "clear even a busy session, or one holding a draft or undelivered messages"},
			},
		},
	},
	{
		"name": "close_session",
		"description": "End another live agent session and drop it from aterm, which closing its window does not. " +
			"`to` resolves as in send_message, and a target matching several sessions refuses. It will not " +
			"close your own session or one you run inside, and one holding Kai's unsent draft or undelivered " +
			"messages stays open unless force is set.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"to"},
			"properties": map[string]any{
				"to":    map[string]any{"type": "string", "description": "session name from list_agents, or role, identity, or harness"},
				"force": map[string]any{"type": "boolean", "description": "close even while it holds a draft or undelivered messages"},
			},
		},
	},
}

// serveMCP is newline-delimited JSON-RPC, the stdio transport. It reads its
// token from the environment the harness started it with.
func serveMCP(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), maxFrame)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}})
			continue
		}
		if len(request.ID) == 0 {
			continue // a notification wants no answer
		}
		response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
		result, err := answerMCP(request)
		if err != nil {
			response.Error = err
		} else {
			response.Result = result
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func answerMCP(request rpcRequest) (any, *rpcError) {
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		protocol := params.ProtocolVersion
		if protocol == "" {
			protocol = mcpProtocol
		}
		return map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "aterm", "version": version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		text, failed := callMCPTool(params.Name, params.Arguments)
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": failed,
		}, nil
	}
	return nil, &rpcError{Code: -32601, Message: "method not found: " + request.Method}
}

// callMCPTool reports a failed call as tool output rather than a protocol
// error, so the agent reads why and can act on it.
func callMCPTool(name string, arguments json.RawMessage) (string, bool) {
	switch name {
	case "list_agents":
		views, err := listAgents()
		if err != nil {
			return err.Error(), true
		}
		encoded, _ := json.MarshalIndent(agentsDocument(views), "", "  ")
		return string(encoded), false
	case "ask_choice":
		return askFromArguments(arguments, askChoice)
	case "send_message":
		var params struct {
			To      string `json:"to"`
			Message string `json:"message"`
			Launch  bool   `json:"launch"`
			New     bool   `json:"new"`
			Wait    int    `json:"wait_seconds"`
			Idle    bool   `json:"notify_when_idle"`
		}
		if err := json.Unmarshal(arguments, &params); err != nil {
			return err.Error(), true
		}
		state, err := sendMessage(params.To, params.Message, sendOptions{Launch: params.Launch, Fresh: params.New,
			NotifyIdle: params.Idle, Wait: time.Duration(params.Wait) * time.Second})
		if err != nil {
			return err.Error(), true
		}
		return describeMessage(state), state.State == "failed"
	case "session_status":
		var params struct {
			To    string `json:"to"`
			Lines int    `json:"lines"`
		}
		if err := json.Unmarshal(arguments, &params); err != nil {
			return err.Error(), true
		}
		status, err := sessionStatusOf(params.To, params.Lines)
		if err != nil {
			return err.Error(), true
		}
		encoded, _ := json.MarshalIndent(status, "", "  ")
		return string(encoded), false
	case "clear_session":
		var params struct {
			To    string `json:"to"`
			Force bool   `json:"force"`
		}
		if err := json.Unmarshal(arguments, &params); err != nil {
			return err.Error(), true
		}
		name, command, err := clearTarget(params.To, params.Force)
		if err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("typed %s into %s", command, name), false
	case "close_session":
		var params struct {
			To    string `json:"to"`
			Force bool   `json:"force"`
		}
		if err := json.Unmarshal(arguments, &params); err != nil {
			return err.Error(), true
		}
		name, code, err := closeSession(params.To, params.Force)
		if err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("closed %s (exit %d)", name, code), false
	}
	return fmt.Sprintf("no tool named %q", name), true
}
