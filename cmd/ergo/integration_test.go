// Integration tests for JSON input flow.
// These test the end-to-end wiring (stdin → validation → events → output)
// rather than individual functions (covered by unit tests).
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

	// Use bash -c to properly pipe stdin (Go's exec doesn't set pipe mode)
	if stdin != "" {
		cmdStr := ergoBinary
		for _, arg := range args {
			// Shell-escape args
			cmdStr += " '" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		}
		cmd := exec.Command("bash", "-c", "echo '"+stdin+"' | "+cmdStr)
		cmd.Dir = dir
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

	// No stdin
	cmd := exec.Command(ergoBinary, args...)
	cmd.Dir = dir
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
	_, _, code = runErgo(t, dir, "", "dep", t2, t1)
	if code != 0 {
		t.Fatalf("dep %s %s failed: exit %d", t2, t1, code)
	}

	// Mutate T1 across multiple dimensions.
	_, stderr, code := runErgo(t, dir, `{"claim":"agent-1","state":"doing","worker":"human","body":"T1\\n\\n## v2\\nmore"}`, "set", t1)
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
		`{"title":"Updated title","worker":"agent","state":"doing","claim":"agent-1"}`,
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

	if !strings.Contains(task["body"].(string), "Updated title") {
		t.Errorf("title not updated, got body: %v", task["body"])
	}
	if task["state"] != "doing" {
		t.Errorf("expected state=doing, got %v", task["state"])
	}
	if task["worker"] != "agent" {
		t.Errorf("expected worker=agent, got %v", task["worker"])
	}
	if task["claimed_by"] != "agent-1" {
		t.Errorf("expected claimed_by=agent-1, got %v", task["claimed_by"])
	}
}

// TestTitleAndBodyStoredCorrectly verifies that title and body are combined
// and stored correctly, with title appearing as the first line.
// This prevents regression of the "list shows body instead of title" bug.
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

	// Verify via show --json that body contains title as first line
	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}

	body, ok := task["body"].(string)
	if !ok {
		t.Fatalf("expected body string, got %T", task["body"])
	}

	// Body should start with title
	if !strings.HasPrefix(body, "My Important Task\n") {
		t.Errorf("body should start with title, got: %q", body)
	}

	// Body should contain the body text
	if !strings.Contains(body, "This is the detailed body text") {
		t.Errorf("body should contain body text, got: %q", body)
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

// TestSetRejectsEpicState verifies that epics cannot have state/worker/claim set.
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
		{"worker rejected", `{"worker":"agent"}`, "epics do not have workers"},
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
