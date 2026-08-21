// These tests protect deterministic prune selection independently of storage.
// Every finished leaf is eligible, and epics become eligible only after all of
// their remaining children disappear from the plan.
package ergo

import (
	"reflect"
	"testing"
)

func TestSelectPruneTargets_TaskEligibilityAndEpics(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"E1": {ID: "E1"},
			"E2": {ID: "E2"},
			"E3": {ID: "E3"}, // legacy empty epic
			"E4": {ID: "E4"},
			"T1": {ID: "T1", EpicID: "E1", State: stateDone},
			"T2": {ID: "T2", EpicID: "E1", State: stateCanceled},
			"T3": {ID: "T3", EpicID: "E2", State: stateTodo},
			"T4": {ID: "T4", State: stateBlocked},
			"T5": {ID: "T5", State: stateDoing},
			"T6": {ID: "T6", State: stateError},
			"T7": {ID: "T7", EpicID: "E4", State: stateDone},
			"T8": {ID: "T8", EpicID: "E4", State: stateTodo},
			"T9": {ID: "T9", State: stateFailed},
		},
		legacyEmptyEpics: map[string]struct{}{"E3": {}},
	}

	got := selectPruneTargets(graph)
	want := []string{"E1", "E3", "T1", "T2", "T7", "T9"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSelectPruneTargets_EmptyEpicsAfterTaskPrune(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"E1": {ID: "E1"},
			"E2": {ID: "E2"},
			"T1": {ID: "T1", EpicID: "E1", State: stateDone},
			"T2": {ID: "T2", EpicID: "E1", State: stateCanceled},
			"T3": {ID: "T3", EpicID: "E2", State: stateBlocked},
		},
	}

	got := selectPruneTargets(graph)
	want := []string{"E1", "T1", "T2"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
