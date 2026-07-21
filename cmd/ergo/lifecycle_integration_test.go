// Purpose: Exercise direct lifecycle verbs through the compiled CLI.
// Exports: none.
// Role: Black-box coverage for state postconditions, results, and stdin bodies.
// Invariants: every successful lifecycle exit clears the task claim.
// Invariants: validation failures leave the event log unchanged.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleCommandsFromEveryState(t *testing.T) {
	verbs := map[string]string{"done": "done", "block": "blocked", "cancel": "canceled"}
	sources := []string{"todo", "doing", "blocked", "done", "canceled", "error"}
	for verb, target := range verbs {
		for _, source := range sources {
			t.Run(verb+"-from-"+source, func(t *testing.T) {
				dir := setupErgo(t)
				id := createLifecycleTask(t, dir)
				putLifecycleTaskInState(t, dir, id, source)
				stdout, stderr, code := runErgo(t, dir, "", "--json", verb, id)
				if code != 0 {
					t.Fatalf("%s failed: %s", verb, stderr)
				}
				var out struct {
					Kind          string   `json:"kind"`
					ID            string   `json:"id"`
					UpdatedFields []string `json:"updated_fields"`
					State         string   `json:"state"`
					ClaimedBy     string   `json:"claimed_by"`
				}
				if err := json.Unmarshal([]byte(stdout), &out); err != nil {
					t.Fatalf("decode output %q: %v", stdout, err)
				}
				if out.Kind != verb || out.ID != id || out.State != target || out.ClaimedBy != "" {
					t.Fatalf("unexpected output: %+v", out)
				}
			})
		}
	}
}

func TestReleaseLifecycleStates(t *testing.T) {
	for _, source := range []string{"todo", "doing", "blocked", "error"} {
		t.Run(source, func(t *testing.T) {
			dir := setupErgo(t)
			id := createLifecycleTask(t, dir)
			putLifecycleTaskInState(t, dir, id, source)
			stdout, stderr, code := runErgo(t, dir, "", "--json", "release", id)
			if code != 0 {
				t.Fatalf("release failed: %s", stderr)
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatal(err)
			}
			if out["state"] != "todo" {
				t.Fatalf("release state = %v", out["state"])
			}
		})
	}
	for _, source := range []string{"done", "canceled"} {
		t.Run("reject-"+source, func(t *testing.T) {
			dir := setupErgo(t)
			id := createLifecycleTask(t, dir)
			putLifecycleTaskInState(t, dir, id, source)
			_, stderr, code := runErgo(t, dir, "", "release", id)
			if code == 0 || !strings.Contains(stderr, "release cannot apply") {
				t.Fatalf("expected release rejection, code=%d stderr=%q", code, stderr)
			}
		})
	}
}

