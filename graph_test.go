package main

import (
	"encoding/json"
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
			Body:      "Task 1",
			Worker:    "any",
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
			Body:      "Task 1",
			Worker:    "any",
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
			Body:      "Task 1",
			Worker:    "any",
			CreatedAt: formatTime(now),
		}),
		mustNewEvent("new_task", now, NewTaskEvent{
			ID:        "T2",
			UUID:      "uuid-2",
			State:     stateTodo,
			Body:      "Task 2",
			Worker:    "any",
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

// Helper to create events without error handling in tests
func mustNewEvent(eventType string, ts time.Time, payload interface{}) Event {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return Event{Type: eventType, TS: formatTime(ts), Data: data}
}
