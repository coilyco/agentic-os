package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"
)

// mcpProtocol is the revision this server answers with when a client asks for
// one it does not name. Tools are all it offers, so older clients fit too.
const mcpProtocol = "2025-06-18"

func newMCPCommand() *cli.Command {
	return &cli.Command{
		Name:  "mcp",
		Usage: "serve list_agents and send_message over MCP stdio, the tool front door to `aterm send`",
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
		"name": "list_agents",
		"description": "List the live agent sessions on this host that send_message can reach. " +
			"Each carries its session name, role, identity, and harness. `self` is the caller.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name": "send_message",
		"description": "Type a message into another live agent session. The recipient sees it prefixed " +
			"`[from <your role> <your identity>]`, which aterm stamps and you cannot change. " +
			"Address it by role slug, identity, harness, or session name from list_agents.",
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"to", "message"},
			"properties": map[string]any{
				"to":      map[string]any{"type": "string", "description": "role slug, identity, harness, or session name"},
				"message": map[string]any{"type": "string"},
				"launch":  map[string]any{"type": "boolean", "description": "open the role when no session answers, and deliver into it"},
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
	case "send_message":
		var params struct {
			To      string `json:"to"`
			Message string `json:"message"`
			Launch  bool   `json:"launch"`
		}
		if err := json.Unmarshal(arguments, &params); err != nil {
			return err.Error(), true
		}
		state, err := sendMessage(params.To, params.Message, params.Launch)
		if err != nil {
			return err.Error(), true
		}
		return describeMessage(state), state.State == "failed"
	}
	return fmt.Sprintf("no tool named %q", name), true
}
