// Purpose: Verify work selection helpers and claim input requirements.
// Exports: none.
// Role: Focused unit coverage for dependency ordering and command guards.
// Invariants: container children retain dependency order with stable ID fallback.
// Invariants: every claim path requires an explicit agent identity.
package ergo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectEpicChildrenSortsByDependencyOrder(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"E1":     {ID: "E1", IsEpic: true, State: stateTodo, Title: "Epic"},
			"A11111": {ID: "A11111", EpicID: "E1", State: stateTodo, Title: "A"},
			"B11111": {ID: "B11111", EpicID: "E1", State: stateTodo, Title: "B"},
			"C11111": {ID: "C11111", EpicID: "E1", State: stateTodo, Title: "C"},
			"D11111": {ID: "D11111", EpicID: "E1", State: stateTodo, Title: "D"},
			"X11111": {ID: "X11111", EpicID: "E2", State: stateTodo, Title: "Other epic"},
		},
		Deps: map[string]map[string]struct{}{
			"A11111": {}, "B11111": {"A11111": {}}, "C11111": {"B11111": {}},
			"D11111": {}, "E1": {}, "X11111": {},
		},
	}
	children := collectEpicChildren("E1", graph)
	if len(children) != 4 {
		t.Fatalf("children = %d, want 4", len(children))
	}
	positions := make(map[string]int, len(children))
	for i, task := range children {
		positions[task.ID] = i
	}
	if !(positions["A11111"] < positions["B11111"] && positions["B11111"] < positions["C11111"]) {
		t.Fatalf("dependency order lost: %v", positions)
	}
	if positions["A11111"] > positions["D11111"] {
		t.Fatalf("stable ID fallback lost: %v", positions)
	}
}

func TestRunClaimRequiresAgent(t *testing.T) {
	err := RunClaim("T1", GlobalOptions{})
	if err == nil || !strings.Contains(err.Error(), "claim requires --agent") {
		t.Fatalf("claim error = %v", err)
	}
}

func TestRunClaimOldestReadyRequiresAgent(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".ergo"), 0755); err != nil {
		t.Fatal(err)
	}
	err := RunClaimOldestReady(GlobalOptions{StartDir: repoDir})
	if err == nil || !strings.Contains(err.Error(), "claim requires --agent") {
		t.Fatalf("claim error = %v", err)
	}
}
