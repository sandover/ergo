// Tests for event-log file parsing and corruption tolerance.
// Focus: replay robustness (truncated final lines, useful error messages).
package ergo

import (
	"encoding/json"
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

func TestSelectEventsPath_RejectsMultipleCandidates(t *testing.T) {
	dir := t.TempDir()
	backlogPath := filepath.Join(dir, backlogFileName)
	plansPath := filepath.Join(dir, plansFileName)
	oldPath := filepath.Join(dir, oldEventsFileName)

	if err := os.WriteFile(backlogPath, []byte{}, 0644); err != nil {
		t.Fatalf("write backlog.jsonl: %v", err)
	}
	if err := os.WriteFile(plansPath, []byte{}, 0644); err != nil {
		t.Fatalf("write plans.jsonl: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte{}, 0644); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}

	_, err := selectEventsPath(dir)
	if err == nil {
		t.Fatal("expected conflicting backlog files to fail")
	}
	for _, path := range []string{backlogPath, plansPath, oldPath} {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("expected error to identify %q, got %q", path, err)
		}
	}
}

func TestSelectEventsPath_UsesPlansJsonl(t *testing.T) {
	dir := t.TempDir()
	plansPath := filepath.Join(dir, plansFileName)
	if err := os.WriteFile(plansPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	result, err := selectEventsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != plansPath {
		t.Fatalf("expected %q, got %q", plansPath, result)
	}
}

func TestSelectEventsPath_UsesEventsJsonl(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, oldEventsFileName)

	// Create only events.jsonl
	if err := os.WriteFile(oldPath, []byte{}, 0644); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}

	// Should use events.jsonl when plans.jsonl doesn't exist
	result, err := selectEventsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != oldPath {
		t.Fatalf("expected %q, got %q", oldPath, result)
	}
}

func TestSelectEventsPath_DefaultsToBacklogJsonl(t *testing.T) {
	dir := t.TempDir()
	backlogPath := filepath.Join(dir, backlogFileName)

	result, err := selectEventsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != backlogPath {
		t.Fatalf("expected %q, got %q", backlogPath, result)
	}
}

func TestLoadGraph_WorksWithBacklogJsonl(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, backlogFileName)
	now := time.Now().UTC()
	events := []Event{mustNewEvent("new_task", now, NewTaskEvent{
		ID: "T0", UUID: "uuid-0", State: stateTodo, Title: "Task 0", CreatedAt: formatTime(now),
	})}
	if err := appendEvents(path, events); err != nil {
		t.Fatal(err)
	}
	graph, err := loadGraph(dir)
	if err != nil || graph.Tasks["T0"] == nil {
		t.Fatalf("graph=%v err=%v", graph, err)
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

func TestAppendTransactionPreservesLegacyEventsAndReplaysBatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, plansFileName)
	now := time.Now().UTC()

	initial := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T1",
			UUID:      "uuid-1",
			State:     stateTodo,
			Title:     "Task 1",
			Body:      "Task 1 body",
			CreatedAt: formatTime(now),
		}),
	}
	appended := []Event{
		mustNewEvent("new_task", now.Add(time.Second), NewTaskEvent{
			ID:        "T2",
			UUID:      "uuid-2",
			State:     stateTodo,
			Title:     "Task 2",
			Body:      "Task 2 body",
			CreatedAt: formatTime(now.Add(time.Second)),
		}),
	}

	if err := writeEventsFile(path, initial); err != nil {
		t.Fatalf("writeEventsFile initial: %v", err)
	}
	if err := appendEvents(path, appended); err != nil {
		t.Fatalf("appendEvents: %v", err)
	}

	events, err := readEvents(path)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents: %v", err)
	}
	if graph.Tasks["T1"] == nil || graph.Tasks["T2"] == nil {
		t.Fatalf("expected T1 and T2 after replay, got tasks: %v", len(graph.Tasks))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], `"type":"transaction"`) {
		t.Fatalf("log does not contain legacy event followed by one transaction: %s", data)
	}
}

func TestAppendTransactionRepairsTruncatedTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, plansFileName)
	legacy := `{"type":"noop","ts":"t","data":{}}` + "\n"
	if err := os.WriteFile(path, []byte(legacy+`{"type":"transaction","version":1,"events":[`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := appendEvents(path, []Event{{Type: "noop", TS: "later", Data: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"events":[{"type":"transaction"`) {
		t.Fatalf("truncated bytes were concatenated with the new transaction: %s", data)
	}
	events, err := readEvents(path)
	if err != nil || len(events) != 2 {
		t.Fatalf("events=%d err=%v log=%s", len(events), err, data)
	}
}

func TestAppendTransactionRejectsTooLongRecordWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), plansFileName)
	event := Event{Type: "body", Data: json.RawMessage(`{"body":"` + strings.Repeat("x", maxLogRecordBytes) + `"}`)}
	err := appendEvents(path, []Event{event})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want size-limit error", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("oversized transaction changed the log: %v", statErr)
	}
}
