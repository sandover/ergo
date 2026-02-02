// CLI integration tests for end-to-end command behavior.
// Purpose: validate stdin→validation→events→output wiring across commands.
// Exports: none.
// Role: verifies user-visible behavior including prune/compact semantics.
// Invariants: tests avoid timing assumptions; outputs follow public contracts.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

var ergoBinary string

func TestMain(m *testing.M) {
	// Build binary before running integration tests
	cwd, err := os.Getwd()
	if err != nil {
		os.Stderr.WriteString("failed to get cwd: " + err.Error() + "\n")
		os.Exit(1)
	}
	ergoBinary = filepath.Join(cwd, "ergo-test")

	cmd := exec.Command("go", "build", "-o", ergoBinary, ".")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		os.Stderr.WriteString("failed to build ergo binary: " + err.Error() + "\n")
		os.Exit(1)
	}
	code := m.Run()
	os.Remove(ergoBinary) // cleanup
	os.Exit(code)
}

// runErgo executes the ergo binary with given stdin and args.
func runErgo(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(ergoBinary, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// setupErgo creates a temp directory and initializes .ergo/
func setupErgo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, _, code := runErgo(t, dir, "", "init")
	if code != 0 {
		t.Fatalf("init failed with exit code %d", code)
	}
	return dir
}

func setupErgoWithEventsOnly(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ergoDir := filepath.Join(dir, ".ergo")
	if err := os.MkdirAll(ergoDir, 0755); err != nil {
		t.Fatalf("failed to create .ergo: %v", err)
	}
	eventsPath := filepath.Join(ergoDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create events.jsonl: %v", err)
	}
	return dir
}

func countEventLines(t *testing.T, dir string) int {
	t.Helper()
	path := filepath.Join(dir, ".ergo", "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

func TestNewTask_HappyPath(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	if len(taskID) != 6 {
		t.Errorf("expected 6-char task ID, got %q", taskID)
	}
}

func TestNewTask_BodyStdin_Multiline(t *testing.T) {
	dir := setupErgo(t)
	body := "line1\nline2\n"

	stdout, stderr, code := runErgo(t, dir, body, "new", "task", "--body-stdin", "--title", "Test task")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	taskID := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}
	if task["body"] != body {
		t.Errorf("expected body=%q, got %q", body, task["body"])
	}
}

func TestNewEpic_BodyStdin_Multiline(t *testing.T) {
	dir := setupErgo(t)
	body := "epic line1\nepic line2\n"

	stdout, stderr, code := runErgo(t, dir, body, "new", "epic", "--body-stdin", "--title", "My Epic")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	epicID := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, "", "show", epicID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	var epic map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &epic); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}
	if epic["body"] != body {
		t.Errorf("expected body=%q, got %q", body, epic["body"])
	}
}

func TestNewTask_BodyStdin_ValidationErrors(t *testing.T) {
	dir := setupErgo(t)

	_, stderr, code := runErgo(t, dir, "x", "new", "task", "--body-stdin")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "requires --title") {
		t.Fatalf("expected missing-title error, got stderr=%q", stderr)
	}

	_, stderr, code = runErgo(t, dir, "x", "new", "task", "--body-stdin", "--title", "T", "--body", "inline")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Fatalf("expected mutual exclusion error, got stderr=%q", stderr)
	}

	_, stderr, code = runErgo(t, dir, "x", "new", "task", "--body-stdin", "--title", "T", "--state", "doing")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "state requires claim") {
		t.Fatalf("expected claim invariant error, got stderr=%q", stderr)
	}
}

func TestInit_RepairsMissingLock(t *testing.T) {
	dir := setupErgoWithEventsOnly(t)

	_, _, code := runErgo(t, dir, "", "init")
	if code != 0 {
		t.Fatalf("init failed with exit code %d", code)
	}

	lockPath := filepath.Join(dir, ".ergo", "lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file to exist: %v", err)
	}
}

func TestNewTask_RepairsMissingLock(t *testing.T) {
	dir := setupErgoWithEventsOnly(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("expected exit 0 when lock is missing, got %d", code)
	}

	taskID := strings.TrimSpace(stdout)
	if len(taskID) != 6 {
		t.Errorf("expected 6-char task ID, got %q", taskID)
	}

	lockPath := filepath.Join(dir, ".ergo", "lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file to exist after new: %v", err)
	}
}

func TestNewTask_ValidationError(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runErgo(t, dir, `{}`, "new", "task", "--json")

	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected JSON output, got: %q", stdout)
	}

	if result["error"] != "validation_failed" {
		t.Errorf("expected error=validation_failed, got %v", result["error"])
	}

	// Only title is required now (body is optional)
	missing, ok := result["missing"].([]interface{})
	if !ok || len(missing) != 1 || missing[0] != "title" {
		t.Errorf("expected missing=[title], got %v", result["missing"])
	}
}

func TestSet_StateTransition(t *testing.T) {
	dir := setupErgo(t)

	// Create task
	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set state to done
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	// Verify state via show --json
	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}

	if task["state"] != "done" {
		t.Errorf("expected state=done, got %v", task["state"])
	}
}

