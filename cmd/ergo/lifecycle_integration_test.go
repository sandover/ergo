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
	t.Parallel()
	verbs := map[string]string{"done": "done", "fail": "failed", "block": "blocked", "cancel": "canceled"}
	sources := []string{"todo", "doing", "blocked", "done", "failed", "canceled", "error"}
	for verb, target := range verbs {
		for _, source := range sources {
			t.Run(verb+"-from-"+source, func(t *testing.T) {
				t.Parallel()
				dir := setupErgo(t)
				id := createLifecycleTask(t, dir)
				putLifecycleTaskInState(t, dir, id, source)
				stdout, stderr, code := runErgo(t, dir, "", verb, id)
				if code != 0 {
					t.Fatalf("%s failed: %s", verb, stderr)
				}
				if !strings.Contains(stdout, id+" - Lifecycle task\n") || !strings.Contains(stdout, "State: "+target+"\n") {
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
	t.Parallel()
	for _, source := range []string{"todo", "doing", "blocked", "error"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			dir := setupErgo(t)
			id := createLifecycleTask(t, dir)
			putLifecycleTaskInState(t, dir, id, source)
			stdout, stderr, code := runErgo(t, dir, "", "release", id)
			if code != 0 {
				t.Fatalf("release failed: %s", stderr)
			}
			if !strings.Contains(stdout, id+" - Lifecycle task\n") || !strings.Contains(stdout, "State: todo\n") {
				t.Fatalf("release output = %q", stdout)
			}
		})
	}
	for _, source := range []string{"done", "failed", "canceled"} {
		t.Run("reject-"+source, func(t *testing.T) {
			t.Parallel()
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

func TestLifecycleReceiptNamesClaimChangesAndNewlyReadyWork(t *testing.T) {
	t.Parallel()
	dir := setupErgo(t)
	first := createLifecycleTask(t, dir)
	stdout, stderr, code := runNewTask(t, dir, "Second task")
	if code != 0 {
		t.Fatalf("create second task: %s", stderr)
	}
	second := strings.TrimSpace(stdout)
	if _, stderr, code = runErgo(t, dir, "", "sequence", first, second); code != 0 {
		t.Fatalf("sequence: %s", stderr)
	}
	if _, stderr, code = runErgo(t, dir, "", "claim", first, "--agent", "model@host"); code != 0 {
		t.Fatalf("claim: %s", stderr)
	}
	stdout, stderr, code = runErgo(t, dir, "", "done", first, "-m", "Verified")
	if code != 0 {
		t.Fatalf("done: %s", stderr)
	}
	for _, fact := range []string{
		first + " - Lifecycle task", "State: done", "Claim: cleared",
		"Message: appended", "Ready: " + second + " - Second task",
	} {
		if !strings.Contains(stdout, fact) {
			t.Fatalf("receipt lacks %q: %s", fact, stdout)
		}
	}
}

func TestLifecycleMessageCardinality(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"done", "fail", "block", "cancel", "release"} {
		for _, messages := range [][]string{nil, {"one note"}, {"first note", "second note"}} {
			name := verb + "-" + string(rune('0'+len(messages)))
			t.Run(name, func(t *testing.T) {
				t.Parallel()
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
	t.Parallel()
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
		"-m", " Primary result ", "-m", "Verified cleanly")
	if code != 0 {
		t.Fatalf("done failed: %s", stderr)
	}
	if !strings.Contains(stdout, id+" - Lifecycle task\n") || !strings.Contains(stdout, "State: done\n") ||
		!strings.Contains(stdout, "Message: appended\n") {
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
	stdout, stderr, code = runErgo(t, dir, "", "result", id, "Primary result", "--file", "result.txt")
	if code != 0 || !strings.Contains(stdout, "File: result.txt") {
		t.Fatalf("result failed: stdout=%s stderr=%s", stdout, stderr)
	}
	shown = showTaskOutput(t, dir, id)
	if strings.Count(shown, "[result.txt](file://") != 1 {
		t.Fatalf("show result missing: %s", shown)
	}

	latePath := filepath.Join(dir, "late.txt")
	if err := os.WriteFile(latePath, []byte("late"), 0644); err != nil {
		t.Fatal(err)
	}
	beforeLate := countEventLines(t, dir)
	stdout, stderr, code = runErgo(t, dir, "", "done", id, "-m", "Late evidence")
	if code != 0 || !strings.Contains(stdout, "State: done\n") || !strings.Contains(stdout, "Message: appended\n") {
		t.Fatalf("late result failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if got := countEventLines(t, dir); got != beforeLate {
		t.Fatalf("journal-only message changed backlog records: before=%d after=%d", beforeLate, got)
	}
	stdout, stderr, code = runErgo(t, dir, "", "result", id, "Late result", "--file", "late.txt")
	if code != 0 || !strings.Contains(stdout, "File: late.txt") {
		t.Fatalf("late result failed: stdout=%s stderr=%s", stdout, stderr)
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
	t.Parallel()
	for _, source := range []string{"todo", "blocked", "done", "failed", "canceled", "error"} {
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

func TestFailedLifecyclePublicJourney(t *testing.T) {
	t.Parallel()
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	if _, stderr, code := runErgo(t, dir, "", "claim", id, "--agent", "failure@local"); code != 0 {
		t.Fatalf("claim: %s", stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "failure.txt"), []byte("failure evidence"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runErgo(t, dir, "", "fail", id, "-m", "Constraint disproved")
	if code != 0 {
		t.Fatalf("fail: %s", stderr)
	}
	for _, fact := range []string{"State: failed", "Claim: cleared", "Message: appended"} {
		if !strings.Contains(stdout, fact) {
			t.Fatalf("receipt lacks %q: %s", fact, stdout)
		}
	}
	stdout, stderr, code = runErgo(t, dir, "", "result", id, "Failure reproduced", "--file", "failure.txt")
	if code != 0 || !strings.Contains(stdout, "Result recorded: Failure reproduced") || !strings.Contains(stdout, "File: failure.txt") {
		t.Fatalf("result receipt: stdout=%q stderr=%q", stdout, stderr)
	}
	shown := showTaskOutput(t, dir, id)
	if !strings.Contains(shown, "state: \"failed\"") || !strings.Contains(shown, "Constraint disproved") || !strings.Contains(shown, "[failure.txt](file://") {
		t.Fatalf("show lacks failed evidence: %s", shown)
	}
	stdout, stderr, code = runErgo(t, dir, "", "list")
	if code != 0 || !strings.Contains(stdout, id) || !strings.Contains(stdout, "✗") {
		t.Fatalf("default list hides failure: stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr, code = runErgo(t, dir, "", "list", "--json")
	if code != 0 || !strings.Contains(stdout, `"id":"`+id+`"`) || !strings.Contains(stdout, `"state":"failed"`) {
		t.Fatalf("JSON list lacks failed state: stdout=%q stderr=%q", stdout, stderr)
	}
	before := countEventLines(t, dir)
	if _, stderr, code = runErgo(t, dir, "", "release", id); code == 0 || !strings.Contains(stderr, "release cannot apply") {
		t.Fatalf("release failed task: code=%d stderr=%q", code, stderr)
	}
	if after := countEventLines(t, dir); after != before {
		t.Fatalf("rejected release wrote events: before=%d after=%d", before, after)
	}
	if _, stderr, code = runErgo(t, dir, "", "claim", id, "--agent", "retry@local"); code != 0 {
		t.Fatalf("retry claim: %s", stderr)
	}
	fields := showTaskFields(t, dir, id)
	if fields["state"] != "doing" || fields["claimed_by"] != "retry@local" {
		t.Fatalf("retry postcondition = %#v", fields)
	}
}

func TestClaimIsIdempotentForOwnerAndConflictsForOthers(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	stdout, stderr, code := runNewTask(t, dir, "Lifecycle task")
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
	data, err := os.ReadFile(filepath.Join(dir, ".ergo", "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var messages []lifecycleMessageLog
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var message lifecycleMessageLog
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatal(err)
		}
		if message.TaskID == id && message.Text != "" && message.Kind != "result" {
			messages = append(messages, message)
		}
	}
	return messages
}
