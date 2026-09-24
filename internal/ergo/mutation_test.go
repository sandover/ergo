// Purpose: Verify atomic mutation event batches and validation rollback.
// Exports: none.
// Role: Unit and storage-level coverage for the shared v2 write path.
// Invariants: true no-ops append nothing and invalid batches append nothing.
// Invariants: lifecycle postconditions clear legacy claims when required.
package ergo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMutationSuppressesTrueNoop(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateDone, Title: "Task", Body: "Body"}
	change, err := buildStateChange(task, stateDone, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(change.events) != 0 || len(change.fields) != 0 {
		t.Fatalf("true no-op produced events=%d fields=%v", len(change.events), change.fields)
	}
}

func TestLifecycleBuildsAtomicStateBatch(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateDoing, ClaimedBy: "agent-1"}
	graph := &Graph{Tasks: map[string]*Task{task.ID: task}}
	change, err := lifecycleChange("block", stateBlocked, "", false)(graph, task, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(change.events); !equalStrings(got, []string{"unclaim", "state"}) {
		t.Fatalf("event types = %v", got)
	}
	if !equalStrings(change.fields, []string{"claim", "state"}) {
		t.Fatalf("updated fields = %v", change.fields)
	}
}

func TestMutationAppendsBodyAgainstLockedTaskState(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateTodo, Title: "Task", Body: "existing"}
	change, err := bodyChange("+new", true)(nil, task, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(change.events); !equalStrings(got, []string{"body"}) {
		t.Fatalf("event types = %v", got)
	}
	if !equalStrings(change.fields, []string{"body"}) {
		t.Fatalf("updated fields = %v", change.fields)
	}
	var update BodyUpdateEvent
	if err := json.Unmarshal(change.events[0].Data, &update); err != nil {
		t.Fatal(err)
	}
	if update.Body != "existing+new" {
		t.Fatalf("appended body = %q", update.Body)
	}
}

func TestMutationSameBlockedStateClearsLegacyClaim(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateBlocked, ClaimedBy: "legacy-agent"}
	change, err := buildStateChange(task, stateBlocked, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(change.events); !equalStrings(got, []string{"unclaim"}) {
		t.Fatalf("event types = %v", got)
	}
	if !equalStrings(change.fields, []string{"claim"}) {
		t.Fatalf("updated fields = %v", change.fields)
	}
}

func TestContentAndMoveKeepLegacyClaimValidation(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateBlocked, ClaimedBy: "legacy-agent", Title: "Task"}
	graph := &Graph{Tasks: map[string]*Task{task.ID: task}}
	now := time.Now().UTC()
	for name, build := range map[string]taskChange{
		"title": titleChange("New title"),
		"body":  bodyChange("New body", false),
		"move":  moveChange(""),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := build(graph, task, now); err == nil {
				t.Fatal("expected legacy claim invariant error")
			}
		})
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
	_, err = applyTaskChange(ergoDir, GlobalOptions{StartDir: repoDir}, "ABCDEF", false, titleChange(" "))
	if err == nil {
		t.Fatal("expected title validation error")
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
