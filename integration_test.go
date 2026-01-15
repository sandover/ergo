// Integration tests for JSON input flow.
// These test the end-to-end wiring (stdin → validation → events → output)
// rather than individual functions (covered by unit tests).
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
