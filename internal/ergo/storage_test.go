// Tests for event-log file parsing and corruption tolerance.
// Focus: replay robustness (truncated final lines, useful error messages).
package ergo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEvents_AllowsValidFinalLineWithoutNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// Valid single JSON object, no trailing newline.
	if err := os.WriteFile(path, []byte(`{"type":"noop","ts":"t","data":{}}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	events, err := readEvents(path)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestReadEvents_ToleratesTruncatedFinalLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// Second line is truncated/invalid and lacks a trailing newline.
	content := `{"type":"noop","ts":"t","data":{}}` + "\n" + `{"type":"noop"`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	events, err := readEvents(path)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestReadEvents_InvalidJSONIncludesLineNumber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	content := `{"type":"noop","ts":"t","data":{}}` + "\n" + `not-json` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := readEvents(path)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, ":2:") {
		t.Fatalf("expected line number in error, got: %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "invalid json") {
		t.Fatalf("expected invalid JSON hint, got: %q", msg)
	}
}

func TestReadEvents_ConflictMarkersHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	content := `<<<<<<< HEAD` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := readEvents(path)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "conflict") {
		t.Fatalf("expected conflict hint, got: %q", msg)
	}
}
