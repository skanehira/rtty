package cmd

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestCheckOriginOK(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Origin", "http://allowed.local:8080")
	if err := checkOrigin([]string{"allowed.local:8080"}, req); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckOriginMissingOrigin(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if err := checkOrigin([]string{"allowed.local:8080"}, req); err == nil {
		t.Fatalf("expected error for missing origin")
	}
}

func TestCheckOriginInvalidOrigin(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Origin", "not-a-url")
	if err := checkOrigin([]string{"allowed.local:8080"}, req); err == nil {
		t.Fatalf("expected error for invalid origin")
	}
}

func TestSessionAppendHistoryLimits(t *testing.T) {
	s := newSession()
	s.historyLimit = 2
	s.historyBufCap = 32
	s.appendHistory([]byte("one\n"))
	s.appendHistory([]byte("two\n"))
	s.appendHistory([]byte("three\n"))
	if len(s.history) != 2 {
		t.Fatalf("expected history length 2, got %d", len(s.history))
	}
	if strings.TrimSpace(s.history[0]) != "two" || strings.TrimSpace(s.history[1]) != "three" {
		t.Fatalf("unexpected history contents: %v", s.history)
	}
}

func TestSessionHandleMessageClose(t *testing.T) {
	s := newSession()
	if err := s.handleMessage(Message{Event: EventClose}); err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestSessionHandleMessageSendKeyWritesPTY(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("failed to close read pipe: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("failed to close write pipe: %v", err)
		}
	})

	s := newSession()
	s.ptmx = w

	msg := Message{Event: EventSendKey, Data: "hello"}
	if err := s.handleMessage(msg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	buf := make([]byte, 5)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("unexpected pipe data: %q", string(buf))
	}
}

func TestSessionHandleMessageSendKeyInvalidData(t *testing.T) {
	s := newSession()
	if err := s.handleMessage(Message{Event: EventSendKey, Data: 42}); err == nil {
		t.Fatalf("expected error for invalid data")
	}
}

func TestSessionHandleMessageDefaultEventString(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("failed to close read pipe: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("failed to close write pipe: %v", err)
		}
	})

	s := newSession()
	s.ptmx = w

	if err := s.handleMessage(Message{Event: "unknown", Data: "hi"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	buf := make([]byte, 2)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	if string(buf) != "hi" {
		t.Fatalf("unexpected pipe data: %q", string(buf))
	}
}

func TestSessionHandleMessageResizeNoPTY(t *testing.T) {
	s := newSession()
	msg := Message{Event: EventResize, Data: map[string]any{"rows": 24.0, "cols": 80.0}}
	if err := s.handleMessage(msg); err == nil {
		t.Fatalf("expected error for resize with nil pty")
	}
}
