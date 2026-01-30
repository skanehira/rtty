package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func startTestServer(t *testing.T, sess *session, commandArgs []string) (*httptest.Server, string, string) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	allowOrigins := []string{parsed.Host}
	mux.Handle("/ws", run(sess, allowOrigins, commandArgs))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	origin := "http://" + parsed.Host
	return srv, wsURL, origin
}

func dialWS(t *testing.T, wsURL string, origin string) (*websocket.Conn, *http.Response) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Origin": {origin},
		},
	}
	conn, resp, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	return conn, resp
}

func closeWS(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Logf("failed to close websocket: %v", err)
	}
}

func sendResize(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := wsjson.Write(ctx, conn, Message{Event: EventResize, Data: map[string]any{"rows": 24.0, "cols": 80.0}})
	if err != nil {
		t.Fatalf("failed to send resize: %v", err)
	}
}

func TestRunHandlerFirstMessageMustBeResize(t *testing.T) {
	sess := newSession()
	srv, wsURL, origin := startTestServer(t, sess, []string{"cat"})
	defer srv.Close()

	conn, _ := dialWS(t, wsURL, origin)
	t.Cleanup(func() { closeWS(t, conn) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := wsjson.Write(ctx, conn, Message{Event: EventSendKey, Data: "hello"})
	if err != nil {
		t.Fatalf("failed to send sendKey: %v", err)
	}

	ctxRead, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	_, _, err = conn.Read(ctxRead)
	if err == nil {
		t.Fatalf("expected connection to close after invalid first message")
	}
}

func TestRunHandlerEchoAndHistory(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found in PATH")
	}

	sess := newSession()
	srv, wsURL, origin := startTestServer(t, sess, []string{"cat"})
	defer srv.Close()

	connA, _ := dialWS(t, wsURL, origin)
	t.Cleanup(func() { closeWS(t, connA) })
	sendResize(t, connA)

	token := fmt.Sprintf("token-%d", time.Now().UnixNano())
	ctxWrite, cancelWrite := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWrite()
	err := wsjson.Write(ctxWrite, connA, Message{Event: EventSendKey, Data: token + "\n"})
	if err != nil {
		t.Fatalf("failed to send key: %v", err)
	}

	ctxRead, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	_, data, err := connA.Read(ctxRead)
	if err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}
	if !strings.Contains(string(data), token) {
		t.Fatalf("expected echo to include token, got: %s", string(data))
	}

	connB, _ := dialWS(t, wsURL, origin)
	t.Cleanup(func() { closeWS(t, connB) })
	sendResize(t, connB)

	ctxReadB, cancelReadB := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReadB()
	_, history, err := connB.Read(ctxReadB)
	if err != nil {
		t.Fatalf("failed to read history: %v", err)
	}
	if !strings.Contains(string(history), token) {
		t.Fatalf("expected history to include token, got: %s", string(history))
	}
}

func TestRunHandlerBroadcastToMultipleClients(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found in PATH")
	}

	sess := newSession()
	srv, wsURL, origin := startTestServer(t, sess, []string{"cat"})
	defer srv.Close()

	connA, _ := dialWS(t, wsURL, origin)
	t.Cleanup(func() { closeWS(t, connA) })
	sendResize(t, connA)

	connB, _ := dialWS(t, wsURL, origin)
	t.Cleanup(func() { closeWS(t, connB) })
	sendResize(t, connB)

	token := fmt.Sprintf("broadcast-%d", time.Now().UnixNano())
	ctxWrite, cancelWrite := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWrite()
	err := wsjson.Write(ctxWrite, connA, Message{Event: EventSendKey, Data: token + "\n"})
	if err != nil {
		t.Fatalf("failed to send key: %v", err)
	}

	ctxRead, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	_, data, err := connB.Read(ctxRead)
	if err != nil {
		t.Fatalf("failed to read broadcast: %v", err)
	}
	if !strings.Contains(string(data), token) {
		t.Fatalf("expected broadcast to include token, got: %s", string(data))
	}
}

func TestRunHandlerOriginRejected(t *testing.T) {
	sess := newSession()
	srv, wsURL, _ := startTestServer(t, sess, []string{"cat"})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	opts := &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Origin": {"http://bad.local:8080"},
		},
	}
	_, resp, err := websocket.Dial(ctx, wsURL, opts)
	if err == nil {
		t.Fatalf("expected dial to fail for disallowed origin")
		return
	}
	if resp == nil {
		t.Fatalf("expected response for disallowed origin")
		return
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
