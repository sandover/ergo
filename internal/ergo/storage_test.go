// Tests for event-log file parsing and corruption tolerance.
// Focus: replay robustness (truncated final lines, useful error messages).
package ergo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestReadEvents_TombstoneRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	now := time.Now().UTC()

	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T1",
			UUID:      "uuid-1",
			State:     stateTodo,
			Title:     "Task 1",
			Body:      "Task 1",
			CreatedAt: formatTime(now),
		}),
		mustNewEvent("tombstone", now.Add(time.Second), TombstoneEvent{
			ID:      "T1",
			AgentID: "agent-1",
			TS:      formatTime(now.Add(time.Second)),
		}),
	}

	if err := appendEvents(path, events); err != nil {
		t.Fatalf("appendEvents: %v", err)
	}
	read, err := readEvents(path)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if _, err := replayEvents(read); err != nil {
		t.Fatalf("replayEvents: %v", err)
	}
}

func TestGetEventsPath_PrefersPlansDotJsonl(t *testing.T) {
	dir := t.TempDir()
	plansPath := filepath.Join(dir, plansFileName)
	oldPath := filepath.Join(dir, oldEventsFileName)

	// Create both files
	if err := os.WriteFile(plansPath, []byte{}, 0644); err != nil {
		t.Fatalf("write plans.jsonl: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte{}, 0644); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}

	// Should prefer plans.jsonl when both exist
	result := getEventsPath(dir)
	if result != plansPath {
		t.Fatalf("expected %q, got %q", plansPath, result)
	}
}

func TestGetEventsPath_FallbackToEventsJsonl(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, oldEventsFileName)

	// Create only events.jsonl
	if err := os.WriteFile(oldPath, []byte{}, 0644); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}

	// Should use events.jsonl when plans.jsonl doesn't exist
	result := getEventsPath(dir)
	if result != oldPath {
		t.Fatalf("expected %q, got %q", oldPath, result)
	}
}

func TestGetEventsPath_DefaultToPlansJsonl(t *testing.T) {
	dir := t.TempDir()
	plansPath := filepath.Join(dir, plansFileName)

	// Neither file exists
	result := getEventsPath(dir)
	if result != plansPath {
		t.Fatalf("expected %q, got %q", plansPath, result)
	}
}

func TestLoadGraph_WorksWithEventsJsonl(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, oldEventsFileName)
	now := time.Now().UTC()

	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T1",
			UUID:      "uuid-1",
			State:     stateTodo,
			Title:     "Task 1",
			Body:      "Test task",
			CreatedAt: formatTime(now),
		}),
	}

	if err := appendEvents(oldPath, events); err != nil {
		t.Fatalf("appendEvents: %v", err)
	}

	graph, err := loadGraph(dir)
	if err != nil {
		t.Fatalf("loadGraph: %v", err)
	}

	if len(graph.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(graph.Tasks))
	}
	if graph.Tasks["T1"] == nil {
		t.Fatal("expected task T1 to exist")
	}
}

func TestLoadGraph_WorksWithPlansJsonl(t *testing.T) {
	dir := t.TempDir()
	plansPath := filepath.Join(dir, plansFileName)
	now := time.Now().UTC()

	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T2",
			UUID:      "uuid-2",
			State:     stateTodo,
			Title:     "Task 2",
			Body:      "Test task",
			CreatedAt: formatTime(now),
		}),
	}

	if err := appendEvents(plansPath, events); err != nil {
		t.Fatalf("appendEvents: %v", err)
	}

	graph, err := loadGraph(dir)
	if err != nil {
		t.Fatalf("loadGraph: %v", err)
	}

	if len(graph.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(graph.Tasks))
	}
	if graph.Tasks["T2"] == nil {
		t.Fatal("expected task T2 to exist")
	}
}
