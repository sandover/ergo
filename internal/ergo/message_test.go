// Purpose: Verify durable lifecycle message validation, replay, and compaction.
// Exports: none.
// Role: Domain and storage coverage for the v3 lifecycle message event.
// Invariants: messages are leaf-only, newest-first, and lossless across compact.
// Invariants: invalid message mutations append no events.
package ergo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMessageReplayAndCompact(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UTC()
	events := []Event{
		mustNewEventT(t, "new_task", t0, NewTaskEvent{
			ID: "ABCDEF", UUID: "uuid", State: stateTodo, Title: "Task", CreatedAt: formatTime(t0),
		}),
		messageEvent(t, t0.Add(time.Minute), "ABCDEF", "block", "Waiting for credentials."),
		messageEvent(t, t0.Add(2*time.Minute), "ABCDEF", "release", "Credentials arrived.\n\nRetry cleanly."),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{
		{Kind: "release", Text: "Credentials arrived.\n\nRetry cleanly.", CreatedAt: t0.Add(2 * time.Minute)},
		{Kind: "block", Text: "Waiting for credentials.", CreatedAt: t0.Add(time.Minute)},
	}
	if got := graph.Tasks["ABCDEF"].Messages; !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}

	compacted, err := compactEvents(graph)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := replayEvents(compacted)
	if err != nil {
		t.Fatal(err)
	}
	if got := replayed.Tasks["ABCDEF"].Messages; !reflect.DeepEqual(got, want) {
		t.Fatalf("compacted messages = %#v, want %#v", got, want)
	}
}

func TestMessageMutationValidation(t *testing.T) {
	task := &Task{ID: "ABCDEF", State: stateDoing, ClaimedBy: "agent"}
	now := time.Now().UTC()
	events, fields, err := buildMutationEvents(task.ID, task, taskMutation{
		State: stateDone, StateSet: true,
		MessageKind: "done", MessageText: "Verified.", MessageSet: true,
	}, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(events); !equalStrings(got, []string{"message", "unclaim", "state"}) {
		t.Fatalf("event types = %v", got)
	}
	if !equalStrings(sortedUniqueStrings(fields), []string{"claim", "message", "state"}) {
		t.Fatalf("updated fields = %v", fields)
	}

	for _, mutation := range []taskMutation{
		{MessageKind: "finish", MessageText: "Text", MessageSet: true},
		{MessageKind: "done", MessageText: "  ", MessageSet: true},
	} {
		if _, _, err := buildMutationEvents(task.ID, task, mutation, "", now); err == nil {
			t.Fatalf("expected validation error for %#v", mutation)
		}
	}
}

func TestContainerRejectsMessageWithoutAppending(t *testing.T) {
	repoDir := t.TempDir()
	ergoDir := filepath.Join(repoDir, ".ergo")
	if err := os.MkdirAll(ergoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensureFileExists(filepath.Join(ergoDir, "lock"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	events := []Event{
		mustNewEventT(t, "new_task", now, NewTaskEvent{ID: "PARENT", UUID: "parent", State: stateTodo, Title: "Parent", CreatedAt: formatTime(now)}),
		mustNewEventT(t, "new_task", now, NewTaskEvent{ID: "CHILD1", UUID: "child", EpicID: "PARENT", State: stateTodo, Title: "Child", CreatedAt: formatTime(now)}),
	}
	path := filepath.Join(ergoDir, plansFileName)
	if err := writeEventsFile(path, events); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = applyTaskMutation(ergoDir, GlobalOptions{StartDir: repoDir}, "PARENT", taskMutation{
		MessageKind: "done", MessageText: "Nope", MessageSet: true,
	})
	if err == nil {
		t.Fatal("expected container message error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid container message appended events")
	}
}