func TestDoneLifecycleBodyResultAndLateResult(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	resultPath := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(resultPath, []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runErgo(t, dir, "completion body\n", "--json", "done", id,
		"--result", "result.txt", "--summary", "Primary result")
	if code != 0 {
		t.Fatalf("done failed: %s", stderr)
	}
	for _, field := range []string{"body", "result", "state"} {
		if !strings.Contains(stdout, `"`+field+`"`) {
			t.Fatalf("output missing %s: %s", field, stdout)
		}
	}
	latePath := filepath.Join(dir, "late.txt")
	if err := os.WriteFile(latePath, []byte("late"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runErgo(t, dir, "", "--json", "done", id, "--result", "late.txt")
	if code != 0 || !strings.Contains(stdout, `"updated_fields":["result"]`) {
		t.Fatalf("late result failed: stdout=%s stderr=%s", stdout, stderr)
	}
	_, stderr, code = runErgo(t, dir, "", "done", id, "--summary", "missing result")
	if code == 0 || !strings.Contains(stderr, "--summary requires --result") {
		t.Fatalf("expected summary validation error, code=%d stderr=%q", code, stderr)
	}
	shown := showTaskJSON(t, dir, id)
	results, ok := shown["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("show results = %v", shown["results"])
	}
	latest := results[0].(map[string]any)
	if latest["sha256_at_attach"] == "" || !strings.HasPrefix(latest["file_url"].(string), "file://") {
		t.Fatalf("result provenance missing: %v", latest)
	}
}

func TestClaimResumesEverySpecificState(t *testing.T) {
	for _, source := range []string{"todo", "blocked", "done", "canceled", "error"} {
		t.Run(source, func(t *testing.T) {
			dir := setupErgo(t)
			id := createLifecycleTask(t, dir)
			putLifecycleTaskInState(t, dir, id, source)
			agent := "resume@local"
			if source == "error" {
				agent = "test@local"
			}
			stdout, stderr, code := runErgo(t, dir, "", "--json", "claim", id, "--agent", agent)
			if code != 0 {
				t.Fatalf("claim from %s failed: %s", source, stderr)
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatal(err)
			}
			if out["id"] != id || out["state"] != "doing" || out["claimed_by"] != agent {
				t.Fatalf("claim output = %v", out)
			}
		})
	}
}

func TestClaimIsIdempotentForOwnerAndConflictsForOthers(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	_, stderr, code := runErgo(t, dir, "", "claim", id, "--agent", "owner@local")
	if code != 0 {
		t.Fatalf("first claim failed: %s", stderr)
	}
	before := countEventLines(t, dir)
	_, stderr, code = runErgo(t, dir, "", "claim", id, "--agent", "owner@local")
	if code != 0 {
		t.Fatalf("repeat claim failed: %s", stderr)
	}
	if after := countEventLines(t, dir); after != before {
		t.Fatalf("idempotent claim appended events: before=%d after=%d", before, after)
	}
	_, stderr, code = runErgo(t, dir, "", "claim", id, "--agent", "other@local")
	if code == 0 || !strings.Contains(stderr, "already claimed by owner@local") {
		t.Fatalf("expected claim conflict, code=%d stderr=%q", code, stderr)
	}
}

func TestClaimDoneTaskReusesOriginalID(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	_, stderr, code := runErgo(t, dir, "", "done", id)
	if code != 0 {
		t.Fatalf("done failed: %s", stderr)
	}
	stdout, stderr, code := runErgo(t, dir, "", "--json", "claim", id, "--agent", "resume@local")
	if code != 0 {
		t.Fatalf("claim done task failed: %s", stderr)
	}
	if !strings.Contains(stdout, `"id":"`+id+`"`) {
		t.Fatalf("claim returned a different task: %s", stdout)
	}
	list, _, code := runErgo(t, dir, "", "--json", "list", "--all")
	if code != 0 || strings.Count(list, `"id":"`+id+`"`) != 1 {
		t.Fatalf("claim duplicated the task: %s", list)
	}
}

func createLifecycleTask(t *testing.T, dir string) string {
	t.Helper()
	stdout, stderr, code := runNewTask(t, dir, `{"title":"Lifecycle task"}`)
	if code != 0 {
		t.Fatalf("new task failed: %s", stderr)
	}
	return strings.TrimSpace(stdout)
}

func putLifecycleTaskInState(t *testing.T, dir, id, state string) {
	t.Helper()
	switch state {
	case "todo":
		return
	case "doing":
		_, stderr, code := runErgo(t, dir, "", "claim", id, "--agent", "test@local")
		if code != 0 {
			t.Fatalf("claim failed: %s", stderr)
		}
	case "error":
		putLifecycleTaskInState(t, dir, id, "doing")
		_, stderr, code := runSetTask(t, dir, id, `{"state":"error"}`)
		if code != 0 {
			t.Fatalf("set error failed: %s", stderr)
		}
	default:
		_, stderr, code := runSetTask(t, dir, id, `{"state":"`+state+`"}`)
		if code != 0 {
			t.Fatalf("set %s failed: %s", state, stderr)
		}
	}
}
