// Tests for graph replay, traversal, and state derivation.
// Validates event handling and dependency resolution.
package ergo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Test event replay - core event sourcing logic
func TestReplayEvents_StateTransitions(t *testing.T) {
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
		mustNewEvent("state", now.Add(time.Second), StateEvent{
			ID:       "T1",
			NewState: stateDoing,
			TS:       formatTime(now.Add(time.Second)),
		}),
		mustNewEvent("state", now.Add(2*time.Second), StateEvent{
			ID:       "T1",
			NewState: stateDone,
			TS:       formatTime(now.Add(2 * time.Second)),
		}),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	task := graph.Tasks["T1"]
	if task == nil {
		t.Fatal("Task T1 not found")
	}
	if task.State != stateDone {
		t.Errorf("Expected state=done, got %s", task.State)
	}
}

func TestReplayEvents_Claims(t *testing.T) {
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
		mustNewEvent("claim", now.Add(time.Second), ClaimEvent{
			ID:      "T1",
			AgentID: "agent-1",
			TS:      formatTime(now.Add(time.Second)),
		}),
		mustNewEvent("state", now.Add(2*time.Second), StateEvent{
			ID:       "T1",
			NewState: stateTodo, // should clear claim
			TS:       formatTime(now.Add(2 * time.Second)),
		}),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	task := graph.Tasks["T1"]
	if task.ClaimedBy != "" {
		t.Errorf("Expected claim cleared by state=todo, got ClaimedBy=%s", task.ClaimedBy)
	}
}

func TestReplayEvents_Dependencies(t *testing.T) {
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
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T2",
			UUID:      "uuid-2",
			State:     stateTodo,
			Title:     "Task 2",
			Body:      "Task 2",
			CreatedAt: formatTime(now),
		}),
		mustNewEvent("link", now, LinkEvent{
			FromID: "T1",
			ToID:   "T2",
			Type:   "depends",
		}),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	if len(graph.Deps["T1"]) != 1 {
		t.Errorf("Expected T1 to have 1 dep, got %d", len(graph.Deps["T1"]))
	}
	if _, ok := graph.Deps["T1"]["T2"]; !ok {
		t.Error("Expected T1 depends on T2")
	}
	if len(graph.RDeps["T2"]) != 1 {
		t.Errorf("Expected T2 to have 1 rdep, got %d", len(graph.RDeps["T2"]))
	}
}

func TestReplayEvents_TombstoneParses(t *testing.T) {
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

	if _, err := replayEvents(events); err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}
}

func TestReplayEvents_TombstoneRemovesTaskAndDeps(t *testing.T) {
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
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T2",
			UUID:      "uuid-2",
			State:     stateTodo,
			Title:     "Task 2",
			Body:      "Task 2",
			CreatedAt: formatTime(now),
		}),
		mustNewEvent("link", now, LinkEvent{
			FromID: "T1",
			ToID:   "T2",
			Type:   "depends",
		}),
		mustNewEvent("tombstone", now.Add(time.Second), TombstoneEvent{
			ID:      "T2",
			AgentID: "agent-1",
			TS:      formatTime(now.Add(time.Second)),
		}),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}
	if _, ok := graph.Tasks["T2"]; ok {
		t.Fatalf("expected T2 to be tombstoned and removed")
	}
	if deps := graph.Deps["T1"]; len(deps) != 0 {
		t.Fatalf("expected tombstoned deps to be dropped, got %v", deps)
	}
}

func TestReplayEvents_TombstoneBeforeCreateWins(t *testing.T) {
	now := time.Now().UTC()

	events := []Event{
		mustNewEvent("tombstone", now, TombstoneEvent{
			ID:      "T1",
			AgentID: "agent-1",
			TS:      formatTime(now),
		}),
		mustNewEvent("new_task", now.Add(time.Second), NewTaskEvent{
			ID:        "T1",
			UUID:      "uuid-1",
			State:     stateTodo,
			Title:     "Task 1",
			Body:      "Task 1",
			CreatedAt: formatTime(now.Add(time.Second)),
		}),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}
	if _, ok := graph.Tasks["T1"]; ok {
		t.Fatalf("expected tombstone to block later create")
	}
	if _, ok := graph.Tombstones["T1"]; !ok {
		t.Fatalf("expected tombstone to be recorded")
	}
}