func TestSet_BodyStdin_UpdatesBody(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	newBody := "updated\nbody\n"
	_, stderr, code := runErgo(t, dir, newBody, "set", taskID, "--body-stdin", "--state", "done")
	if code != 0 {
		t.Fatalf("set --body-stdin failed: exit %d (stderr=%q)", code, stderr)
	}

	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}
	if task["body"] != newBody {
		t.Errorf("expected body=%q, got %q", newBody, task["body"])
	}
	if task["state"] != "done" {
		t.Errorf("expected state=done, got %v", task["state"])
	}
}

func TestSet_JSONOutput(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID, "--json")
	if code != 0 {
		t.Fatalf("set --json failed: exit %d", code)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse set --json output: %v", err)
	}
	if out["kind"] != "set" {
		t.Errorf("expected kind=set, got %v", out["kind"])
	}
	if out["id"] != taskID {
		t.Errorf("expected id=%s, got %v", taskID, out["id"])
	}
	if out["state"] != "done" {
		t.Errorf("expected state=done, got %v", out["state"])
	}
	fields, ok := out["updated_fields"].([]interface{})
	if !ok || len(fields) == 0 {
		t.Fatalf("expected updated_fields array, got %v", out["updated_fields"])
	}
	foundState := false
	for _, f := range fields {
		if f == "state" {
			foundState = true
		}
	}
	if !foundState {
		t.Errorf("expected updated_fields to include state, got %v", fields)
	}
}

func TestSet_InvalidTransition(t *testing.T) {
	dir := setupErgo(t)

	// Create task
	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set to done
	runErgo(t, dir, `{"state":"done"}`, "set", taskID)

	// Try invalid transition done→doing
	_, stderr, code := runErgo(t, dir, `{"state":"doing","claim":"agent-1"}`, "set", taskID)
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid transition")
	}

	// Error message should mention transition or invalid
	errMsg := strings.ToLower(stderr)
	if !strings.Contains(errMsg, "transition") && !strings.Contains(errMsg, "invalid") {
		t.Errorf("expected error about invalid transition, got: %q", stderr)
	}
}

func TestSequence_JSONOutput(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"title":"Task B"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, "", "sequence", taskB, taskA, "--json")
	if code != 0 {
		t.Fatalf("sequence --json failed: exit %d", code)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse sequence --json output: %v", err)
	}
	if out["kind"] != "sequence" || out["action"] != "link" {
		t.Errorf("expected kind=sequence action=link, got %v", out)
	}
	edges, ok := out["edges"].([]interface{})
	if !ok || len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %v", out["edges"])
	}
	edge, ok := edges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected edge object, got %v", edges[0])
	}
	if edge["from_id"] != taskA || edge["to_id"] != taskB {
		t.Errorf("unexpected edge ids: %v", edge)
	}
	if edge["type"] != "depends" {
		t.Errorf("expected type=depends, got %v", edge["type"])
	}

	stdout, _, code = runErgo(t, dir, "", "sequence", "rm", taskB, taskA, "--json")
	if code != 0 {
		t.Fatalf("sequence rm --json failed: exit %d", code)
	}
	out = map[string]interface{}{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse sequence rm --json output: %v", err)
	}
	if out["kind"] != "sequence" || out["action"] != "unlink" {
		t.Errorf("expected kind=sequence action=unlink, got %v", out)
	}
	edges, ok = out["edges"].([]interface{})
	if !ok || len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %v", out["edges"])
	}
	edge, ok = edges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected edge object, got %v", edges[0])
	}
	if edge["from_id"] != taskA || edge["to_id"] != taskB {
		t.Errorf("unexpected edge ids: %v", edge)
	}
}

func TestSequence_ChainOrder_Readiness(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"title":"Task B"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"title":"Task C"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskC := strings.TrimSpace(stdout)

	_, _, code = runErgo(t, dir, "", "sequence", taskA, taskB, taskC)
	if code != 0 {
		t.Fatalf("sequence chain failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "list", "--ready", "--json")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	var ready []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &ready); err != nil {
		t.Fatalf("failed to parse list --ready output: %v", err)
	}
	if len(ready) != 1 || ready[0]["id"] != taskA {
		t.Fatalf("expected only Task A ready, got %v", ready)
	}

	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskA)
	if code != 0 {
		t.Fatalf("set taskA done failed: exit %d", code)
	}
	stdout, _, code = runErgo(t, dir, "", "list", "--ready", "--json")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &ready); err != nil {
		t.Fatalf("failed to parse list --ready output: %v", err)
	}
	if len(ready) != 1 || ready[0]["id"] != taskB {
		t.Fatalf("expected only Task B ready, got %v", ready)
	}

	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskB)
	if code != 0 {
		t.Fatalf("set taskB done failed: exit %d", code)
	}
	stdout, _, code = runErgo(t, dir, "", "list", "--ready", "--json")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &ready); err != nil {
		t.Fatalf("failed to parse list --ready output: %v", err)
	}
	if len(ready) != 1 || ready[0]["id"] != taskC {
		t.Fatalf("expected only Task C ready, got %v", ready)
	}
}

func TestPrune_DefaultIsDryRun(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	before := countEventLines(t, dir)
	stdout, _, code = runErgo(t, dir, "", "prune")
	if code != 0 {
		t.Fatalf("prune dry-run failed: exit %d", code)
	}
	after := countEventLines(t, dir)
	if before != after {
		t.Fatalf("expected dry-run to avoid writes (lines %d -> %d)", before, after)
	}
	if !strings.Contains(stdout, "preview") || !strings.Contains(stdout, "To apply: ergo prune --yes") {
		t.Fatalf("expected preview output to explain how to apply, got: %q", stdout)
	}

	_, _, code = runErgo(t, dir, "", "show", taskID)
	if code != 0 {
		t.Fatalf("expected task to remain after dry-run")
	}
}

