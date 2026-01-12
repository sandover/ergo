package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateResultSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"valid short", "Fix complete", false},
		{"valid with spaces", "  Fix complete  ", false}, // trimmed before storing
		{"max length", strings.Repeat("a", 120), false},
		{"too long", strings.Repeat("a", 121), true},
		{"newline not allowed", "Line1\nLine2", true},
		{"carriage return not allowed", "Line1\rLine2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResultSummary(tt.summary)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResultSummary(%q) error = %v, wantErr %v", tt.summary, err, tt.wantErr)
			}
		})
	}
}

func TestWriteArtifact(t *testing.T) {
	dir := t.TempDir()

	content := "Test artifact content"
	ref, err := writeArtifact(dir, content)
	if err != nil {
		t.Fatalf("writeArtifact failed: %v", err)
	}

	// Check ref format
	if !strings.HasPrefix(ref, "artifacts/") {
		t.Errorf("ref should start with 'artifacts/', got: %s", ref)
	}
	if !strings.HasSuffix(ref, ".txt") {
		t.Errorf("ref should end with '.txt', got: %s", ref)
	}

	// Check file exists and has correct content
	fullPath := filepath.Join(dir, ref)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed to read artifact: %v", err)
	}
	if string(data) != content {
		t.Errorf("artifact content = %q, want %q", string(data), content)
	}

	// Same content should produce same ref (content-addressed)
	ref2, err := writeArtifact(dir, content)
	if err != nil {
		t.Fatalf("second writeArtifact failed: %v", err)
	}
	if ref != ref2 {
		t.Errorf("same content should produce same ref: got %s and %s", ref, ref2)
	}

	// Different content should produce different ref
	ref3, err := writeArtifact(dir, "Different content")
	if err != nil {
		t.Fatalf("third writeArtifact failed: %v", err)
	}
	if ref == ref3 {
		t.Errorf("different content should produce different ref")
	}
}

func TestReadArtifact(t *testing.T) {
	dir := t.TempDir()

	content := "Test artifact content for reading"
	ref, err := writeArtifact(dir, content)
	if err != nil {
		t.Fatalf("writeArtifact failed: %v", err)
	}

	// Read it back
	readContent, err := readArtifact(dir, ref)
	if err != nil {
		t.Fatalf("readArtifact failed: %v", err)
	}
	if readContent != content {
		t.Errorf("readArtifact = %q, want %q", readContent, content)
	}

	// Reading non-existent artifact should fail
	_, err = readArtifact(dir, "artifacts/nonexistent.txt")
	if err == nil {
		t.Error("readArtifact should fail for non-existent file")
	}
}

func TestResolveResultValue(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantURL    bool
		wantErr    bool
	}{
		{"http URL", "http://example.com/result", true, false},
		{"https URL", "https://example.com/result", true, false},
		{"invalid format", "just text", false, true},
		{"file path", "/path/to/file", false, true},
		// Note: @- requires stdin, tested separately
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, urlRef, err := resolveResultValue(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveResultValue(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.wantURL {
				if urlRef != tt.value {
					t.Errorf("expected urlRef = %q, got %q", tt.value, urlRef)
				}
				if content != "" {
					t.Errorf("expected content = \"\", got %q", content)
				}
			}
		})
	}
}

func TestResultEventReplay(t *testing.T) {
	// Create events including a result
	now := time.Now().UTC()
	nowStr := formatTime(now)

	createEvent, _ := newEvent("new_task", now, NewTaskEvent{
		ID:        "T1",
		UUID:      "uuid-1",
		EpicID:    "",
		Body:      "Test task",
		State:     stateTodo,
		Worker:    "any",
		CreatedAt: nowStr,
	})
	result1Event, _ := newEvent("result", now.Add(time.Hour), ResultEvent{
		TaskID:  "T1",
		Summary: "First result",
		Ref:     "https://example.com/1",
		TS:      formatTime(now.Add(time.Hour)),
	})
	result2Event, _ := newEvent("result", now.Add(2*time.Hour), ResultEvent{
		TaskID:  "T1",
		Summary: "Second result",
		Ref:     "artifacts/abc123.txt",
		TS:      formatTime(now.Add(2 * time.Hour)),
	})

	events := []Event{createEvent, result1Event, result2Event}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	task := graph.Tasks["T1"]
	if task == nil {
		t.Fatal("task T1 not found")
	}

	// Results should be in reverse chronological order (newest first)
	if len(task.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(task.Results))
	}
	if task.Results[0].Summary != "Second result" {
		t.Errorf("first result should be newest: got %q", task.Results[0].Summary)
	}
	if task.Results[1].Summary != "First result" {
		t.Errorf("second result should be oldest: got %q", task.Results[1].Summary)
	}
}
