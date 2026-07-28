// Purpose: Prove lifecycle commands explicitly normalize readable legacy states.
// Exports: none.
// Role: Narrow synthetic edge-case coverage beside released compatibility fixtures.
// Invariants: only an explicit lifecycle postcondition clears legacy ownership.
package ergo

import (
	"testing"
	"time"
)

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