func TestReplayEvents_TombstoneRandomized(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	base := time.Unix(1_700_000_000, 0).UTC()
	now := func(i int) time.Time { return base.Add(time.Duration(i) * time.Second) }

	var events []Event
	tombstoned := map[string]struct{}{}

	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("T%d", i+1)
		ts := now(len(events))
		events = append(events, mustNewEvent("new_task", ts, NewTaskEvent{
			ID:        id,
			UUID:      "uuid-" + id,
			State:     stateTodo,
			Title:     "Task " + id,
			Body:      "Body " + id,
			CreatedAt: formatTime(ts),
		}))

		if r.Intn(3) == 0 {
			ts = now(len(events))
			events = append(events, mustNewEvent("state", ts, StateEvent{
				ID:       id,
				NewState: stateDoing,
				TS:       formatTime(ts),
			}))
		}

		if r.Intn(4) == 0 {
			ts = now(len(events))
			events = append(events, mustNewEvent("tombstone", ts, TombstoneEvent{
				ID:      id,
				AgentID: "agent-1",
				TS:      formatTime(ts),
			}))
			tombstoned[id] = struct{}{}

			// Add post-tombstone noise to ensure replay ignores it.
			ts = now(len(events))
			events = append(events, mustNewEvent("title", ts, TitleUpdateEvent{
				ID:    id,
				Title: "ignored",
				TS:    formatTime(ts),
			}))
		}
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	for id := range tombstoned {
		if _, ok := graph.Tasks[id]; ok {
			t.Fatalf("expected tombstoned id %s to be absent", id)
		}
		if deps := graph.Deps[id]; len(deps) != 0 {
			t.Fatalf("expected tombstoned id %s to have no deps, got %v", id, deps)
		}
	}
}

