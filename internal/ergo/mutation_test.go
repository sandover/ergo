// Purpose: Verify atomic mutation event batches and validation rollback.
// Exports: none.
// Role: Unit and storage-level coverage for the shared v2 write path.
// Invariants: true no-ops append nothing and invalid batches append nothing.
// Invariants: lifecycle postconditions clear legacy claims when required.
package ergo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMutationSuppressesTrueNoop(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateDone, Title: "Task", Body: "Body"}
	events, fields, err := buildMutationEvents(task.ID, task, taskMutation{State: stateDone, StateSet: true}, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 || len(fields) != 0 {
		t.Fatalf("true no-op produced events=%d fields=%v", len(events), fields)
	}
}

func TestMutationBuildsMixedAtomicBatch(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateDoing, ClaimedBy: "agent-1", Body: "old"}
	mutation := taskMutation{State: stateBlocked, StateSet: true, Body: "new", BodySet: true}
	events, fields, err := buildMutationEvents(task.ID, task, mutation, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(events); !equalStrings(got, []string{"body", "unclaim", "state"}) {
		t.Fatalf("event types = %v", got)
	}
	if !equalStrings(sortedUniqueStrings(fields), []string{"body", "claim", "state"}) {
		t.Fatalf("updated fields = %v", fields)
	}
}

func TestMutationSameBlockedStateClearsLegacyClaim(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateBlocked, ClaimedBy: "legacy-agent"}
	events, fields, err := buildMutationEvents(task.ID, task, taskMutation{State: stateBlocked, StateSet: true}, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(events); !equalStrings(got, []string{"unclaim"}) {
		t.Fatalf("event types = %v", got)
	}
	if !equalStrings(fields, []string{"claim"}) {
		t.Fatalf("updated fields = %v", fields)
	}
}

func TestMutationValidationFailureDoesNotAppend(t *testing.T) {
	repoDir := t.TempDir()
	ergoDir := filepath.Join(repoDir, ".ergo")
	if err := os.MkdirAll(ergoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensureFileExists(filepath.Join(ergoDir, "lock"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	event, err := newEvent("new_task", now, NewTaskEvent{ID: "ABCDEF", UUID: "uuid", State: stateTodo, Title: "Task", CreatedAt: formatTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ergoDir, plansFileName)
	if err := writeEventsFile(path, []Event{event}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyTaskMutation(ergoDir, GlobalOptions{StartDir: repoDir}, "ABCDEF", taskMutation{
		State: stateDone, StateSet: true, Body: "new", BodySet: true,
		ResultPath: "missing.txt", ResultSet: true,
	})
	if err == nil {
		t.Fatal("expected missing result validation error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid mutation appended a partial event batch")
	}
}

func eventTypes(events []Event) []string {
	types := make([]string, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