func TestPrune_JSONDryRun(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "prune", "--json")
	if code != 0 {
		t.Fatalf("prune --json dry-run failed: exit %d", code)
	}
	var out struct {
		Kind      string   `json:"kind"`
		DryRun    bool     `json:"dry_run"`
		PrunedIDs []string `json:"pruned_ids"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse prune json: %v", err)
	}
	if out.Kind != "prune" || !out.DryRun {
		t.Fatalf("expected kind=prune dry_run=true, got %+v", out)
	}
	if len(out.PrunedIDs) != 1 || out.PrunedIDs[0] != taskID {
		t.Fatalf("expected pruned_ids to include %s, got %v", taskID, out.PrunedIDs)
	}
}

func TestPrune_YesWrites(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	_, _, code = runErgo(t, dir, "", "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune --yes failed: exit %d", code)
	}

	_, _, code = runErgo(t, dir, "", "show", taskID)
	if code == 0 {
		t.Fatalf("expected pruned task to be gone after prune --yes")
	}
}

func TestPrune_RemovesDepsAndErrorsOnPrunedIDs(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)
	stdout, _, code = runErgo(t, dir, `{"title":"Task B"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	_, _, code = runErgo(t, dir, "", "sequence", taskB, taskA)
	if code != 0 {
		t.Fatalf("sequence failed: exit %d", code)
	}
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskB)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "show", taskA, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	var before map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &before); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}
	if deps, ok := before["deps"].([]interface{}); !ok || len(deps) != 1 || deps[0] != taskB {
		t.Fatalf("expected deps to include %s, got %v", taskB, before["deps"])
	}

	_, _, code = runErgo(t, dir, "", "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune --yes failed: exit %d", code)
	}

	_, stderr, code := runErgo(t, dir, "", "show", taskB)
	if code == 0 || !strings.Contains(stderr, "pruned") {
		t.Fatalf("expected show to fail with pruned error, got code=%d stderr=%q", code, stderr)
	}

	stdout, _, code = runErgo(t, dir, "", "show", taskA, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	var after map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &after); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}
	if deps, ok := after["deps"]; ok {
		if deps == nil {
			return
		}
		if list, ok := deps.([]interface{}); ok {
			if len(list) != 0 {
				t.Fatalf("expected deps to be empty after prune, got %v", deps)
			}
		} else {
			t.Fatalf("expected deps to be empty after prune, got %v", deps)
		}
	} else {
		t.Fatalf("expected deps field after prune")
	}
}

func TestPrune_PrunesEmptyEpicsAndPreservesActiveTasks(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Epic 1"}`, "new", "epic")
	if code != 0 {
		t.Fatalf("new epic failed: exit %d", code)
	}
	epic1 := strings.TrimSpace(stdout)
	stdout, _, code = runErgo(t, dir, `{"title":"Epic 2"}`, "new", "epic")
	if code != 0 {
		t.Fatalf("new epic failed: exit %d", code)
	}
	epic2 := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"title":"Task Done","epic":"`+epic1+`"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskDone := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskDone)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, `{"title":"Task Active","epic":"`+epic2+`"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskActive := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"title":"Blocked"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskBlocked := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"blocked"}`, "set", taskBlocked)
	if code != 0 {
		t.Fatalf("set state=blocked failed: exit %d", code)
	}

	_, _, code = runErgo(t, dir, "", "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune --yes failed: exit %d", code)
	}

	_, stderr, code := runErgo(t, dir, "", "show", epic1)
	if code == 0 || !strings.Contains(stderr, "pruned") {
		t.Fatalf("expected epic1 to be pruned, got code=%d stderr=%q", code, stderr)
	}
	_, _, code = runErgo(t, dir, "", "show", epic2)
	if code != 0 {
		t.Fatalf("expected epic2 to remain")
	}

	_, stderr, code = runErgo(t, dir, "", "show", taskDone)
	if code == 0 || !strings.Contains(stderr, "pruned") {
		t.Fatalf("expected taskDone to be pruned, got code=%d stderr=%q", code, stderr)
	}
	_, _, code = runErgo(t, dir, "", "show", taskActive)
	if code != 0 {
		t.Fatalf("expected taskActive to remain")
	}
	_, _, code = runErgo(t, dir, "", "show", taskBlocked)
	if code != 0 {
		t.Fatalf("expected taskBlocked to remain")
	}
}

func TestPrune_CompactRemovesHistory(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}
	_, _, code = runErgo(t, dir, "", "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune --yes failed: exit %d", code)
	}

	_, stderr, code := runErgo(t, dir, "", "show", taskID)
	if code == 0 || !strings.Contains(stderr, "pruned") {
		t.Fatalf("expected pre-compact pruned error, got code=%d stderr=%q", code, stderr)
	}

	_, _, code = runErgo(t, dir, "", "compact")
	if code != 0 {
		t.Fatalf("compact failed: exit %d", code)
	}

	_, stderr, code = runErgo(t, dir, "", "show", taskID)
	if code == 0 || strings.Contains(stderr, "pruned") {
		t.Fatalf("expected post-compact not-found error, got code=%d stderr=%q", code, stderr)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".ergo", "events.jsonl"))
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}
	if strings.Contains(string(data), "tombstone") || strings.Contains(string(data), taskID) {
		t.Fatalf("expected compacted log to remove pruned history, got: %s", string(data))
	}
}