// Test READY/BLOCKED logic - critical for task selection
func TestIsReady_BasicCases(t *testing.T) {
	tests := []struct {
		name     string
		task     *Task
		deps     map[string]*Task
		expected bool
	}{
		{
			name:     "todo unclaimed no deps",
			task:     &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps:     map[string]*Task{},
			expected: true,
		},
		{
			name:     "doing state not ready",
			task:     &Task{ID: "T1", State: stateDoing, ClaimedBy: ""},
			deps:     map[string]*Task{},
			expected: false,
		},
		{
			name:     "todo but claimed not ready",
			task:     &Task{ID: "T1", State: stateTodo, ClaimedBy: "agent-1"},
			deps:     map[string]*Task{},
			expected: false,
		},
		{
			name: "todo with done deps is ready",
			task: &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps: map[string]*Task{
				"T2": {ID: "T2", State: stateDone},
			},
			expected: true,
		},
		{
			name: "todo with todo deps not ready",
			task: &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps: map[string]*Task{
				"T2": {ID: "T2", State: stateTodo},
			},
			expected: false,
		},
		{
			name: "todo with canceled deps is ready",
			task: &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps: map[string]*Task{
				"T2": {ID: "T2", State: stateCanceled},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := &Graph{
				Tasks: map[string]*Task{tt.task.ID: tt.task},
				Deps:  map[string]map[string]struct{}{},
			}

			// Add deps
			if len(tt.deps) > 0 {
				graph.Deps[tt.task.ID] = make(map[string]struct{})
				for depID, depTask := range tt.deps {
					graph.Tasks[depID] = depTask
					graph.Deps[tt.task.ID][depID] = struct{}{}
				}
			}

			result := isReady(tt.task, graph)
			if result != tt.expected {
				t.Errorf("Expected isReady=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsBlocked_BasicCases(t *testing.T) {
	tests := []struct {
		name     string
		task     *Task
		deps     map[string]*Task
		expected bool
	}{
		{
			name:     "state=blocked is blocked",
			task:     &Task{ID: "T1", State: stateBlocked},
			deps:     map[string]*Task{},
			expected: true,
		},
		{
			name:     "todo with unmet deps is blocked",
			task:     &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps:     map[string]*Task{"T2": {ID: "T2", State: stateTodo}},
			expected: true,
		},
		{
			name:     "todo with met deps not blocked",
			task:     &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps:     map[string]*Task{"T2": {ID: "T2", State: stateDone}},
			expected: false,
		},
		{
			name:     "doing not blocked by deps",
			task:     &Task{ID: "T1", State: stateDoing, ClaimedBy: "agent-1"},
			deps:     map[string]*Task{"T2": {ID: "T2", State: stateTodo}},
			expected: false,
		},
		{
			name:     "todo with error dep is blocked",
			task:     &Task{ID: "T1", State: stateTodo, ClaimedBy: ""},
			deps:     map[string]*Task{"T2": {ID: "T2", State: stateError, ClaimedBy: "agent-1"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := &Graph{
				Tasks: map[string]*Task{tt.task.ID: tt.task},
				Deps:  map[string]map[string]struct{}{},
			}

			if len(tt.deps) > 0 {
				graph.Deps[tt.task.ID] = make(map[string]struct{})
				for depID, depTask := range tt.deps {
					graph.Tasks[depID] = depTask
					graph.Deps[tt.task.ID][depID] = struct{}{}
				}
			}

			result := isBlocked(tt.task, graph)
			if result != tt.expected {
				t.Errorf("Expected isBlocked=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestStateClearsClaim(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name           string
		newState       string
		expectClaimNil bool
	}{
		{name: "todo clears claim", newState: stateTodo, expectClaimNil: true},
		{name: "done clears claim", newState: stateDone, expectClaimNil: true},
		{name: "canceled clears claim", newState: stateCanceled, expectClaimNil: true},
		{name: "doing keeps claim", newState: stateDoing, expectClaimNil: false},
		{name: "blocked keeps claim", newState: stateBlocked, expectClaimNil: false},
		{name: "error keeps claim", newState: stateError, expectClaimNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := []Event{
				mustNewEvent("new_task", now, NewTaskEvent{
					ID:        "T1",
					UUID:      "uuid-1",
					State:     stateDoing,
					Title:     "Test",
					Body:      "Test",
					CreatedAt: formatTime(now),
				}),
				mustNewEvent("claim", now, ClaimEvent{ID: "T1", AgentID: "agent-1", TS: formatTime(now)}),
				mustNewEvent("state", now, StateEvent{ID: "T1", NewState: tt.newState, TS: formatTime(now)}),
			}
			graph, err := replayEvents(events)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			task := graph.Tasks["T1"]
			if tt.expectClaimNil && task.ClaimedBy != "" {
				t.Errorf("Expected claim cleared, got %q", task.ClaimedBy)
			}
			if !tt.expectClaimNil && task.ClaimedBy == "" {
				t.Error("Expected claim kept, but was cleared")
			}
		})
	}
}

func TestLegacyTitleMigration(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T1",
			UUID:      "uuid-1",
			State:     stateTodo,
			Title:     "",
			Body:      "Title line\nDetails line",
			CreatedAt: formatTime(now),
		}),
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T2",
			UUID:      "uuid-2",
			State:     stateTodo,
			Title:     "",
			Body:      "# Heading\n\nActual title\nDetails",
			CreatedAt: formatTime(now),
		}),
	}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	task1 := graph.Tasks["T1"]
	if task1.Title != "Title line" {
		t.Errorf("expected migrated title 'Title line', got %q", task1.Title)
	}
	if task1.Body != "Details line" {
		t.Errorf("expected migrated body 'Details line', got %q", task1.Body)
	}

	task2 := graph.Tasks["T2"]
	if task2.Title != "Actual title" {
		t.Errorf("expected migrated title 'Actual title', got %q", task2.Title)
	}
	if task2.Body != "Details" {
		t.Errorf("expected migrated body 'Details', got %q", task2.Body)
	}
}

// Helper to create events without error handling in tests
func mustNewEvent(eventType string, ts time.Time, payload interface{}) Event {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return Event{Type: eventType, TS: formatTime(ts), Data: data}
}

func TestIsEpicComplete(t *testing.T) {
	tests := []struct {
		name     string
		epicID   string
		tasks    map[string]*Task
		expected bool
	}{
		{
			name:     "empty epic is complete",
			epicID:   "E1",
			tasks:    map[string]*Task{},
			expected: true,
		},
		{
			name:   "all tasks done",
			epicID: "E1",
			tasks: map[string]*Task{
				"T1": {ID: "T1", EpicID: "E1", State: stateDone},
				"T2": {ID: "T2", EpicID: "E1", State: stateDone},
			},
			expected: true,
		},
		{
			name:   "all tasks canceled",
			epicID: "E1",
			tasks: map[string]*Task{
				"T1": {ID: "T1", EpicID: "E1", State: stateCanceled},
			},
			expected: true,
		},
		{
			name:   "mix of done and canceled",
			epicID: "E1",
			tasks: map[string]*Task{
				"T1": {ID: "T1", EpicID: "E1", State: stateDone},
				"T2": {ID: "T2", EpicID: "E1", State: stateCanceled},
			},
			expected: true,
		},
		{
			name:   "one task not done",
			epicID: "E1",
			tasks: map[string]*Task{
				"T1": {ID: "T1", EpicID: "E1", State: stateDone},
				"T2": {ID: "T2", EpicID: "E1", State: stateTodo},
			},
			expected: false,
		},
		{
			name:   "task in different epic ignored",
			epicID: "E1",
			tasks: map[string]*Task{
				"T1": {ID: "T1", EpicID: "E1", State: stateDone},
				"T2": {ID: "T2", EpicID: "E2", State: stateTodo}, // different epic
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := &Graph{Tasks: tt.tasks}
			result := isEpicComplete(tt.epicID, graph)
			if result != tt.expected {
				t.Errorf("Expected isEpicComplete=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsReady_EpicDeps(t *testing.T) {
	tests := []struct {
		name     string
		task     *Task
		setup    func(*Graph)
		expected bool
	}{
		{
			name: "task ready when epic has no deps",
			task: &Task{ID: "T1", EpicID: "E1", State: stateTodo},
			setup: func(g *Graph) {
				g.Tasks["E1"] = &Task{ID: "E1", EpicID: "", IsEpic: true} // E1 is an epic
			},
			expected: true,
		},
		{
			name: "task blocked when epic dep incomplete",
			task: &Task{ID: "T1", EpicID: "E-build", State: stateTodo},
			setup: func(g *Graph) {
				g.Tasks["E-build"] = &Task{ID: "E-build", EpicID: "", IsEpic: true}   // epic
				g.Tasks["E-design"] = &Task{ID: "E-design", EpicID: "", IsEpic: true} // epic
				g.Tasks["T-design"] = &Task{ID: "T-design", EpicID: "E-design", State: stateTodo}
				g.Deps["E-build"] = map[string]struct{}{"E-design": {}}
			},
			expected: false,
		},
		{
			name: "task ready when epic dep complete",
			task: &Task{ID: "T1", EpicID: "E-build", State: stateTodo},
			setup: func(g *Graph) {
				g.Tasks["E-build"] = &Task{ID: "E-build", EpicID: "", IsEpic: true}   // epic
				g.Tasks["E-design"] = &Task{ID: "E-design", EpicID: "", IsEpic: true} // epic
				g.Tasks["T-design"] = &Task{ID: "T-design", EpicID: "E-design", State: stateDone}
				g.Deps["E-build"] = map[string]struct{}{"E-design": {}}
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := &Graph{
				Tasks: map[string]*Task{tt.task.ID: tt.task},
				Deps:  map[string]map[string]struct{}{},
			}
			if tt.setup != nil {
				tt.setup(graph)
			}
			result := isReady(tt.task, graph)
			if result != tt.expected {
				t.Errorf("Expected isReady=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasCycle(t *testing.T) {
	tests := []struct {
		name     string
		deps     map[string]map[string]struct{}
		from     string
		to       string
		expected bool
	}{
		{
			name:     "self loop",
			deps:     map[string]map[string]struct{}{},
			from:     "A",
			to:       "A",
			expected: true,
		},
		{
			name:     "no existing deps - no cycle",
			deps:     map[string]map[string]struct{}{},
			from:     "A",
			to:       "B",
			expected: false,
		},
		{
			name: "direct cycle: A->B, adding B->A",
			deps: map[string]map[string]struct{}{
				"A": {"B": {}},
			},
			from:     "B",
			to:       "A",
			expected: true,
		},
		{
			name: "indirect cycle: A->B->C, adding C->A",
			deps: map[string]map[string]struct{}{
				"A": {"B": {}},
				"B": {"C": {}},
			},
			from:     "C",
			to:       "A",
			expected: true,
		},
		{
			name: "no cycle in diamond: A->B, A->C, B->D, C->D",
			deps: map[string]map[string]struct{}{
				"A": {"B": {}, "C": {}},
				"B": {"D": {}},
				"C": {"D": {}},
			},
			from:     "D",
			to:       "E", // new node
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := &Graph{
				Tasks: map[string]*Task{},
				Deps:  tt.deps,
			}
			result := hasCycle(graph, tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("Expected hasCycle=%v, got %v", tt.expected, result)
			}
		})
	}
}
