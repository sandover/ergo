// Tests for work command validation and event generation.
// Focuses on claim/state invariants and buildSetEvents behavior.
package ergo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test buildSetEvents - critical validation logic
func TestBuildSetEvents_StateValidation(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{ID: "T1", State: stateTodo, ClaimedBy: "", EpicID: "E1"} // task not epic

	tests := []struct {
		name        string
		updates     map[string]string
		agentID     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "implicit claim when transitioning to doing",
			updates:     map[string]string{"state": "doing"},
			agentID:     "test-agent",
			expectError: false, // now succeeds via implicit claim
		},
		{
			name:        "implicit claim without agent id",
			updates:     map[string]string{"state": "doing"},
			agentID:     "",
			expectError: true,
			errorMsg:    "state requires claim",
		},
		{
			name:        "invalid state",
			updates:     map[string]string{"state": "invalid"},
			agentID:     "test-agent",
			expectError: true,
			errorMsg:    "invalid state",
		},
		{
			name:        "valid state with explicit claim",
			updates:     map[string]string{"claim": "agent-1", "state": "doing"},
			agentID:     "",
			expectError: false,
		},
		{
			name:        "empty title rejected",
			updates:     map[string]string{"title": ""},
			agentID:     "test-agent",
			expectError: true,
			errorMsg:    "title cannot be empty",
		},
		{
			name:        "empty title with whitespace rejected",
			updates:     map[string]string{"title": "   "},
			agentID:     "test-agent",
			expectError: true,
			errorMsg:    "title cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock body resolver that just returns the input
			bodyResolver := func(s string) (string, error) {
				return s, nil
			}

			_, _, err := buildSetEvents("T1", task, tt.updates, tt.agentID, now, bodyResolver)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestBuildSetEvents_StateValidation_ClaimedTaskNoAgent(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{ID: "T1", State: stateTodo, ClaimedBy: "agent-1", EpicID: "E1"}
	updates := map[string]string{"state": "doing"}
	bodyResolver := func(s string) (string, error) { return s, nil }

	_, _, err := buildSetEvents("T1", task, updates, "", now, bodyResolver)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

func TestRunClaimRequiresAgent(t *testing.T) {
	err := RunClaim("T1", GlobalOptions{})
	if err == nil || !contains(err.Error(), "claim requires --agent") {
		t.Fatalf("Expected claim requires --agent error, got %v", err)
	}
}

func TestRunClaimOldestReadyRequiresAgent(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".ergo"), 0755); err != nil {
		t.Fatalf("failed to create .ergo dir: %v", err)
	}
	opts := GlobalOptions{StartDir: repoDir}
	err := RunClaimOldestReady("", opts)
	if err == nil || !contains(err.Error(), "claim requires --agent") {
		t.Fatalf("Expected claim requires --agent error, got %v", err)
	}
}

func TestBuildSetEvents_ClaimHandling(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name          string
		task          *Task
		updates       map[string]string
		expectEvents  int
		checkClaim    bool
		expectClaimed bool
	}{
		{
			name:         "set claim generates claim event and state=doing",
			task:         &Task{ID: "T1", State: stateTodo, ClaimedBy: "", EpicID: "E1"}, // task has EpicID
			updates:      map[string]string{"claim": "agent-1"},
			expectEvents: 2, // claim + state=doing
		},
		{
			name:         "clear claim generates unclaim event",
			task:         &Task{ID: "T1", State: stateTodo, ClaimedBy: "agent-1", EpicID: "E1"},
			updates:      map[string]string{"claim": ""},
			expectEvents: 1,
		},
		{
			name:         "claim on epic ignored",
			task:         &Task{ID: "E1", State: stateTodo, ClaimedBy: "", EpicID: "", IsEpic: true}, // epic has IsEpic=true
			updates:      map[string]string{"claim": "agent-1"},
			expectEvents: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyResolver := func(s string) (string, error) { return s, nil }

			events, _, err := buildSetEvents(tt.task.ID, tt.task, tt.updates, "test-agent", now, bodyResolver)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(events) != tt.expectEvents {
				t.Errorf("Expected %d events, got %d", tt.expectEvents, len(events))
			}
		})
	}
}

func TestBuildSetEvents_UnknownKeys(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{ID: "T1", State: stateTodo}
	bodyResolver := func(s string) (string, error) { return s, nil }

	updates := map[string]string{
		"valid":   "done",
		"unknown": "value",
	}

	// Map "valid" to "state" for this test
	updates["state"] = "done"
	delete(updates, "valid")

	_, remaining, err := buildSetEvents("T1", task, updates, "test-agent", now, bodyResolver)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// The function should have consumed "state" but left "unknown"
	updates2 := map[string]string{"unknown": "value", "state": "done"}
	_, remaining2, _ := buildSetEvents("T1", task, updates2, "test-agent", now, bodyResolver)

	if len(remaining2) != 1 {
		t.Errorf("Expected 1 unknown key, got %d (remaining: %v)", len(remaining2), remaining)
	}
	if _, ok := remaining2["unknown"]; !ok {
		t.Error("Expected 'unknown' key to remain")
	}
}

func TestBuildSetEvents_EventOrdering(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{ID: "T1", State: stateTodo, ClaimedBy: "", EpicID: "E1"} // task not epic
	bodyResolver := func(s string) (string, error) { return s, nil }

	// State must come last per spec
	updates := map[string]string{
		"state": "doing",
		"claim": "agent-1",
	}

	events, _, err := buildSetEvents("T1", task, updates, "test-agent", now, bodyResolver)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected some events")
	}

	// Last event should be state change
	lastEvent := events[len(events)-1]
	if lastEvent.Type != "state" {
		t.Errorf("Expected last event to be 'state', got %s", lastEvent.Type)
	}
}