func TestCompact_JSONOutput(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, "", "compact", "--json")
	if code != 0 {
		t.Fatalf("compact --json failed: exit %d", code)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse compact --json output: %v", err)
	}
	if out["kind"] != "compact" || out["status"] != "ok" {
		t.Errorf("unexpected compact output: %v", out)
	}
}

func TestPrune_LockBusy(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	lockPath := filepath.Join(dir, ".ergo", "lock")
	lockFile, err := os.OpenFile(lockPath, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer func() {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
			t.Errorf("failed to release lock: %v", err)
		}
	}()

	before := countEventLines(t, dir)
	_, stderr, code := runErgo(t, dir, "", "prune", "--yes")
	if code == 0 || !strings.Contains(stderr, "lock busy") {
		t.Fatalf("expected lock busy error, got code=%d stderr=%q", code, stderr)
	}
	after := countEventLines(t, dir)
	if before != after {
		t.Fatalf("expected no writes on lock busy (lines %d -> %d)", before, after)
	}
}

func TestPrune_ConcurrentRuns(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Task A"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	type result struct {
		stdout string
		stderr string
		code   int
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			out, errOut, exit := runErgo(t, dir, "", "prune", "--yes", "--json")
			results <- result{stdout: out, stderr: errOut, code: exit}
		}()
	}
	r1 := <-results
	r2 := <-results

	if r1.code != 0 && r2.code != 0 {
		t.Fatalf("expected at least one prune to succeed, got codes %d and %d", r1.code, r2.code)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".ergo", "events.jsonl"))
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}
	tombstones := strings.Count(string(data), "tombstone")
	if tombstones != 1 {
		t.Fatalf("expected exactly one tombstone event, got %d (stdout1=%q stderr1=%q stdout2=%q stderr2=%q)", tombstones, r1.stdout, r1.stderr, r2.stdout, r2.stderr)
	}
}

func TestCreateAndClaim_Atomic(t *testing.T) {
	dir := setupErgo(t)

	// Create task with state=doing and claim in one operation
	stdout, _, code := runErgo(t, dir,
		`{"title":"Urgent task","body":"Urgent task","state":"doing","claim":"agent-1"}`,
		"new", "task", "--json")

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Parse created task
	var created map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	taskID, ok := created["id"].(string)
	if !ok || len(taskID) != 6 {
		t.Fatalf("expected 6-char task ID, got %v", created["id"])
	}

	// Verify via show
	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}

	if task["state"] != "doing" {
		t.Errorf("expected state=doing, got %v", task["state"])
	}
	if task["claimed_by"] != "agent-1" {
		t.Errorf("expected claimed_by=agent-1, got %v", task["claimed_by"])
	}
}

