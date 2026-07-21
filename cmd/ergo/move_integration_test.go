// Purpose: Exercise move, promotion, root placement, and rejection paths.
// Exports: none.
// Role: Black-box verification of placement changes through the compiled CLI.
// Invariants: moves are atomic and current-parent moves are event-free no-ops.
// Invariants: a leaf may never turn a nested task into another container level.
package main

import (
	"strings"
	"testing"
)

func TestMovePromotesDestinationAndReturnsToRoot(t *testing.T) {
	dir := setupErgo(t)
	destination := createLifecycleTask(t, dir)
	source := createLifecycleTask(t, dir)
	stdout, stderr, code := runErgo(t, dir, "", "--json", "move", source, destination)
	if code != 0 {
		t.Fatalf("move failed: %s", stderr)
	}
	if !strings.Contains(stdout, `"updated_fields":["epic"]`) {
		t.Fatalf("unexpected move output: %s", stdout)
	}
	if showTaskJSON(t, dir, source)["epic_id"] != destination {
		t.Fatal("source was not moved into destination")
	}
	shownContainer := showTaskJSON(t, dir, destination)
	if _, ok := shownContainer["container"].(map[string]any); !ok {
		t.Fatalf("clean todo destination was not promoted: %v", shownContainer)
	}

	before := countEventLines(t, dir)
	stdout, stderr, code = runErgo(t, dir, "", "--json", "move", source, destination)
	if code != 0 || !strings.Contains(stdout, `"updated_fields":[]`) || countEventLines(t, dir) != before {
		t.Fatalf("same-parent no-op failed: stdout=%s stderr=%s", stdout, stderr)
	}
	_, stderr, code = runErgo(t, dir, "", "move", source, "--root")
	if code != 0 {
		t.Fatalf("move root failed: %s", stderr)
	}
	if showTaskJSON(t, dir, source)["epic_id"] != "" {
		t.Fatal("source did not return to root")
	}
}

func TestMoveRejectsInvalidPlacement(t *testing.T) {
	t.Run("mutually exclusive", func(t *testing.T) {
		dir := setupErgo(t)
		source := createLifecycleTask(t, dir)
		destination := createLifecycleTask(t, dir)
		_, stderr, code := runErgo(t, dir, "", "move", source, destination, "--root")
		if code == 0 || !strings.Contains(stderr, "mutually exclusive") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
	t.Run("container and nested", func(t *testing.T) {
		dir := setupErgo(t)
		container := createLifecycleTask(t, dir)
		child := createLifecycleTask(t, dir)
		other := createLifecycleTask(t, dir)
		if _, stderr, code := runErgo(t, dir, "", "move", child, container); code != 0 {
			t.Fatalf("setup move failed: %s", stderr)
		}
		_, stderr, code := runErgo(t, dir, "", "move", container, other)
		if code == 0 || !strings.Contains(stderr, "cannot move container") {
			t.Fatalf("container move: code=%d stderr=%q", code, stderr)
		}
		_, stderr, code = runErgo(t, dir, "", "move", other, child)
		if code == 0 || !strings.Contains(stderr, "cannot nest") {
			t.Fatalf("nested move: code=%d stderr=%q", code, stderr)
		}
	})
	t.Run("self missing dirty and dependency", func(t *testing.T) {
		dir := setupErgo(t)
		source := createLifecycleTask(t, dir)
		destination := createLifecycleTask(t, dir)
		checks := [][]string{{source, source, "itself"}, {source, "ABSENT", "unknown container"}}
		for _, check := range checks {
			_, stderr, code := runErgo(t, dir, "", "move", check[0], check[1])
			if code == 0 || !strings.Contains(stderr, check[2]) {
				t.Fatalf("move %v: code=%d stderr=%q", check, code, stderr)
			}
		}
		if _, stderr, code := runErgo(t, dir, "", "claim", destination, "--agent", "owner@local"); code != 0 {
			t.Fatalf("claim destination failed: %s", stderr)
		}
		_, stderr, code := runErgo(t, dir, "", "move", source, destination)
		if code == 0 || !strings.Contains(stderr, "claimed") {
			t.Fatalf("dirty destination: code=%d stderr=%q", code, stderr)
		}
		if _, stderr, code := runErgo(t, dir, "", "release", destination); code != 0 {
			t.Fatalf("release destination failed: %s", stderr)
		}
		if _, stderr, code := runErgo(t, dir, "", "sequence", destination, source); code != 0 {
			t.Fatalf("sequence failed: %s", stderr)
		}
		_, stderr, code = runErgo(t, dir, "", "move", source, destination)
		if code == 0 || !strings.Contains(stderr, "depend on its destination") {
			t.Fatalf("ancestry dependency: code=%d stderr=%q", code, stderr)
		}
	})
}
