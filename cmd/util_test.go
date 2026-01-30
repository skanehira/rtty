package cmd

import "testing"

func TestWindowSizeOK(t *testing.T) {
	rows, cols, err := windowSize(map[string]any{"rows": 24.0, "cols": 80.0})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rows != 24 || cols != 80 {
		t.Fatalf("unexpected size: rows=%d cols=%d", rows, cols)
	}
}

func TestWindowSizeInvalidMessageType(t *testing.T) {
	_, _, err := windowSize("bad")
	if err == nil {
		t.Fatalf("expected error for invalid message type")
	}
}

func TestWindowSizeInvalidRows(t *testing.T) {
	_, _, err := windowSize(map[string]any{"rows": "bad", "cols": 80.0})
	if err == nil {
		t.Fatalf("expected error for invalid rows")
	}
}

func TestWindowSizeInvalidCols(t *testing.T) {
	_, _, err := windowSize(map[string]any{"rows": 24.0, "cols": "bad"})
	if err == nil {
		t.Fatalf("expected error for invalid cols")
	}
}