func TestCompact_PreservesShowJSON(t *testing.T) {
	dir := setupErgo(t)

	// Create an epic
	stdout, _, code := runErgo(t, dir, `{"title":"Epic","body":"Epic"}`, "new", "epic")
	if code != 0 {
		t.Fatalf("new epic failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	// Create two tasks in the epic
	stdout, _, code = runErgo(t, dir, `{"title":"T1","body":"T1","epic":"`+epicID+`"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task T1 failed: exit %d", code)
	}
	t1 := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, `{"title":"T2","body":"T2","epic":"`+epicID+`"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task T2 failed: exit %d", code)
	}
	t2 := strings.TrimSpace(stdout)

	// Add dependency T2 depends on T1.
	_, _, code = runErgo(t, dir, "", "sequence", t1, t2)
	if code != 0 {
		t.Fatalf("sequence %s %s failed: exit %d", t1, t2, code)
	}

	// Mutate T1 across multiple dimensions.
	_, stderr, code := runErgo(t, dir, `{"claim":"agent-1","state":"doing","body":"T1\\n\\n## v2\\nmore"}`, "set", t1)
	if code != 0 {
		t.Fatalf("set %s failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = runErgo(t, dir, `{"state":"error","claim":"agent-1"}`, "set", t1)
	if code != 0 {
		t.Fatalf("set %s state=error failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = runErgo(t, dir, `{"state":"doing","claim":"agent-1"}`, "set", t1)
	if code != 0 {
		t.Fatalf("set %s state=doing failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = runErgo(t, dir, `{"state":"done"}`, "set", t1)
	if code != 0 {
		t.Fatalf("set %s state=done failed: exit %d stderr=%q", t1, code, stderr)
	}

	// Attach a result to T1 (ensures evidence fields survive compaction).
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "r1.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write result file failed: %v", err)
	}
	_, _, code = runErgo(t, dir, `{"result_path":"docs/r1.md","result_summary":"first result"}`, "set", t1)
	if code != 0 {
		t.Fatalf("attach result failed: exit %d", code)
	}

	show := func(id string) map[string]interface{} {
		t.Helper()
		stdout, _, code := runErgo(t, dir, "", "show", id, "--json")
		if code != 0 {
			t.Fatalf("show %s failed: exit %d", id, code)
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("parse show %s output failed: %v", id, err)
		}
		return out
	}

	beforeT1 := show(t1)
	beforeT2 := show(t2)

	_, _, code = runErgo(t, dir, "", "compact")
	if code != 0 {
		t.Fatalf("compact failed: exit %d", code)
	}

	afterT1 := show(t1)
	afterT2 := show(t2)

	if !reflect.DeepEqual(beforeT1, afterT1) {
		t.Fatalf("show --json changed for %s after compact", t1)
	}
	if !reflect.DeepEqual(beforeT2, afterT2) {
		t.Fatalf("show --json changed for %s after compact", t2)
	}
}

func TestNewEpic_HappyPath(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runErgo(t, dir, `{"title":"Test Epic","body":"Test Epic"}`, "new", "epic")

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	epicID := strings.TrimSpace(stdout)
	if len(epicID) != 6 {
		t.Errorf("expected 6-char epic ID, got %q", epicID)
	}
}

func TestSet_MultipleFields(t *testing.T) {
	dir := setupErgo(t)

	// Create task
	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Update multiple fields in one call
	_, _, code = runErgo(t, dir,
		`{"title":"Updated title","state":"doing","claim":"agent-1"}`,
		"set", taskID)
	if code != 0 {
		t.Fatalf("set failed: exit %d", code)
	}

	// Verify all fields updated
	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}

	if task["title"] != "Updated title" {
		t.Errorf("expected title updated, got %v", task["title"])
	}
	if task["state"] != "doing" {
		t.Errorf("expected state=doing, got %v", task["state"])
	}
	if task["claimed_by"] != "agent-1" {
		t.Errorf("expected claimed_by=agent-1, got %v", task["claimed_by"])
	}
}

func TestClaim_WithAgentFlag(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, `{"title":"Test task","body":"Test task"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	agentID := "sonnet@agent-host"
	_, _, code = runErgo(t, dir, "", "claim", taskID, "--agent", agentID)
	if code != 0 {
		t.Fatalf("claim failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}

	if task["claimed_by"] != agentID {
		t.Errorf("expected claimed_by=%q, got %v", agentID, task["claimed_by"])
	}
	if task["state"] != "doing" {
		t.Errorf("expected state=doing, got %v", task["state"])
	}
}

// TestTitleAndBodyStoredCorrectly verifies that title and body are stored separately.
func TestTitleAndBodyStoredCorrectly(t *testing.T) {
	dir := setupErgo(t)

	// Create task with distinct title and body
	stdout, _, code := runErgo(t, dir,
		`{"title":"My Important Task","body":"This is the detailed body text"}`,
		"new", "task")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Verify via show --json that title and body are distinct
	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}

	if task["title"] != "My Important Task" {
		t.Errorf("expected title %q, got %v", "My Important Task", task["title"])
	}
	body, ok := task["body"].(string)
	if !ok {
		t.Fatalf("expected body string, got %T", task["body"])
	}
	if body != "This is the detailed body text" {
		t.Errorf("expected body %q, got %q", "This is the detailed body text", body)
	}
}

// TestSetOutputsTaskID verifies that 'ergo set' prints the task ID on success.
func TestSetOutputsTaskID(t *testing.T) {
	dir := setupErgo(t)

	// Create a task
	stdout, _, code := runErgo(t, dir, `{"title":"Test","body":"Test body"}`, "new", "task")
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set state and verify output
	stdout, _, code = runErgo(t, dir, `{"state":"done"}`, "set", taskID)
	if code != 0 {
		t.Fatalf("set failed: exit %d", code)
	}

	output := strings.TrimSpace(stdout)
	if output != taskID {
		t.Errorf("expected set to output %q, got %q", taskID, output)
	}
}

// TestSetRejectsEpicState verifies that epics cannot have state/claim set.
func TestSetRejectsEpicState(t *testing.T) {
	dir := setupErgo(t)

	// Create an epic
	stdout, _, code := runErgo(t, dir, `{"title":"Test Epic"}`, "new", "epic")
	if code != 0 {
		t.Fatalf("new epic failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"state rejected", `{"state":"done"}`, "epics do not have state"},
		{"claim rejected", `{"claim":"agent-1"}`, "epics cannot be claimed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runErgo(t, dir, tt.input, "set", epicID)
			if code == 0 {
				t.Errorf("expected error, got success")
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tt.wantErr, stderr)
			}
		})
	}
}

// TestListJSONIncludesAllTasks verifies JSON output includes all tasks (no filtering).
func TestListJSONIncludesAllTasks(t *testing.T) {
	dir := setupErgo(t)

	// Create an epic with tasks in various states
	stdout, _, _ := runErgo(t, dir, `{"title":"Test Epic"}`, "new", "epic")
	epicID := strings.TrimSpace(stdout)

	// Create tasks: one done, one canceled, one todo
	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"Done task","epic":"%s"}`, epicID), "new", "task")
	doneID := strings.TrimSpace(stdout)
	runErgo(t, dir, `{"state":"done"}`, "set", doneID)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"Canceled task","epic":"%s"}`, epicID), "new", "task")
	canceledID := strings.TrimSpace(stdout)
	runErgo(t, dir, `{"state":"canceled"}`, "set", canceledID)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"Todo task","epic":"%s"}`, epicID), "new", "task")
	todoID := strings.TrimSpace(stdout)

	// List with JSON format and --all - should include ALL tasks
	stdout, _, code := runErgo(t, dir, "", "list", "--json", "--all")
	if code != 0 {
		t.Fatalf("list --json --all failed: exit %d", code)
	}

	var tasks []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Should have 3 tasks
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks in JSON, got %d", len(tasks))
	}

	// Verify all task IDs present
	ids := make(map[string]bool)
	for _, task := range tasks {
		ids[task["id"].(string)] = true
	}

	if !ids[doneID] {
		t.Error("done task missing from JSON output")
	}
	if !ids[canceledID] {
		t.Error("canceled task missing from JSON output")
	}
	if !ids[todoID] {
		t.Error("todo task missing from JSON output")
	}

	// Global --json should work before the command as well
	stdout, _, code = runErgo(t, dir, "", "--json", "list", "--all")
	if code != 0 {
		t.Fatalf("--json list --all failed: exit %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("failed to parse JSON with global --json: %v", err)
	}
}

func TestListJSONReadyFilters(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Ready task"}`, "new", "task")
	readyID := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"Done task"}`, "new", "task")
	doneID := strings.TrimSpace(stdout)
	runErgo(t, dir, `{"state":"done"}`, "set", doneID)

	stdout, _, _ = runErgo(t, dir, `{"title":"Blocked task"}`, "new", "task")
	blockedID := strings.TrimSpace(stdout)
	runErgo(t, dir, `{"state":"blocked"}`, "set", blockedID)

	stdout, _, code := runErgo(t, dir, "", "list", "--json", "--ready")
	if code != 0 {
		t.Fatalf("list --json --ready failed: exit %d", code)
	}

	var tasks []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	ids := make(map[string]bool)
	for _, task := range tasks {
		ids[task["id"].(string)] = true
	}

	if !ids[readyID] {
		t.Errorf("expected ready task %s in JSON output", readyID)
	}
	if ids[doneID] {
		t.Errorf("did not expect done task %s in JSON output", doneID)
	}
	if ids[blockedID] {
		t.Errorf("did not expect blocked task %s in JSON output", blockedID)
	}
}

func TestListJSONEpicFilters(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Epic A"}`, "new", "epic")
	epicA := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"Epic B"}`, "new", "epic")
	epicB := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"A1","epic":"%s"}`, epicA), "new", "task")
	taskA1 := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"B1","epic":"%s"}`, epicB), "new", "task")
	taskB1 := strings.TrimSpace(stdout)

	stdout, _, code := runErgo(t, dir, "", "list", "--json", "--epic", epicA)
	if code != 0 {
		t.Fatalf("list --json --epic failed: exit %d", code)
	}

	var tasks []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	ids := make(map[string]bool)
	for _, task := range tasks {
		ids[task["id"].(string)] = true
	}

	if !ids[taskA1] {
		t.Errorf("expected epic A task %s in JSON output", taskA1)
	}
	if ids[taskB1] {
		t.Errorf("did not expect epic B task %s in JSON output", taskB1)
	}
}

func TestListJSONEpicsOnly(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Epic A"}`, "new", "epic")
	epicA := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"Epic B"}`, "new", "epic")
	epicB := strings.TrimSpace(stdout)

	stdout, _, code := runErgo(t, dir, "", "list", "--json", "--epics")
	if code != 0 {
		t.Fatalf("list --json --epics failed: exit %d", code)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	ids := make(map[string]bool)
	for _, item := range items {
		ids[item["id"].(string)] = true
		if item["title"] == "" {
			t.Errorf("expected epic title to be present in JSON output")
		}
		if kind, ok := item["kind"]; ok && kind != "epic" {
			t.Errorf("expected kind=epic, got: %v", kind)
		}
	}

	if !ids[epicA] || !ids[epicB] {
		t.Errorf("expected both epics in JSON output, got ids: %v", ids)
	}
}

func TestListJSONConflictingFlags(t *testing.T) {
	dir := setupErgo(t)

	_, stderr, code := runErgo(t, dir, "", "list", "--json", "--ready", "--all")
	if code == 0 {
		t.Fatalf("expected error for conflicting --ready and --all with --json")
	}
	if !strings.Contains(stderr, "conflicting flags: --ready and --all") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}
}

func TestListNoTasksEmptyState(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, "", "list")
	if code != 0 {
		t.Fatalf("list failed: exit %d", code)
	}
	if !strings.Contains(stdout, "No tasks.") {
		t.Errorf("expected no tasks message, got: %s", stdout)
	}
	if strings.Contains(stdout, "ready") || strings.Contains(stdout, "blocked") || strings.Contains(stdout, "done") {
		t.Errorf("expected no summary when no tasks, got: %s", stdout)
	}
}

func TestListReadyBlockedByDepsCountsAsBlocked(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Blocker"}`, "new", "task")
	blockerID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", blockerID, "--agent", "test@local")

	stdout, _, _ = runErgo(t, dir, `{"title":"Blocked by dependency"}`, "new", "task")
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "sequence", blockerID, blockedID)

	stdout, _, code := runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if !strings.Contains(stdout, "No ready tasks.") {
		t.Fatalf("expected no ready tasks message, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 in progress") {
		t.Errorf("expected in progress count for blocker, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 blocked") {
		t.Errorf("expected blocked count for unmet deps, got: %s", stdout)
	}
}

func TestListSummaryIncludesErrorBucket(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Error task"}`, "new", "task")
	errorID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", errorID, "--agent", "test@local")
	_, _, _ = runErgo(t, dir, `{"state":"error"}`, "set", errorID)

	stdout, _, code := runErgo(t, dir, "", "list")
	if code != 0 {
		t.Fatalf("list failed: exit %d", code)
	}
	if !strings.Contains(stdout, "1 error") {
		t.Errorf("expected error bucket in summary, got: %s", stdout)
	}
}

func TestListEpicDoneTasksNotHidden(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Epic"}`, "new", "epic")
	epicID := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"Done","epic":"%s"}`, epicID), "new", "task")
	doneID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"done"}`, "set", doneID)

	stdout, _, code := runErgo(t, dir, "", "list", "--epic", epicID)
	if code != 0 {
		t.Fatalf("list --epic failed: exit %d", code)
	}
	if !strings.Contains(stdout, doneID) {
		t.Errorf("expected done task %s in epic output, got: %s", doneID, stdout)
	}
	if strings.Contains(stdout, "No tasks in this epic.") {
		t.Errorf("did not expect empty message when epic has tasks, got: %s", stdout)
	}
}

