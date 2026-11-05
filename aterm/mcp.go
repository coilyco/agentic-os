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
		Usage: "serve list_agents, send_message, read_messages, session_status, clear_session, close_session, stage_command, ask_choice and send_user_question over MCP stdio",
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
	stage      func(command, note string) (stageResult, error)
	// artifactAdd registers a page, and artifact changes or removes one by request.
	artifactAdd func(address, title, source string, waiting, standing bool) (frame, error)
	artifact    func(frame) (frame, error)
	ask         func(choiceAsk) (choiceAnswer, error)
	inbox       func(all bool) ([]inboxMessage, error)
}

var liveBackend = mcpBackend{
	listAgents:  listAgents,
	send:        sendMessage,
	status:      sessionStatusOf,
	clear:       clearTarget,
	close:       closeSession,
	stage:       stageCommand,
	artifactAdd: addArtifact,
	artifact:    artifactCall,
	ask:         askChoice,
	inbox:       readInbox,
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

type addArtifactInput struct {
	URL      string `json:"url" jsonschema:"exactly https://claude.ai/artifact/<id>"`
	Title    string `json:"title" jsonschema:"the name shown above the page, one line"`
	Source   string `json:"source" jsonschema:"the artifact's HTML source file, under your working directory or scratchpad, at most 2 MiB"`
	Waiting  bool   `json:"waiting,omitempty" jsonschema:"mark it as waiting on Kai"`
	Standing bool   `json:"standing,omitempty" jsonschema:"keep it after your session ends and across a daemon restart"`
}

type artifactRefInput struct {
	Artifact string `json:"artifact" jsonschema:"the artifact's address or its id"`
}

type artifactWaitingInput struct {
	artifactRefInput
	Waiting bool `json:"waiting" jsonschema:"true while the page waits on Kai, false once it does not"`
}

type stageCommandInput struct {
	Command string `json:"command" jsonschema:"one shell command line, with no newline, carriage return or other control character"`
	Note    string `json:"note,omitempty" jsonschema:"one line shown beside the terminal for Kai, never typed into the shell"`
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

// askHandler runs ask_choice and send_user_question, which differ only in who they ask.
// askFromArguments owns the one-or-many rules and takes the raw arguments as JSON.
func askHandler(ask func(choiceAsk) (choiceAnswer, error)) func(context.Context, *mcp.CallToolRequest, askChoiceInput) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, input askChoiceInput) (*mcp.CallToolResult, any, error) {
		arguments, err := json.Marshal(input)
		if err != nil {
			return nil, nil, err
		}
		return textResult(askFromArguments(arguments, ask))
	}
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
	}, askHandler(backend.ask))

	mcp.AddTool(server, &mcp.Tool{
		Name: "send_user_question",
		Description: "Put a multiple-choice question for Kai on the director seat's card in her aterm " +
			"client, and wait for the pick. Use it as a worker seat that needs Kai, instead of messaging " +
			"the director. aterm stamps your seat on it, your own row stays marked as asking, and the " +
			"director gets a line with her answer. With no director up the card shows on your own seat. " +
			"Shaped like ask_choice, plus `shown_on`, the director seat the card appeared on, absent " +
			"when it showed on yours.",
	}, askHandler(func(ask choiceAsk) (choiceAnswer, error) {
		ask.Relay = true
		return backend.ask(ask)
	}))

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
			"this returns earns a receipt in your inbox when it lands or fails, which read_messages " +
			"returns. It is never typed into your session. `notify_when_idle` adds one `[from aterm daemon]` " +
			"line typed into your session when the recipient next goes idle, is held at a permission " +
			"or question card, or ends.",
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
		Description: "Read the messages other sessions sent you with send_message or `aterm send`, and the " +
			"delivery receipts from `aterm daemon` for yours. Each carries its id, `from` (`<role> <identity>`, " +
			"stamped by aterm), `text` (the stamped line, typed into your session too unless it is a receipt), " +
			"and when it was received. Returns the unread ones, oldest first, and " +
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
			"set one that is busy, holds Kai's unsent draft, or has undelivered messages. Like close_session it " +
			"refuses a name you did not list or that was reassigned, and takes `name@ref`.",
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
			"messages stays open unless force is set. Names are reused, so list_agents first: a name you did not " +
			"list, or one reassigned since, is refused. `name@ref` with the ref from list_agents is exact.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input closeSessionInput) (*mcp.CallToolResult, any, error) {
		name, code, err := backend.close(input.To, input.Force)
		if err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("closed %s (exit %d)", name, code), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "stage_command",
		Description: "Type a command into the plain terminal beside your session, on Kai's host shell, " +
			"without running it. Kai reads the line and presses Enter. The terminal opens when none exists. " +
			"It is never submitted and no argument can submit it: a command with a newline, carriage " +
			"return or any other control character is refused, and the terminal is always your own, " +
			"found from your session. `note` is one line the client shows beside it. Returns staged, " +
			"refused with the reason, or no client attached. Use it to hand Kai a command to run " +
			"instead of pasting it into chat.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input stageCommandInput) (*mcp.CallToolResult, any, error) {
		result, err := backend.stage(input.Command, input.Note)
		if err != nil {
			return nil, nil, err
		}
		return textResult(describeStage(result), result.State != stageStaged)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "add_artifact",
		Description: "Show a claude.ai artifact's source in Kai's aterm client, labeled with your session, so she can " +
			"read it there. claude.ai refuses to be framed, so the client shows your own source file and links out to " +
			"the page. The daemon reads `source` itself, only from your working directory or scratchpad, at most 2 MiB, " +
			"and `url` must be exactly https://claude.ai/artifact/<id>. Adding the same url again replaces the page. " +
			"Returns the artifact id and how many clients show it.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input addArtifactInput) (*mcp.CallToolResult, any, error) {
		reply, err := backend.artifactAdd(input.URL, input.Title, input.Source, input.Waiting, input.Standing)
		if err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("added %s, shown to %d clients", reply.ArtifactID, reply.Delivered), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "set_artifact_waiting",
		Description: "Mark an artifact you added as waiting on Kai, or not. The client shows a waiting page the way it " +
			"shows a waiting seat.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input artifactWaitingInput) (*mcp.CallToolResult, any, error) {
		waiting := input.Waiting
		if _, err := backend.artifact(frame{Type: "artifact_waiting", ArtifactID: input.Artifact, Waiting: &waiting}); err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("waiting %t", waiting), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_artifact",
		Description: "Close an artifact you added in Kai's client. A standing page whose seat has ended may be removed by any seat.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input artifactRefInput) (*mcp.CallToolResult, any, error) {
		if _, err := backend.artifact(frame{Type: "artifact_remove", ArtifactID: input.Artifact}); err != nil {
			return nil, nil, err
		}
		return textResult("removed", false)
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
