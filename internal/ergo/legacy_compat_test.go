// Purpose: Prove v2 reads, preserves, and lazily normalizes legacy task states.
// Exports: none.
// Role: Compatibility coverage for old event logs and lossless compaction.
// Invariants: opening or compacting a plan never guesses a new lifecycle state.
// Invariants: only an explicit lifecycle postcondition clears legacy ownership.
package ergo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyReplaySupportsEventsFileAndDerivedViews(t *testing.T) {
	repoDir := t.TempDir()
	ergoDir := filepath.Join(repoDir, ".ergo")
	mustMkdirAll(t, ergoDir)
	now := time.Now().UTC()
	events := []Event{
		mustLegacyEvent(t, "new_epic", now, NewTaskEvent{ID: "EPIC01", UUID: "epic", State: stateTodo, Title: "Legacy container", CreatedAt: formatTime(now)}),
		mustLegacyEvent(t, "new_task", now, NewTaskEvent{ID: "ERR001", UUID: "error", EpicID: "EPIC01", State: stateTodo, Title: "Legacy error", CreatedAt: formatTime(now)}),
		mustLegacyEvent(t, "claim", now, ClaimEvent{ID: "ERR001", AgentID: "old-agent", TS: formatTime(now)}),
		mustLegacyEvent(t, "state", now, StateEvent{ID: "ERR001", NewState: stateError, TS: formatTime(now)}),
		mustLegacyEvent(t, "new_task", now, NewTaskEvent{ID: "RDY001", UUID: "ready", EpicID: "EPIC01", State: stateTodo, Title: "Ready", CreatedAt: formatTime(now)}),
	}
	if err := writeEventsFile(filepath.Join(ergoDir, oldEventsFileName), events); err != nil {
		t.Fatal(err)
	}

	graph, err := loadGraph(ergoDir)
	if err != nil {
		t.Fatal(err)
	}
	if !isContainer(graph.Tasks["EPIC01"], graph) {
		t.Fatal("legacy epic was not replayed as a container")
	}
	errorTask := graph.Tasks["ERR001"]
	if errorTask.State != stateError || errorTask.ClaimedBy != "old-agent" {
		t.Fatalf("legacy error changed during replay: state=%s claim=%q", errorTask.State, errorTask.ClaimedBy)
	}
	ready := readyTasks(graph)
	if len(ready) != 1 || ready[0].ID != "RDY001" {
		t.Fatalf("ready tasks = %v, want RDY001", taskIDsForTest(ready))
	}
}

func TestLegacyCompactPreservesErrorAndClaimedBlocked(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustLegacyEvent(t, "new_task", now, NewTaskEvent{ID: "ERR001", UUID: "error", State: stateError, Title: "Error", CreatedAt: formatTime(now)}),
		mustLegacyEvent(t, "claim", now, ClaimEvent{ID: "ERR001", AgentID: "old-error", TS: formatTime(now)}),
		mustLegacyEvent(t, "new_task", now, NewTaskEvent{ID: "BLK001", UUID: "blocked", State: stateBlocked, Title: "Blocked", CreatedAt: formatTime(now)}),
		mustLegacyEvent(t, "claim", now, ClaimEvent{ID: "BLK001", AgentID: "old-block", TS: formatTime(now)}),
	}
	before, err := replayEvents(events)
	if err != nil {
		t.Fatal(err)
	}
	compacted, err := compactEvents(before)
	if err != nil {
		t.Fatal(err)
	}
	after, err := replayEvents(compacted)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ERR001", "BLK001"} {
		if after.Tasks[id].State != before.Tasks[id].State || after.Tasks[id].ClaimedBy != before.Tasks[id].ClaimedBy {
			t.Fatalf("%s changed across compaction: before=%s/%q after=%s/%q", id,
				before.Tasks[id].State, before.Tasks[id].ClaimedBy, after.Tasks[id].State, after.Tasks[id].ClaimedBy)
		}
	}
}

func TestLegacyLifecycleNormalizationIsExplicit(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name      string
		task      *Task
		target    string
		wantTypes []string
	}{
		{"release error", &Task{ID: "ERR001", State: stateError, ClaimedBy: "old-agent"}, stateTodo, []string{"unclaim", "state"}},
		{"block claimed blocked", &Task{ID: "BLK001", State: stateBlocked, ClaimedBy: "old-agent"}, stateBlocked, []string{"unclaim"}},
		{"done error", &Task{ID: "ERR002", State: stateError, ClaimedBy: "old-agent"}, stateDone, []string{"unclaim", "state"}},
		{"cancel error", &Task{ID: "ERR003", State: stateError, ClaimedBy: "old-agent"}, stateCanceled, []string{"unclaim", "state"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events, _, err := buildMutationEvents(test.task.ID, test.task, taskMutation{State: test.target, StateSet: true}, "", now)
			if err != nil {
				t.Fatal(err)
			}
			if got := eventTypes(events); !equalStrings(got, test.wantTypes) {
				t.Fatalf("event types = %v, want %v", got, test.wantTypes)
			}
		})
	}
}

func TestLegacyWriterAppendRemainsReplayable(t *testing.T) {
	now := time.Now().UTC()
	initial := []Event{
		mustLegacyEvent(t, "new_task", now, NewTaskEvent{ID: "TASK01", UUID: "task", State: stateTodo, Title: "Task", CreatedAt: formatTime(now)}),
	}
	graph, err := replayEvents(initial)
	if err != nil {
		t.Fatal(err)
	}
	compacted, err := compactEvents(graph)
	if err != nil {
		t.Fatal(err)
	}
	appended := append(compacted,
		mustLegacyEvent(t, "claim", now, ClaimEvent{ID: "TASK01", AgentID: "old-agent", TS: formatTime(now)}),
		mustLegacyEvent(t, "state", now, StateEvent{ID: "TASK01", NewState: stateError, TS: formatTime(now)}),
	)
	read, err := replayEvents(appended)
	if err != nil {
		t.Fatal(err)
	}
	if read.Tasks["TASK01"].State != stateError || read.Tasks["TASK01"].ClaimedBy != "old-agent" {
		t.Fatal("events appended by an older writer were not preserved")
	}
}

func mustLegacyEvent(t *testing.T, kind string, now time.Time, payload any) Event {
	t.Helper()
	event, err := newEvent(kind, now, payload)
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func taskIDsForTest(tasks []*Task) []string {
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	return ids
}