func TestListEpicsNoEpicsOnlyMessage(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, "", "list", "--epics")
	if code != 0 {
		t.Fatalf("list --epics failed: exit %d", code)
	}
	if strings.TrimSpace(stdout) != "No epics." {
		t.Errorf("expected only no epics message, got: %s", stdout)
	}
}

func TestListJSONEpicsEmptyArray(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, "", "list", "--json", "--epics")
	if code != 0 {
		t.Fatalf("list --json --epics failed: exit %d", code)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty array for no epics, got %d", len(items))
	}
}

func TestListJSONEpicInvalidReturnsEmpty(t *testing.T) {
	dir := setupErgo(t)

	stdout, stderr, code := runErgo(t, dir, "", "list", "--json", "--epic", "ZZZZZZ")
	if code != 0 {
		t.Fatalf("expected success for invalid epic ID in JSON mode, got exit %d", code)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("expected no stderr output, got: %s", stderr)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty array for invalid epic ID, got %d", len(items))
	}
}

// TestListReadyExcludesCompletedTasks verifies --ready hides done/canceled tasks in human output.
func TestListReadyExcludesCompletedTasks(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Ready task"}`, "new", "task")
	readyID := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"Done task"}`, "new", "task")
	doneID := strings.TrimSpace(stdout)
	runErgo(t, dir, `{"state":"done"}`, "set", doneID)

	stdout, _, code := runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}

	if !strings.Contains(stdout, readyID) {
		t.Errorf("expected ready task %s in output", readyID)
	}
	if strings.Contains(stdout, doneID) {
		t.Errorf("did not expect done task %s in output", doneID)
	}
}

func TestListEpicFilterHuman(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Epic A"}`, "new", "epic")
	epicA := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"Epic B"}`, "new", "epic")
	epicB := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"A1","epic":"%s"}`, epicA), "new", "task")
	taskA1 := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"A2","epic":"%s"}`, epicA), "new", "task")
	taskA2 := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"done"}`, "set", taskA2)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"B1","epic":"%s"}`, epicB), "new", "task")
	taskB1 := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"Orphan"}`, "new", "task")
	orphan := strings.TrimSpace(stdout)

	stdout, stderr, code := runErgo(t, dir, "", "list", "--epic", epicA)
	if code != 0 {
		t.Fatalf("list --epic failed: exit %d", code)
	}
	if !strings.Contains(stdout, epicA) {
		t.Errorf("expected epic %s in output", epicA)
	}
	if !strings.Contains(stdout, taskA1) || !strings.Contains(stdout, taskA2) {
		t.Errorf("expected epic A tasks in output")
	}
	if strings.Contains(stdout, epicB) || strings.Contains(stdout, taskB1) {
		t.Errorf("did not expect epic B in output")
	}
	if strings.Contains(stdout, orphan) {
		t.Errorf("did not expect orphan task in output")
	}
	if !strings.Contains(stdout, taskA2) {
		t.Errorf("expected done epic task %s in output (epic-focused view shows done by default)", taskA2)
	}
	if strings.Contains(stdout, "No tasks in this epic.") {
		t.Errorf("did not expect 'No tasks in this epic.' when epic has tasks")
	}
	if !strings.Contains(stderr, "agents: use 'ergo --json list'") {
		t.Errorf("expected agents hint in stderr, got: %s", stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "list", "--epic", "ZZZZZZ")
	if code == 0 {
		t.Fatalf("expected error for invalid epic ID")
	}
	if !strings.Contains(stderr, "no such epic: ZZZZZZ") {
		t.Errorf("expected invalid epic error, got: %s", stderr)
	}
}

