// Purpose: Prove deterministic snapshot encoding, integrity, and mixed-log replay.
package ergo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotEncodingIsDeterministic(t *testing.T) {
	now := time.Date(2026, 2, 3, 4, 5, 6, 7, time.UTC)
	makeGraph := func(reverse bool) *Graph {
		graph := newGraph()
		first := &Task{ID: "AAAAAA", UUID: "uuid-a", State: stateDone, Title: "A", CreatedAt: now, UpdatedAt: now}
		second := &Task{ID: "BBBBBB", UUID: "uuid-b", State: stateTodo, Title: "B", CreatedAt: now, UpdatedAt: now}
		if reverse {
			graph.Tasks[second.ID], graph.Tasks[first.ID] = second, first
		} else {
			graph.Tasks[first.ID], graph.Tasks[second.ID] = first, second
		}
		graph.Deps[second.ID] = map[string]struct{}{first.ID: {}}
		graph.rebuildIndexes()
		return graph
	}
	first, _, err := marshalSnapshot(makeGraph(false))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := marshalSnapshot(makeGraph(true))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("semantically identical graphs produced different snapshots:\n%s\n%s", first, second)
	}
	again, _, err := marshalSnapshot(roundTripSnapshot(t, makeGraph(false)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, again) {
		t.Fatalf("repeated compaction was not byte-identical:\n%s\n%s", first, again)
	}
}

func TestSnapshotIntegrityRejectsChangedData(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	graph.Tasks["AAAAAA"] = &Task{ID: "AAAAAA", UUID: "uuid", State: stateTodo, Title: "Original", CreatedAt: now, UpdatedAt: now}
	data, _, err := marshalSnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"title":"Original"`), []byte(`"title":"Changed!"`), 1)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, backlogFileName), data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadGraph(dir); err == nil || !strings.Contains(err.Error(), "snapshot integrity mismatch") {
		t.Fatalf("integrity error = %v", err)
	}
}

func TestSnapshotTruncatedDataRecordFailsClosed(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	graph.Tasks["AAAAAA"] = &Task{ID: "AAAAAA", UUID: "uuid", State: stateTodo, Title: "Task", CreatedAt: now, UpdatedAt: now}
	data, _, err := marshalSnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	data = data[:len(data)-10]
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, backlogFileName), data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadGraph(dir); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("truncated snapshot error = %v", err)
	}
}

func TestSnapshotLoadsLaterTransaction(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	graph.Tasks["AAAAAA"] = &Task{ID: "AAAAAA", UUID: "uuid", State: stateTodo, Title: "Before", CreatedAt: now, UpdatedAt: now}
	data, _, err := marshalSnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, backlogFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	update := mustNewEvent("title", now.Add(time.Minute), TitleUpdateEvent{
		ID: "AAAAAA", Title: "After", TS: formatTime(now.Add(time.Minute)),
	})
	if err := appendEvents(path, []Event{update}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tasks["AAAAAA"].Title != "After" {
		t.Fatalf("title = %q, want After", loaded.Tasks["AAAAAA"].Title)
	}
}

func TestSnapshotCompactionGarbageCollectsTombstones(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	graph.Tasks["LIVE00"] = &Task{ID: "LIVE00", UUID: "uuid", State: stateTodo, Title: "Live", CreatedAt: now, UpdatedAt: now}
	graph.Tombstones["GONE00"] = TombstoneInfo{AgentID: "agent", At: now}
	data, _, err := marshalSnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("GONE00")) || bytes.Contains(data, []byte("tombstone")) {
		t.Fatalf("snapshot retained garbage-collected tombstone: %s", data)
	}
	replayed := roundTripSnapshot(t, graph)
	if len(replayed.Tombstones) != 0 || replayed.Tasks["LIVE00"] == nil {
		t.Fatalf("unexpected state after tombstone GC: %#v", replayed)
	}
}

func TestSnapshotDerivesNonemptyEpicIdentityFromChildren(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	graph.Tasks["EPIC00"] = &Task{ID: "EPIC00", UUID: "epic", State: stateTodo, Title: "Epic", CreatedAt: now, UpdatedAt: now}
	graph.Tasks["CHILD0"] = &Task{ID: "CHILD0", UUID: "child", EpicID: "EPIC00", State: stateTodo, Title: "Child", CreatedAt: now, UpdatedAt: now}
	graph.legacyEmptyEpics["EPIC00"] = struct{}{}
	graph.rebuildIndexes()

	data, _, err := marshalSnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"id":"EPIC00","uuid":"epic","epic_id":"","explicit_epic":true`)) {
		t.Fatalf("nonempty epic retained a redundant explicit marker: %s", data)
	}
	replayed := roundTripSnapshot(t, graph)
	if !replayed.IsEpic("EPIC00") || len(replayed.Children("EPIC00")) != 1 {
		t.Fatal("nonempty epic identity was not derived from its child")
	}
}

func TestSnapshotExcludesJournalOwnedAccumulatedItems(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	task := &Task{ID: "AAAAAA", UUID: "uuid", State: stateTodo, Title: "Task", CreatedAt: now, UpdatedAt: now}
	for i := 0; i < 100; i++ {
		task.Messages = append(task.Messages, Message{Kind: "release", Text: strings.Repeat("x", 1024), CreatedAt: now})
		task.Results = append(task.Results, Result{Summary: "result", Path: "result", Sha256AtAttach: "hash", CreatedAt: now})
	}
	graph.Tasks[task.ID] = task
	data, stats, err := marshalSnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Records != 2 {
		t.Fatalf("records = %d, want 2", stats.Records)
	}
	for lineNo, line := range bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) {
		if len(line) > maxLogRecordBytes {
			t.Fatalf("line %d exceeds record limit", lineNo+1)
		}
	}
}

func TestSnapshotAtomicReplacementFailureLeavesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, backlogFileName)
	original := []byte("original\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0755); err != nil {
		t.Fatal(err)
	}
	if err := replaceLogAtomically(path, []byte("replacement\n")); err == nil {
		t.Fatal("expected replacement failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("failed replacement changed original: %q", got)
	}
}
