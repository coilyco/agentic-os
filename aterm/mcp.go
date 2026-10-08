package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urfave/cli/v3"
)

func newMCPCommand() *cli.Command {
	return &cli.Command{
		Name:  "mcp",
		Usage: "serve list_agents, send_message, read_messages, session_status, clear_session, close_session and ask_choice over MCP stdio",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return serveMCP(ctx, os.Stdin, cmd.Root().Writer)
		},
	}
}

// mcpBackend is what each tool calls, so a test can stand in for the daemon.
// liveBackend is the real one.
type mcpBackend struct {
	listAgents func() ([]sessionView, error)
	send       func(target, body string, opts sendOptions) (peerMessage, error)
	status     func(target string, lines int) (sessionStatus, error)
	clear      func(target string, force bool) (string, string, error)
	close      func(target string, force bool) (string, int, error)
	ask        func(choiceAsk) (choiceAnswer, error)
	inbox      func(all bool) ([]inboxMessage, error)
}

var liveBackend = mcpBackend{
	listAgents: listAgents,
	send:       sendMessage,
	status:     sessionStatusOf,
	clear:      clearTarget,
	close:      closeSession,
	ask:        askChoice,
	inbox:      readInbox,
}

type askOptionInput struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type askQuestionInput struct {
	Question   string           `json:"question,omitempty"`
	Header     string           `json:"header,omitempty" jsonschema:"a short label above the question"`
	AllowOther bool             `json:"allow_other,omitempty" jsonschema:"let Kai type an answer instead"`
	Multi      bool             `json:"multi,omitempty" jsonschema:"allow more than one pick"`
	Options    []askOptionInput `json:"options,omitempty"`
}

type askChoiceInput struct {
	askQuestionInput
	Questions []askQuestionInput `json:"questions,omitempty" jsonschema:"up to four questions, each shaped like the single-question fields here, instead of them"`
}

type listAgentsInput struct{}

type sendMessageInput struct {
	To      string `json:"to" jsonschema:"role slug, identity, harness, or session name"`
	Message string `json:"message"`
	Launch  bool   `json:"launch,omitempty" jsonschema:"open the role when no session answers, and deliver into it"`
	Wait    int    `json:"wait_seconds,omitempty" jsonschema:"hold the answer up to this long, at most 120, for delivered or failed, instead of 3 seconds"`
	Idle    bool   `json:"notify_when_idle,omitempty" jsonschema:"type one line into your session when the recipient next goes idle after the message lands"`
	New     bool   `json:"new,omitempty" jsonschema:"open a new instance of the role even when one is live, deliver into it, and return its session name. to must be a role slug"`
}

type readMessagesInput struct {
	All bool `json:"all,omitempty" jsonschema:"also return messages you already read, not only the unread"`
}

type sessionStatusInput struct {
	To    string `json:"to" jsonschema:"session name from list_agents, or role, identity, or harness"`
	Lines int    `json:"lines,omitempty" jsonschema:"screen rows to return, default 30, at most 200"`
}

type clearSessionInput struct {
	To    string `json:"to" jsonschema:"session name from list_agents, or role, identity, or harness"`
	Force bool   `json:"force,omitempty" jsonschema:"clear even a busy session, or one holding a draft or undelivered messages"`
}

type closeSessionInput struct {
	To    string `json:"to" jsonschema:"session name from list_agents, or role, identity, or harness"`
	Force bool   `json:"force,omitempty" jsonschema:"close even while it holds a draft or undelivered messages"`
}

// textResult is a tool's answer as one text block. failed sets isError, which
// is how a refusal reaches the agent as something to read and act on.
func textResult(text string, failed bool) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: failed}, nil, nil
}

func jsonResult(value any) (*mcp.CallToolResult, any, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return textResult(string(encoded), false)
}