func TestListConflictingFlags(t *testing.T) {
	dir := setupErgo(t)

	_, stderr, code := runErgo(t, dir, "", "list", "--ready", "--all")
	if code == 0 {
		t.Fatalf("expected error for conflicting --ready and --all")
	}
	if !strings.Contains(stderr, "conflicting flags: --ready and --all") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "list", "--epics", "--ready")
	if code == 0 {
		t.Fatalf("expected error for conflicting --epics and --ready")
	}
	if !strings.Contains(stderr, "conflicting flags: --epics and --ready") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "list", "--epics", "--all")
	if code == 0 {
		t.Fatalf("expected error for conflicting --epics and --all")
	}
	if !strings.Contains(stderr, "conflicting flags: --epics and --all") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "list", "--epics", "--epic", "ABCDEF")
	if code == 0 {
		t.Fatalf("expected error for conflicting --epics and --epic")
	}
	if !strings.Contains(stderr, "conflicting flags: --epics and --epic") {
		t.Errorf("expected conflict error, got: %s", stderr)
	}
}

func TestListReadyEmptyStateWithContext(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Doing task"}`, "new", "task")
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runErgo(t, dir, `{"title":"Blocked task"}`, "new", "task")
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"blocked"}`, "set", blockedID)

	stdout, _, code := runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if !strings.Contains(stdout, "No ready tasks.") {
		t.Fatalf("expected empty ready message, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 in progress") {
		t.Errorf("expected contextual in progress count, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 blocked") {
		t.Errorf("expected contextual blocked count, got: %s", stdout)
	}
}

func TestListNoActiveTasksSummary(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Done task"}`, "new", "task")
	doneID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"done"}`, "set", doneID)

	stdout, _, _ = runErgo(t, dir, `{"title":"Canceled task"}`, "new", "task")
	canceledID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"canceled"}`, "set", canceledID)

	stdout, _, code := runErgo(t, dir, "", "list")
	if code != 0 {
		t.Fatalf("list failed: exit %d", code)
	}
	if !strings.Contains(stdout, "No active tasks.") {
		t.Fatalf("expected no active tasks message, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 done") || !strings.Contains(stdout, "1 canceled") {
		t.Errorf("expected done/canceled summary, got: %s", stdout)
	}
}

