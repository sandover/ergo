// Purpose: Prove replay rejects histories it cannot interpret completely.
package ergo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReplayUnknownEventIncludesTransactionSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, backlogFileName)
	content := `{"type":"transaction","version":1,"events":[{"type":"mystery","ts":"2026-01-01T00:00:00Z","data":{"id":"ABCDEF","future":true}}]}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	events, err := readEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = replayEvents(events)
	if err == nil {
		t.Fatal("unknown event unexpectedly replayed")
	}
	for _, want := range []string{path + ":1", "transaction event 1", `"mystery"`, "unknown event kind"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestReplayCorruptHistoriesFailClosed(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	task := func(id string) Event {
		return mustNewEvent("new_task", now, NewTaskEvent{
			ID: id, UUID: "uuid-" + id, State: stateTodo, Title: id, CreatedAt: formatTime(now),
		})
	}
	tests := []struct {
		name   string
		events []Event
		want   []string
	}{
		{
			name: "orphan mutation",
			events: []Event{mustNewEvent("title", now, TitleUpdateEvent{
				ID: "MISSING", Title: "lost", TS: formatTime(now),
			})},
			want: []string{`event "title"`, "MISSING", "orphan"},
		},
		{
			name: "dangling dependency",
			events: []Event{
				task("FROM"),
				mustNewEvent("link", now, LinkEvent{FromID: "FROM", ToID: "MISSING", Type: dependsLinkType}),
			},
			want: []string{`event "link"`, "FROM -> MISSING", "dangling"},
		},
		{
			name: "invalid state",
			events: []Event{
				task("TASK"),
				mustNewEvent("state", now, StateEvent{ID: "TASK", NewState: "future", TS: formatTime(now)}),
			},
			want: []string{`event "state"`, "TASK", `invalid state "future"`},
		},
		{
			name: "invalid claim postcondition",
			events: []Event{
				task("TASK"),
				mustNewEvent("claim", now, ClaimEvent{ID: "TASK", AgentID: "agent", TS: formatTime(now)}),
			},
			want: []string{`event "claim"`, "TASK", "state=todo must have no claim"},
		},
		{
			name: "cycle",
			events: []Event{
				task("FIRST"), task("SECOND"),
				mustNewEvent("link", now, LinkEvent{FromID: "FIRST", ToID: "SECOND", Type: dependsLinkType}),
				mustNewEvent("link", now, LinkEvent{FromID: "SECOND", ToID: "FIRST", Type: dependsLinkType}),
			},
			want: []string{`event "link"`, "SECOND -> FIRST", "cycle"},
		},
		{
			name: "unknown parent",
			events: []Event{
				mustNewEvent("new_task", now, NewTaskEvent{
					ID: "CHILD", UUID: "uuid-child", EpicID: "MISSING", State: stateTodo, Title: "Child", CreatedAt: formatTime(now),
				}),
			},
			want: []string{`event "new_task"`, "CHILD", "unknown parent epic MISSING"},
		},
		{
			name: "nested parent",
			events: []Event{
				task("ROOT"),
				mustNewEvent("new_task", now, NewTaskEvent{
					ID: "MIDDLE", UUID: "uuid-middle", EpicID: "ROOT", State: stateTodo, Title: "Middle", CreatedAt: formatTime(now),
				}),
				mustNewEvent("new_task", now, NewTaskEvent{
					ID: "LEAF", UUID: "uuid-leaf", EpicID: "MIDDLE", State: stateTodo, Title: "Leaf", CreatedAt: formatTime(now),
				}),
			},
			want: []string{`event "new_task"`, "LEAF", "nested epic relationship"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := replayEvents(test.events)
			if err == nil {
				t.Fatal("corrupt history unexpectedly replayed")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestReplayUnknownDependencyIsConservativelyIncomplete(t *testing.T) {
	graph := &Graph{Tasks: map[string]*Task{}}
	if isDepComplete("MISSING", graph) {
		t.Fatal("unknown dependency must not count as complete")
	}
}

func TestReplayInvariantUsesEpicAssignmentThatInvalidatedExistingLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, backlogFileName)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{
			ID: "CHILD", UUID: "uuid-child", State: stateTodo, Title: "Child", CreatedAt: formatTime(now),
		}),
		mustNewEvent("new_task", now, NewTaskEvent{
			ID: "PARENT", UUID: "uuid-parent", State: stateTodo, Title: "Parent", CreatedAt: formatTime(now),
		}),
		mustNewEvent("link", now, LinkEvent{
			FromID: "CHILD", ToID: "PARENT", Type: dependsLinkType,
		}),
		mustNewEvent("epic", now, EpicAssignEvent{
			ID: "CHILD", EpicID: "PARENT", TS: formatTime(now),
		}),
	}
	if err := writeEventsFile(path, events); err != nil {
		t.Fatal(err)
	}
	read, err := readEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = replayEvents(read)
	if err == nil {
		t.Fatal("parent assignment unexpectedly preserved a child-to-parent dependency")
	}
	for _, want := range []string{
		path + ":4",
		`event "epic"`,
		"CHILD -> PARENT",
		"task cannot depend on its own epic",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestReplayToleratesAdditivePayloadFields(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	event := Event{
		Type: "new_task",
		TS:   formatTime(now),
		Data: []byte(`{"id":"TASK","uuid":"uuid-task","epic_id":"","state":"todo","title":"Task","body":"","created_at":"2026-01-02T03:04:05Z","future_field":{"enabled":true}}`),
	}
	graph, err := replayEvents([]Event{event})
	if err != nil {
		t.Fatal(err)
	}
	if graph.Tasks["TASK"] == nil {
		t.Fatal("recognized event with additive fields was not replayed")
	}
}

func TestLegacyReleasedFixturesPassStrictReplay(t *testing.T) {
	for _, fixture := range releasedFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join("testdata", "compatibility", fixture.file)
			events, err := readEvents(path)
			if err != nil {
				t.Fatalf("read released fixture: %v", err)
			}
			graph, err := replayEvents(events)
			if err != nil {
				t.Fatalf("strict replay rejected released fixture: %v", err)
			}
			assertReleasedFixture(t, graph, fixture)
		})
	}
}
