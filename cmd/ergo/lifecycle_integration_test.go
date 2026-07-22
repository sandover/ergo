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
				stdout, stderr, code := runErgo(t, dir, "", verb, id)
				if code != 0 {
					t.Fatalf("%s failed: %s", verb, stderr)
				}
				if stdout != id+" "+target+"\n" {
					t.Fatalf("unexpected output: %q", stdout)
				}
				shown := showTaskFields(t, dir, id)
				if shown["state"] != target || shown["claimed_by"] != "" {
					t.Fatalf("unexpected postcondition: %v", shown)
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
			stdout, stderr, code := runErgo(t, dir, "", "release", id)
			if code != 0 {
				t.Fatalf("release failed: %s", stderr)
			}
			if stdout != id+" todo\n" {
				t.Fatalf("release output = %q", stdout)
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

func TestLifecycleMessageCardinality(t *testing.T) {
	for _, verb := range []string{"done", "block", "cancel", "release"} {
		for _, messages := range [][]string{nil, {"one note"}, {"first note", "second note"}} {
			name := verb + "-" + string(rune('0'+len(messages)))
			t.Run(name, func(t *testing.T) {
				dir := setupErgo(t)
				id := createLifecycleTask(t, dir)
				args := []string{verb, id}
				for _, message := range messages {
					args = append(args, "-m", message)
				}
				if _, stderr, code := runErgo(t, dir, "", args...); code != 0 {
					t.Fatalf("%s failed: %s", verb, stderr)
				}
				logged := readLifecycleMessages(t, dir, id)
				if len(messages) == 0 {
					if len(logged) != 0 {
						t.Fatalf("unexpected messages: %#v", logged)
					}
					return
				}
				if len(logged) != 1 || logged[0].Kind != verb || logged[0].Text != strings.Join(messages, "\n\n") {
					t.Fatalf("messages = %#v", logged)
				}
			})
		}
	}
}

func TestDoneLifecycleMessagesBodyAndResults(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	if _, stderr, code := runErgo(t, dir, "original body\n", "body", id); code != 0 {
		t.Fatalf("set original body: %s", stderr)
	}
	resultPath := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(resultPath, []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runErgo(t, dir, "", "done", id,
		"--result", "result.txt", "-m", " Primary result ", "-m", "Verified cleanly")
	if code != 0 {
		t.Fatalf("done failed: %s", stderr)
	}
	if stdout != id+" done\n" {
		t.Fatalf("done output = %q", stdout)
	}
	shown := showTaskOutput(t, dir, id)
	if !strings.Contains(shown, "original body\n") {
		t.Fatalf("lifecycle changed body: %s", shown)
	}
	messages := readLifecycleMessages(t, dir, id)
	if len(messages) != 1 || messages[0].Kind != "done" || messages[0].Text != "Primary result\n\nVerified cleanly" {
		t.Fatalf("messages = %#v", messages)
	}
	if strings.Count(shown, "[result.txt](file://") != 1 || strings.Contains(shown, "): result.txt") {
		t.Fatalf("show result missing: %s", shown)
	}

	latePath := filepath.Join(dir, "late.txt")
	if err := os.WriteFile(latePath, []byte("late"), 0644); err != nil {
		t.Fatal(err)
	}
	beforeLate := countEventLines(t, dir)
	stdout, stderr, code = runErgo(t, dir, "", "done", id, "--result", "late.txt", "-m", "Late evidence")
	if code != 0 || stdout != id+" done\n" {
		t.Fatalf("late result failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if got := countEventLines(t, dir); got != beforeLate+2 {
		t.Fatalf("late result/message events = %d, want %d", got, beforeLate+2)
	}
	shown = showTaskOutput(t, dir, id)
	if strings.Count(shown, "(file://") != 2 || !strings.Contains(shown, "[late.txt](file://") {
		t.Fatalf("show results missing: %s", shown)
	}

	beforeInvalid := countEventLines(t, dir)
	for _, test := range []struct {
		name  string
		stdin string
		args  []string
		hint  string
	}{
		{"piped body", "replacement\n", []string{"done", id}, "does not read stdin"},
		{"summary", "", []string{"done", id, "--summary", "caption"}, "use -m"},
		{"blank message", "", []string{"done", id, "-m", "   "}, "cannot be blank"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, stderr, code := runErgo(t, dir, test.stdin, test.args...)
			if code == 0 || !strings.Contains(stderr, test.hint) {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
		})
	}
	_, stderr, code = runErgoWithEmptyPipe(t, dir, "done", id)
	if code == 0 || !strings.Contains(stderr, "does not read stdin") {
		t.Fatalf("empty pipe: code=%d stderr=%q", code, stderr)
	}
	if got := countEventLines(t, dir); got != beforeInvalid {
		t.Fatalf("invalid lifecycle input appended events: before=%d after=%d", beforeInvalid, got)
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
			stdout, stderr, code := runErgo(t, dir, "", "claim", id, "--agent", agent)
			if code != 0 {
				t.Fatalf("claim from %s failed: %s", source, stderr)
			}
			if !strings.Contains(stdout, "id: \""+id+"\"") ||
				!strings.Contains(stdout, "state: \"doing\"") ||
				!strings.Contains(stdout, "claimed_by: \""+agent+"\"") {
				t.Fatalf("claim output = %s", stdout)
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
	stdout, stderr, code := runErgo(t, dir, "", "claim", id, "--agent", "resume@local")
	if code != 0 {
		t.Fatalf("claim done task failed: %s", stderr)
	}
	if !strings.Contains(stdout, "id: \""+id+"\"") {
		t.Fatalf("claim returned a different task: %s", stdout)
	}
	list, _, code := runErgo(t, dir, "", "list", "--all")
	if code != 0 || strings.Count(list, id) != 1 {
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
		_, stderr, code := putTaskInState(t, dir, id, "error", "")
		if code != 0 {
			t.Fatalf("set error failed: %s", stderr)
		}
	default:
		_, stderr, code := putTaskInState(t, dir, id, state, "")
		if code != 0 {
			t.Fatalf("set %s failed: %s", state, stderr)
		}
	}
}

type lifecycleMessageLog struct {
	TaskID string `json:"task_id"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
}

func readLifecycleMessages(t *testing.T, dir, id string) []lifecycleMessageLog {
	t.Helper()
	data, err := os.ReadFile(getEventFilePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var messages []lifecycleMessageLog
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "message" {
			continue
		}
		var message lifecycleMessageLog
		if err := json.Unmarshal(event.Data, &message); err != nil {
			t.Fatal(err)
		}
		if message.TaskID == id {
			messages = append(messages, message)
		}
	}
	return messages
}