func TestListAllSummaryIncludesTerminalStates(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Ready task"}`, "new", "task")
	_ = strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, `{"title":"In progress task"}`, "new", "task")
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runErgo(t, dir, `{"title":"Blocked task"}`, "new", "task")
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"blocked"}`, "set", blockedID)

	stdout, _, _ = runErgo(t, dir, `{"title":"Error task"}`, "new", "task")
	errorID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", errorID, "--agent", "test@local")
	_, _, _ = runErgo(t, dir, `{"state":"error"}`, "set", errorID)

	stdout, _, _ = runErgo(t, dir, `{"title":"Done task"}`, "new", "task")
	doneID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"done"}`, "set", doneID)

	stdout, _, _ = runErgo(t, dir, `{"title":"Canceled task"}`, "new", "task")
	canceledID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"canceled"}`, "set", canceledID)

	stdout, _, code := runErgo(t, dir, "", "list", "--all")
	if code != 0 {
		t.Fatalf("list --all failed: exit %d", code)
	}
	for _, needle := range []string{"1 ready", "1 in progress", "1 blocked", "1 error", "1 done", "1 canceled"} {
		if !strings.Contains(stdout, needle) {
			t.Errorf("expected summary to include %q, got: %s", needle, stdout)
		}
	}
}

func TestListEpicReadyEmptyState(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Epic"}`, "new", "epic")
	epicID := strings.TrimSpace(stdout)

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"Doing","epic":"%s"}`, epicID), "new", "task")
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runErgo(t, dir, fmt.Sprintf(`{"title":"Blocked","epic":"%s"}`, epicID), "new", "task")
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"blocked"}`, "set", blockedID)

	stdout, _, code := runErgo(t, dir, "", "list", "--epic", epicID, "--ready")
	if code != 0 {
		t.Fatalf("list --epic --ready failed: exit %d", code)
	}
	if !strings.Contains(stdout, epicID) {
		t.Errorf("expected epic header in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "No ready tasks in this epic.") {
		t.Errorf("expected epic ready empty message, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 in progress") || !strings.Contains(stdout, "1 blocked") {
		t.Errorf("expected epic contextual counts, got: %s", stdout)
	}
}

func TestListEpicsNoEpicsMessage(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, "", "list", "--epics")
	if code != 0 {
		t.Fatalf("list --epics failed: exit %d", code)
	}
	if !strings.Contains(stdout, "No epics.") {
		t.Errorf("expected no epics message, got: %s", stdout)
	}
}

func TestListEpicsRendersLikeListRows(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Epic A"}`, "new", "epic")
	epicA := strings.TrimSpace(stdout)

	stdout, _, code := runErgo(t, dir, "", "list", "--epics")
	if code != 0 {
		t.Fatalf("list --epics failed: exit %d", code)
	}
	if !strings.Contains(stdout, "Ⓔ") {
		t.Errorf("expected epic icon in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, epicA) {
		t.Errorf("expected epic ID %s in output, got: %s", epicA, stdout)
	}
	if strings.Contains(stdout, "  "+epicA+"  ") {
		t.Errorf("expected aligned list-row format (not raw 'ID  Title' lines), got: %s", stdout)
	}
}

func TestListQuietSuppressesSummaryAndHints(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runErgo(t, dir, `{"title":"Doing task"}`, "new", "task")
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runErgo(t, dir, `{"title":"Blocked task"}`, "new", "task")
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, `{"state":"blocked"}`, "set", blockedID)

	stdout, stderr, code := runErgo(t, dir, "", "list", "--ready", "--quiet")
	if code != 0 {
		t.Fatalf("list --ready --quiet failed: exit %d", code)
	}
	if !strings.Contains(stdout, "No ready tasks.") {
		t.Errorf("expected empty message in quiet mode, got: %s", stdout)
	}
	if strings.Contains(stdout, "in progress") || strings.Contains(stdout, "blocked") {
		t.Errorf("expected summary suppressed in quiet mode, got: %s", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("expected hints suppressed in quiet mode, got: %s", stderr)
	}
}
