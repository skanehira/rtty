package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestK6Lifecycle(t *testing.T) {
	if _, err := exec.LookPath("k6"); err != nil {
		t.Skip("k6 not installed")
	}
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found in PATH")
	}

	sess := newSession()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	allowOrigins := []string{parsed.Host}
	mux.Handle("/ws", run(sess, allowOrigins, []string{"cat"}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	origin := "http://" + parsed.Host

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	scriptPath := filepath.Join(cwd, "..", "test", "k6", "ws_lifecycle.js")
	absScriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("failed to resolve script path: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "k6", "run", absScriptPath)
	cmd.Env = append(os.Environ(),
		"RTTY_WS_URL="+wsURL,
		"RTTY_ORIGIN="+origin,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("k6 lifecycle test failed: %v\n%s", err, string(output))
	}
}