// newMCPServer builds the tool server with no transport, for COI-2534 to connect.
// A handler error is an isError result, only a *jsonrpc.Error a protocol error.
func newMCPServer(backend mcpBackend) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "aterm", Version: version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "ask_choice",
		Description: "Ask Kai a multiple-choice question on her aterm client and wait for the pick. " +
			"aterm stamps your seat on it. Returns the picked labels and any free text. A cancelled " +
			"or timed-out ask returns an error saying which. Give one question, or `questions` for up " +
			"to four, which Kai answers one card at a time and which return an answer each.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input askChoiceInput) (*mcp.CallToolResult, any, error) {
		// askFromArguments owns the one-or-many rules and their wording, and
		// takes the raw arguments, so the typed input is handed back as JSON.
		arguments, err := json.Marshal(input)
		if err != nil {
			return nil, nil, err
		}
		return textResult(askFromArguments(arguments, backend.ask))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_agents",
		Description: "List the live agent sessions on this host that send_message can reach. " +
			"Each carries its session name, role, identity, and harness. `self` is the caller.",
	}, func(context.Context, *mcp.CallToolRequest, listAgentsInput) (*mcp.CallToolResult, any, error) {
		views, err := backend.listAgents()
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(agentsDocument(views))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "send_message",
		Description: "Type a message into another live agent session. The recipient sees it prefixed " +
			"`[from <your role> <your identity>]`, which aterm stamps and you cannot change. " +
			"Address it by role slug, identity, harness, or session name from list_agents. Returns " +
			"queued, held (with the reason), delivered, or failed. A message still queued or held when " +
			"this returns is followed by a `[from aterm daemon]` receipt typed into your own session " +
			"when it lands or fails, so do not poll. `notify_when_idle` adds one `[from aterm daemon]` line " +
			"when the recipient next goes idle, is held at a permission or question card, or ends.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input sendMessageInput) (*mcp.CallToolResult, any, error) {
		state, err := backend.send(input.To, input.Message, sendOptions{Launch: input.Launch, Fresh: input.New,
			NotifyIdle: input.Idle, Wait: time.Duration(input.Wait) * time.Second})
		if err != nil {
			return nil, nil, err
		}
		return textResult(describeMessage(state), state.State == "failed")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "read_messages",
		Description: "Read the messages other sessions sent you with send_message or `aterm send`. Each carries " +
			"its id, `from` (`<role> <identity>`, stamped by aterm), `text` (the stamped line, which is also " +
			"typed into your session), and when it was received. Returns the unread ones, oldest first, and " +
			"marks them read, so a second call returns only what arrived since. `all` also returns the ones " +
			"already read. It holds the last 200 and empties when the aterm daemon restarts.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input readMessagesInput) (*mcp.CallToolResult, any, error) {
		messages, err := backend.inbox(input.All)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]any{"messages": messages})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "session_status",
		Description: "Read another live agent session without typing into it: its state (starting, prompt, " +
			"busy, idle), whether it sits on a permission or choice prompt and the prompt text, seconds since " +
			"it last wrote and since anyone typed, whether Kai has a draft, and the last rows of its screen. " +
			"`to` resolves as in send_message. Use it to tell a seat waiting on Kai from one still working.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input sessionStatusInput) (*mcp.CallToolResult, any, error) {
		status, err := backend.status(input.To, input.Lines)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(status)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "clear_session",
		Description: "Start another live agent session over by typing its harness's clear command, which " +
			"discards its context. Only the director role may call it. `to` resolves as in send_message. " +
			"It refuses your own session, a session on a permission or choice prompt, and unless force is " +
			"set one that is busy, holds Kai's unsent draft, or has undelivered messages.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input clearSessionInput) (*mcp.CallToolResult, any, error) {
		name, command, err := backend.clear(input.To, input.Force)
		if err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("typed %s into %s", command, name), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "close_session",
		Description: "End another live agent session and drop it from aterm, which closing its window does not. " +
			"`to` resolves as in send_message, and a target matching several sessions refuses. It will not " +
			"close your own session or one you run inside, and one holding Kai's unsent draft or undelivered " +
			"messages stays open unless force is set.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input closeSessionInput) (*mcp.CallToolResult, any, error) {
		name, code, err := backend.close(input.To, input.Force)
		if err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("closed %s (exit %d)", name, code), false)
	})

	return server
}

// serveMCP answers one stdio client until it closes its input. The token the
// tools use comes from the environment the harness started the process with.
func serveMCP(ctx context.Context, input io.Reader, output io.Writer) error {
	return newMCPServer(liveBackend).Run(ctx, &mcp.IOTransport{
		Reader:        io.NopCloser(input),
		Writer:        nopWriteCloser{output},
		MaxLineLength: maxFrame,
	})
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
