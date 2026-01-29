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
	err := RunClaimOldestReady(opts)
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
		"worker": "human",
		"state":  "doing",
		"claim":  "agent-1",
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
