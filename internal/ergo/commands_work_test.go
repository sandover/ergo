package ergo

import (
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
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid state transition",
			updates:     map[string]string{"state": "doing"},
			expectError: true, // should fail - doing requires claim
			errorMsg:    "state=doing requires a claim",
		},
		{
			name:        "invalid state",
			updates:     map[string]string{"state": "invalid"},
			expectError: true,
			errorMsg:    "invalid state",
		},
		{
			name:        "valid state with claim",
			updates:     map[string]string{"claim": "agent-1", "state": "doing"},
			expectError: false, // claim is set in same update, so state=doing should work
		},
		{
			name:        "empty title rejected",
			updates:     map[string]string{"title": ""},
			expectError: true,
			errorMsg:    "title cannot be empty",
		},
		{
			name:        "empty title with whitespace rejected",
			updates:     map[string]string{"title": "   "},
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

			_, _, err := buildSetEvents("T1", task, tt.updates, now, bodyResolver)

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

			events, _, err := buildSetEvents(tt.task.ID, tt.task, tt.updates, now, bodyResolver)
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

	_, remaining, err := buildSetEvents("T1", task, updates, now, bodyResolver)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// The function should have consumed "state" but left "unknown"
	updates2 := map[string]string{"unknown": "value", "state": "done"}
	_, remaining2, _ := buildSetEvents("T1", task, updates2, now, bodyResolver)

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
		"worker": "human",
		"state":  "doing",
		"claim":  "agent-1",
	}

	events, _, err := buildSetEvents("T1", task, updates, now, bodyResolver)
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
