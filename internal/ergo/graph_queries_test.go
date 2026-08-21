// Purpose: Verify canonical graph indexes and graph-owned domain queries.
package ergo

import (
	"testing"
	"time"
)

func TestGraphDerivedIndexesRemainConsistent(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"EPIC01": {ID: "EPIC01"},
			"TASK02": {ID: "TASK02", EpicID: "EPIC01"},
			"TASK01": {ID: "TASK01", EpicID: "EPIC01"},
			"BLOCK1": {ID: "BLOCK1"},
		},
		Deps: map[string]map[string]struct{}{
			"TASK01": {"BLOCK1": {}},
			"TASK02": {"BLOCK1": {}},
		},
	}
	graph.rebuildIndexes()

	children := graph.Children("EPIC01")
	if len(children) != 2 || children[0].ID != "TASK01" || children[1].ID != "TASK02" {
		t.Fatalf("children = %#v", children)
	}
	if !graph.IsEpic("EPIC01") {
		t.Fatal("parent with children was not derived as an epic")
	}
	if got := graph.Dependents("BLOCK1"); !equalStrings(got, []string{"TASK01", "TASK02"}) {
		t.Fatalf("dependents = %v", got)
	}
	if got := graph.Dependencies("TASK01"); !equalStrings(got, []string{"BLOCK1"}) {
		t.Fatalf("dependencies = %v", got)
	}
}

func TestGraphReadinessAndBlockersIncludeEpicDependencies(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"EPIC01": {ID: "EPIC01"},
			"TASK01": {ID: "TASK01", EpicID: "EPIC01", State: stateTodo},
			"DIRECT": {ID: "DIRECT", State: stateDone},
			"SHARED": {ID: "SHARED", State: stateTodo},
		},
		Deps: map[string]map[string]struct{}{
			"TASK01": {"DIRECT": {}},
			"EPIC01": {"SHARED": {}},
		},
	}
	graph.rebuildIndexes()

	if graph.IsReady("TASK01") {
		t.Fatal("task with inherited blocker was ready")
	}
	if got := graph.Blockers("TASK01"); !equalStrings(got, []string{"SHARED"}) {
		t.Fatalf("blockers = %v", got)
	}
	graph.Tasks["SHARED"].State = stateDone
	if !graph.IsReady("TASK01") {
		t.Fatal("task did not become ready after every blocker completed")
	}
}

func TestPreparedDerivedQueriesMatchLiveQueries(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"EPIC01": {ID: "EPIC01"},
			"TASK01": {ID: "TASK01", EpicID: "EPIC01", State: stateTodo},
			"TASK02": {ID: "TASK02", EpicID: "EPIC01", State: stateFailed},
			"BLOCK1": {ID: "BLOCK1", State: stateTodo},
		},
		Deps: map[string]map[string]struct{}{
			"TASK01": {"BLOCK1": {}},
		},
	}
	graph.rebuildIndexes()

	wantComplete := graph.IsComplete("EPIC01")
	wantState := graph.EpicState("EPIC01")
	wantBlockers := graph.Blockers("TASK01")
	wantReady := graph.IsReady("TASK01")
	graph.prepareDerivedQueries()

	if graph.IsComplete("EPIC01") != wantComplete || graph.EpicState("EPIC01") != wantState || graph.IsReady("TASK01") != wantReady {
		t.Fatal("prepared scalar queries differ from live queries")
	}
	gotBlockers := graph.Blockers("TASK01")
	if !equalStrings(gotBlockers, wantBlockers) {
		t.Fatalf("prepared blockers = %v, want %v", gotBlockers, wantBlockers)
	}
	gotBlockers[0] = "MUTATED"
	if got := graph.Blockers("TASK01"); !equalStrings(got, wantBlockers) {
		t.Fatalf("caller mutation changed cached blockers: %v", got)
	}
}

func TestGraphPreservesLegacyEmptyEpicExplicitly(t *testing.T) {
	now := time.Now().UTC()
	graph, err := replayEvents([]Event{mustNewEvent("new_epic", now, NewTaskEvent{
		ID: "EPIC01", UUID: "legacy", State: stateTodo, Title: "Empty", CreatedAt: formatTime(now),
	})})
	if err != nil {
		t.Fatal(err)
	}
	if !graph.IsEpic("EPIC01") || len(graph.Children("EPIC01")) != 0 || !graph.IsComplete("EPIC01") {
		t.Fatal("legacy empty epic identity or completion was lost")
	}
}