func TestBuildSetEvents_TitleAndBody(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{ID: "T1", State: stateTodo, ClaimedBy: "", EpicID: "E1"}
	bodyResolver := func(s string) (string, error) { return s, nil }

	updates := map[string]string{
		"title": "New title",
		"body":  "New body",
	}

	events, _, err := buildSetEvents("T1", task, updates, "test-agent", now, bodyResolver)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[0].Type != "title" {
		t.Errorf("Expected first event to be title, got %s", events[0].Type)
	}
	if events[1].Type != "body" {
		t.Errorf("Expected second event to be body, got %s", events[1].Type)
	}
}

// TestRunClaimOldestReady_EpicFilter verifies that claim respects epic filtering.
func TestRunClaimOldestReady_EpicFilter(t *testing.T) {
	dir := t.TempDir()
	ergoDir := filepath.Join(dir, ".ergo")
	if err := os.Mkdir(ergoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create tasks in different epics
	now := time.Now().UTC()
	createdAt := formatTime(now)
	events := []Event{}

	// Epic E1 with task T1 (ready)
	e1, _ := newEvent("new_epic", now, NewTaskEvent{
		ID:        "E1",
		UUID:      "uuid-e1",
		State:     stateTodo,
		Title:     "Epic 1",
		CreatedAt: createdAt,
	})
	t1, _ := newEvent("new_task", now, NewTaskEvent{
		ID:        "T1",
		UUID:      "uuid-t1",
		EpicID:    "E1",
		State:     stateTodo,
		Title:     "Task 1",
		CreatedAt: createdAt,
	})
	events = append(events, e1, t1)

	// Epic E2 with task T2 (ready)
	e2, _ := newEvent("new_epic", now, NewTaskEvent{
		ID:        "E2",
		UUID:      "uuid-e2",
		State:     stateTodo,
		Title:     "Epic 2",
		CreatedAt: createdAt,
	})
	t2, _ := newEvent("new_task", now, NewTaskEvent{
		ID:        "T2",
		UUID:      "uuid-t2",
		EpicID:    "E2",
		State:     stateTodo,
		Title:     "Task 2",
		CreatedAt: createdAt,
	})
	events = append(events, e2, t2)

	// Task T3 with no epic (ready)
	t3, _ := newEvent("new_task", now, NewTaskEvent{
		ID:        "T3",
		UUID:      "uuid-t3",
		EpicID:    "",
		State:     stateTodo,
		Title:     "Task 3",
		CreatedAt: createdAt,
	})
	events = append(events, t3)

	eventsPath := filepath.Join(ergoDir, "events.jsonl")
	if err := writeEventsFile(eventsPath, events); err != nil {
		t.Fatal(err)
	}

	t.Run("claim filters to specified epic", func(t *testing.T) {
		opts := GlobalOptions{
			StartDir: dir,
			AgentID:  "test-agent",
		}

		// Claim from E2 should get T2
		err := RunClaimOldestReady("E2", opts)
		if err != nil {
			t.Fatalf("claim failed: %v", err)
		}

		// Verify T2 was claimed
		graph, _ := loadGraph(ergoDir)
		if graph.Tasks["T2"].State != stateDoing {
			t.Errorf("expected T2 to be doing, got %s", graph.Tasks["T2"].State)
		}
		if graph.Tasks["T2"].ClaimedBy != "test-agent" {
			t.Errorf("expected T2 claimed by test-agent, got %s", graph.Tasks["T2"].ClaimedBy)
		}
		// T1 and T3 should still be todo
		if graph.Tasks["T1"].State != stateTodo {
			t.Errorf("expected T1 to remain todo, got %s", graph.Tasks["T1"].State)
		}
		if graph.Tasks["T3"].State != stateTodo {
			t.Errorf("expected T3 to remain todo, got %s", graph.Tasks["T3"].State)
		}
	})

	t.Run("claim without epic filter gets oldest ready", func(t *testing.T) {
		// Reset T2
		resetEvent, _ := newEvent("state", now, StateEvent{ID: "T2", NewState: stateTodo, TS: formatTime(now)})
		unclaimEvent, _ := newEvent("unclaim", now, UnclaimEvent{ID: "T2", TS: formatTime(now)})
		if err := appendEvents(eventsPath, []Event{resetEvent, unclaimEvent}); err != nil {
			t.Fatal(err)
		}

		opts := GlobalOptions{
			StartDir: dir,
			AgentID:  "test-agent-2",
		}

		// Claim without epic should get T1 (oldest by ID)
		err := RunClaimOldestReady("", opts)
		if err != nil {
			t.Fatalf("claim failed: %v", err)
		}

		graph, _ := loadGraph(ergoDir)
		if graph.Tasks["T1"].State != stateDoing {
			t.Errorf("expected T1 to be claimed, got state %s", graph.Tasks["T1"].State)
		}
		if graph.Tasks["T1"].ClaimedBy != "test-agent-2" {
			t.Errorf("expected T1 claimed by test-agent-2, got %s", graph.Tasks["T1"].ClaimedBy)
		}
	})

	t.Run("claim with non-existent epic returns no tasks", func(t *testing.T) {
		opts := GlobalOptions{
			StartDir: dir,
			AgentID:  "test-agent-3",
		}

		// Claim from non-existent epic should succeed with no tasks message
		err := RunClaimOldestReady("NONEXISTENT", opts)
		if err != nil {
			t.Errorf("expected no error for non-existent epic, got: %v", err)
		}

		// Verify no new claims
		graph, _ := loadGraph(ergoDir)
		for id, task := range graph.Tasks {
			if task.ClaimedBy == "test-agent-3" {
				t.Errorf("expected no task claimed by test-agent-3, but %s was claimed", id)
			}
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
