// Purpose: Verify placement validation for direct move operations.
// Exports: none.
// Role: Focused unit coverage for promotion, nesting, and ancestry rules.
// Invariants: containers stay at root and dependency edges never cross ancestry.
// Invariants: promotion accepts only clean, unclaimed todo destinations.
package ergo

import (
	"strings"
	"testing"
)

func TestMovePlacementValidation(t *testing.T) {
	base := func() *Graph {
		return &Graph{
			Tasks: map[string]*Task{
				"SOURCE": {ID: "SOURCE", State: stateTodo},
				"DEST01": {ID: "DEST01", State: stateTodo},
			},
			Deps: map[string]map[string]struct{}{},
		}
	}
	tests := []struct {
		name    string
		change  func(*Graph)
		dest    string
		wantErr string
	}{
		{"promote clean root", func(*Graph) {}, "DEST01", ""},
		{"move to root", func(*Graph) {}, "", ""},
		{"self", func(*Graph) {}, "SOURCE", "itself"},
		{"missing", func(*Graph) {}, "ABSENT", "unknown container"},
		{"nested destination", func(g *Graph) { g.Tasks["DEST01"].EpicID = "PARENT" }, "DEST01", "cannot nest"},
		{"claimed destination", func(g *Graph) { g.Tasks["DEST01"].ClaimedBy = "agent" }, "DEST01", "claimed"},
		{"closed destination", func(g *Graph) { g.Tasks["DEST01"].State = stateDone }, "DEST01", "must be todo"},
		{"result destination", func(g *Graph) { g.Tasks["DEST01"].Results = []Result{{Summary: "result"}} }, "DEST01", "results"},
		{"task depends on destination", func(g *Graph) { g.Deps["SOURCE"] = map[string]struct{}{"DEST01": {}} }, "DEST01", "depend on its destination"},
		{"destination depends on task", func(g *Graph) { g.Deps["DEST01"] = map[string]struct{}{"SOURCE": {}} }, "DEST01", "depend on its child"},
		{"moving container", func(g *Graph) { g.Tasks["CHILD1"] = &Task{ID: "CHILD1", EpicID: "SOURCE", State: stateTodo} }, "DEST01", "cannot move container"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := base()
			test.change(graph)
			err := validateMovePlacement(graph, graph.Tasks["SOURCE"], test.dest)
			if test.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
