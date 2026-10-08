package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

// callThroughGateway is a harness reaching the daemon's gateway over HTTP.
func callThroughGateway(t *testing.T, addr, path, tool string) string {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-harness", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "http://" + addr + path}, nil)
	if err != nil {
		t.Fatalf("connect %s: %v", path, err)
	}
	defer cs.Close()
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool})
	if err != nil {
		t.Fatalf("call %s: %v", tool, err)
	}
	return resultText(result)
}

// A daemon killed under an opted-in seat leaves its gateway servers callable
// from the next one, and the spec's env never reaches a file (COI-2522).
func TestGatewaySurvivesADaemonRestart(t *testing.T) {
	rig := newHoldRig(t)
	rig.websocket = freeLoopbackAddr(t)
	t.Setenv(mcpAppsEnv, "1")
	rig.startDaemon()
	first := dialTest(t)
	first.spawn("eng-platform-test", "eng-platform", "Beetle-Ox", "echo TOKEN=$ATERM_SESSION_TOKEN; echo READY; exec cat")
	first.until("READY")
	token := first.token()
	firstPID := rig.daemon.Process.Pid

	const secret = "ATERM_TEST_SECRET=hunter2-restart-canary"
	spec := stdioSpec()
	spec.Env = []string{secret}
	reply, err := first.c.request(frame{Type: "gateway_add", Token: token, Server: "fake", Gateway: &spec})
	if err != nil || reply.Gateway == nil {
		t.Fatalf("gateway_add: %+v %v", reply, err)
	}
	path := reply.Gateway.Path
	if got := callThroughGateway(t, rig.websocket, path, "plain"); got != "ran plain" {
		t.Fatalf("before the restart the call returned %q", got)
	}

	rig.killDaemon(syscall.SIGKILL)
	rig.startDaemon()
	secondPID := rig.daemon.Process.Pid
	if secondPID == firstPID {
		t.Fatalf("the restart kept pid %d", firstPID)
	}
	t.Logf("daemon pid %d before, %d after", firstPID, secondPID)

	if got := callThroughGateway(t, rig.websocket, path, "plain"); got != "ran plain" {
		t.Fatalf("after the restart the call returned %q\n%s", got, rig.log.String())
	}
	// The opt-in came back too, so the adopted seat can register another server.
	again := dialTest(t)
	if reply, err := again.c.request(frame{Type: "gateway_add", Token: token, Server: "second", Gateway: &spec}); err != nil || reply.Type != "gateway_added" {
		t.Fatalf("gateway_add after the restart: %+v %v", reply, err)
	}

	_ = filepath.WalkDir(rig.dir, func(file string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type()&os.ModeSocket != 0 {
			return nil
		}
		if raw, err := os.ReadFile(file); err == nil && strings.Contains(string(raw), "hunter2-restart-canary") {
			t.Errorf("%s holds the gateway spec's env", file)
		}
		return nil
	})
}
