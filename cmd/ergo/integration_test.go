// CLI integration tests for end-to-end command behavior.
// Purpose: validate stdin→validation→events→output wiring across commands.
// Exports: none.
// Role: verifies user-visible behavior including prune/compact semantics.
// Invariants: tests avoid timing assumptions; outputs follow public contracts.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
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

func runNewTask(t *testing.T, dir string, inlineJSON string, extraArgs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	args := []string{"new", "task"}
	if inlineJSON != "" {
		args = append(args, inlineJSON)
	}
	args = append(args, extraArgs...)
	return runErgo(t, dir, "", args...)
}

func runNewTaskWithBody(t *testing.T, dir string, body string, inlineJSON string, extraArgs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	args := []string{"new", "task"}
	if inlineJSON != "" {
		args = append(args, inlineJSON)
	}
	args = append(args, extraArgs...)
	return runErgo(t, dir, body, args...)
}

func runSetTask(t *testing.T, dir string, id string, inlineJSON string, extraArgs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	args := []string{"set", id}
	if inlineJSON != "" {
		args = append(args, inlineJSON)
	}
	args = append(args, extraArgs...)
	return runErgo(t, dir, "", args...)
}

func runSetTaskWithBody(t *testing.T, dir string, id string, body string, inlineJSON string, extraArgs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	args := []string{"set", id}
	if inlineJSON != "" {
		args = append(args, inlineJSON)
	}
	args = append(args, extraArgs...)
	return runErgo(t, dir, body, args...)
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

// getEventFilePath returns the path to the event log file (plans.jsonl or events.jsonl)
func getEventFilePath(dir string) string {
	plansPath := filepath.Join(dir, ".ergo", "plans.jsonl")
	eventsPath := filepath.Join(dir, ".ergo", "events.jsonl")

	// Prefer plans.jsonl if it exists
	if _, err := os.Stat(plansPath); err == nil {
		return plansPath
	}

	// Fall back to events.jsonl
	if _, err := os.Stat(eventsPath); err == nil {
		return eventsPath
	}

	// Default to plans.jsonl
	return plansPath
}

func countEventLines(t *testing.T, dir string) int {
	t.Helper()
	path := getEventFilePath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read event log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

func writePlanFile(t *testing.T, dir string, content string) string {
	t.Helper()
	file, err := os.CreateTemp(dir, "plan-*.md")
	if err != nil {
		t.Fatalf("failed to create plan file: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}
	return file.Name()
}

func runPlan(t *testing.T, dir string, planContent string, inlineJSON string, extraArgs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	planPath := writePlanFile(t, dir, planContent)
	args := []string{"plan", "--file", planPath}
	if inlineJSON != "" {
		args = append(args, inlineJSON)
	}
	args = append(args, extraArgs...)
	return runErgo(t, dir, "", args...)
}

func TestNewTask_HappyPath(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	if len(taskID) != 6 {
		t.Errorf("expected 6-char task ID, got %q", taskID)
	}
}

func TestRemovedNewEpicCommandFails(t *testing.T) {
	dir := setupErgo(t)

	stdout, stderr, code := runErgo(t, dir, "", "new", "epic")
	if code == 0 {
		t.Fatalf("expected removed new epic command to fail, got stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, `unknown command "epic" for "ergo new"`) {
		t.Fatalf("expected removed-command error, got stderr=%q", stderr)
	}
	if strings.Contains(stdout, "COMMANDS") {
		t.Fatalf("expected no parent help on removed command, got stdout=%q", stdout)
	}
}

func TestNewTask_StdinBody_Multiline(t *testing.T) {
	dir := setupErgo(t)
	body := "line1\nline2\n"

	stdout, stderr, code := runErgo(t, dir, body, "new", "task", `{"title":"Test task"}`)
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

func TestNewContainer_StdinBody_Multiline(t *testing.T) {
	dir := setupErgo(t)
	body := "epic line1\nepic line2\n"

	stdout, stderr, code := runErgo(t, dir, body, "new", "task", `{"title":"My Epic"}`)
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

func TestShowEpicChildrenDependencyOrder(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Body", `{"title":"Order Epic"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"A","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task A failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"B","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task B failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"C","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task C failed: exit %d", code)
	}
	taskC := strings.TrimSpace(stdout)

	_, _, code = runErgo(t, dir, "", "sequence", taskA, taskB, taskC)
	if code != 0 {
		t.Fatalf("sequence failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "show", epicID, "--json")
	if code != 0 {
		t.Fatalf("show --json failed: exit %d", code)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse show --json: %v", err)
	}
	rawChildren, ok := payload["children"].([]interface{})
	if !ok {
		t.Fatalf("expected children array in show --json payload: %v", payload)
	}
	if len(rawChildren) != 3 {
		t.Fatalf("expected 3 children, got %d", len(rawChildren))
	}

	pos := map[string]int{}
	for i, raw := range rawChildren {
		child, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected child object, got %T", raw)
		}
		id, _ := child["id"].(string)
		pos[id] = i
	}
	if !(pos[taskA] < pos[taskB] && pos[taskB] < pos[taskC]) {
		t.Fatalf("expected child order %s -> %s -> %s, got positions %v", taskA, taskB, taskC, pos)
	}

	stdout, _, code = runErgo(t, dir, "", "show", epicID)
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	idxA := strings.Index(stdout, taskA)
	idxB := strings.Index(stdout, taskB)
	idxC := strings.Index(stdout, taskC)
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("expected all task IDs in human show output, got: %s", stdout)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Fatalf("expected human show order %s -> %s -> %s", taskA, taskB, taskC)
	}
}

func TestShowEpicHumanDocumentFirstLayout(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Plan body line\n\n- item", `{"title":"Plan Epic"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	stdout, _, code = runNewTaskWithBody(t, dir, "First body", fmt.Sprintf(`{"title":"First task","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	task1 := strings.TrimSpace(stdout)

	stdout, _, code = runNewTaskWithBody(t, dir, "Second body", fmt.Sprintf(`{"title":"Claimed task","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	task2 := strings.TrimSpace(stdout)

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "r1.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write result file failed: %v", err)
	}
	_, _, code = runSetTask(t, dir, task1, `{"result":"docs/r1.md"}`)
	if code != 0 {
		t.Fatalf("set result failed: exit %d", code)
	}
	_, _, code = runSetTask(t, dir, task2, `{"claim":"agent-x"}`)
	if code != 0 {
		t.Fatalf("set claim failed: exit %d", code)
	}
	_, _, code = runErgo(t, dir, "", "sequence", task1, task2)
	if code != 0 {
		t.Fatalf("sequence failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "show", epicID)
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}

	if !strings.HasPrefix(stdout, "---\n") {
		t.Fatalf("expected front matter document start: %s", stdout)
	}
	if !strings.Contains(stdout, "\ncontainer: \"true\"\n") || !strings.Contains(stdout, "\nid: \""+epicID+"\"\n") {
		t.Fatalf("expected container front matter keys in output: %s", stdout)
	}
	if !strings.Contains(stdout, "# Plan Epic") {
		t.Fatalf("expected heading in show output: %s", stdout)
	}
	if strings.Contains(stdout, "──────────────────────────────────────────────────") {
		t.Fatalf("did not expect non-markdown separator lines in show output: %s", stdout)
	}
	if !strings.Contains(stdout, "### "+task1+" - First task") || !strings.Contains(stdout, "### "+task2+" - Claimed task") {
		t.Fatalf("expected child markdown headings in output: %s", stdout)
	}
	if !strings.Contains(stdout, "First body") || !strings.Contains(stdout, "Second body") {
		t.Fatalf("expected child task bodies in epic show output: %s", stdout)
	}

	idxBody := strings.Index(stdout, "Plan body line")
	idxTasks := strings.Index(stdout, "## Tasks")
	if idxBody < 0 || idxTasks < 0 {
		t.Fatalf("expected epic body and tasks section in output: %s", stdout)
	}
	if !(idxBody < idxTasks) {
		t.Fatalf("expected epic body before tasks section, got output: %s", stdout)
	}

	task1Start := strings.Index(stdout, "### "+task1+" - First task")
	task2Start := strings.Index(stdout, "### "+task2+" - Claimed task")
	if task1Start < 0 || task2Start < 0 || task2Start <= task1Start {
		t.Fatalf("expected child sections in dependency order: %s", stdout)
	}
	task1Section := stdout[task1Start:task2Start]
	if !strings.Contains(task1Section, "- state: ") {
		t.Fatalf("expected child state metadata in task section: %s", task1Section)
	}
	if !strings.Contains(task1Section, "- results:") {
		t.Fatalf("expected child results metadata in task section: %s", task1Section)
	}
	if strings.Contains(task1Section, "claim:") || strings.Contains(task1Section, "deps:") || strings.Contains(task1Section, "rdeps:") || strings.Contains(task1Section, "created:") || strings.Contains(task1Section, "updated:") {
		t.Fatalf("expected child metadata to exclude claim/deps/rdeps/timestamps: %s", task1Section)
	}
}

func TestShowEpicOmitsBodySectionWhenEmpty(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"No Body Epic"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	_, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"Task 1","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "show", epicID)
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	if !strings.Contains(stdout, "# No Body Epic\n\n## Tasks") {
		t.Fatalf("expected heading followed by tasks section when epic body is empty: %s", stdout)
	}
	if strings.Contains(stdout, "──────────────────────────────────────────────────") {
		t.Fatalf("did not expect non-markdown separator lines in output: %s", stdout)
	}
}

func TestShowTaskHumanOutputUnchanged(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Body text", `{"title":"Standalone"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, "", "show", taskID)
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	if !strings.HasPrefix(stdout, "---\n") {
		t.Fatalf("expected markdown front matter start: %s", stdout)
	}
	if !strings.Contains(stdout, "\nid: \""+taskID+"\"\n") {
		t.Fatalf("expected id in front matter: %s", stdout)
	}
	if !strings.Contains(stdout, "# Standalone") || !strings.Contains(stdout, "Body text") {
		t.Fatalf("expected task markdown heading and body in output: %s", stdout)
	}
	if strings.Contains(stdout, "──────────────────────────────────────────────────") {
		t.Fatalf("did not expect non-markdown separator lines in task output: %s", stdout)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("did not expect ANSI escape sequences in task markdown output: %s", stdout)
	}
}

func TestShowTaskHeaderDense(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"Epic"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"Task A","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"Task B","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	_, _, code = runErgo(t, dir, "", "sequence", taskA, taskB)
	if code != 0 {
		t.Fatalf("sequence failed: exit %d", code)
	}

	_, _, code = runSetTask(t, dir, taskB, `{"claim":"agent-x"}`)
	if code != 0 {
		t.Fatalf("set claim failed: exit %d", code)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "r1.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write result file failed: %v", err)
	}
	_, _, code = runSetTask(t, dir, taskB, `{"result":"docs/r1.md"}`)
	if code != 0 {
		t.Fatalf("set result failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "show", taskB)
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	if !strings.Contains(stdout, "## Results") {
		t.Fatalf("expected markdown results heading for task show output: %s", stdout)
	}
	if !strings.Contains(stdout, "docs/r1.md") {
		t.Fatalf("expected markdown results entry to include result path: %s", stdout)
	}
}

func TestNewTask_InlineJSONValidationErrors(t *testing.T) {
	dir := setupErgo(t)

	_, stderr, code := runErgo(t, dir, "x", "new", "task")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "missing required: title") {
		t.Fatalf("expected missing-title error, got stderr=%q", stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "new", "task", `{"titl":"T"}`)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "unknown field") {
		t.Fatalf("expected unknown-field error, got stderr=%q", stderr)
	}

	_, stderr, code = runErgo(t, dir, "x", "new", "task", `{"title":"T","state":"doing"}`)
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

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
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
	stdout, _, code := runNewTask(t, dir, `{}`, "--json")

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
	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set state to done
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

func TestSet_StdinBody_UpdatesBody(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	newBody := "updated\nbody\n"
	_, stderr, code := runSetTaskWithBody(t, dir, taskID, newBody, `{"state":"done"}`)
	if code != 0 {
		t.Fatalf("set failed: exit %d (stderr=%q)", code, stderr)
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

func TestSet_MetadataOnly_KeepsBody(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	_, stderr, code := runErgo(t, dir, "", "set", taskID, `{"state":"blocked"}`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}

	stdout, _, code = runErgo(t, dir, "", "show", taskID, "--json")
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	var task map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("failed to parse show output: %v", err)
	}
	if task["body"] != "Test task" {
		t.Fatalf("expected body to remain unchanged, got %q", task["body"])
	}
	if task["state"] != "blocked" {
		t.Fatalf("expected state=blocked, got %v", task["state"])
	}
}

func TestSet_JSONOutput(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	stdout, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`, "--json")
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
	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set to done
	runSetTask(t, dir, taskID, `{"state":"done"}`)

	// Try invalid transition done→doing
	_, stderr, code := runSetTask(t, dir, taskID, `{"state":"doing","claim":"agent-1"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, `{"title":"Task B"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, `{"title":"Task B"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, `{"title":"Task C"}`)
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

	_, _, code = runSetTask(t, dir, taskA, `{"state":"done"}`)
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

	_, _, code = runSetTask(t, dir, taskB, `{"state":"done"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)
	stdout, _, code = runNewTask(t, dir, `{"title":"Task B"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	_, _, code = runErgo(t, dir, "", "sequence", taskB, taskA)
	if code != 0 {
		t.Fatalf("sequence failed: exit %d", code)
	}
	_, _, code = runSetTask(t, dir, taskB, `{"state":"done"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Epic 1"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epic1 := strings.TrimSpace(stdout)
	stdout, _, code = runNewTask(t, dir, `{"title":"Epic 2"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epic2 := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, `{"title":"Task Done","epic":"`+epic1+`"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskDone := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskDone, `{"state":"done"}`)
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	stdout, _, code = runNewTask(t, dir, `{"title":"Task Active","epic":"`+epic2+`"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskActive := strings.TrimSpace(stdout)

	stdout, _, code = runNewTask(t, dir, `{"title":"Blocked"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskBlocked := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskBlocked, `{"state":"blocked"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

	data, err := os.ReadFile(getEventFilePath(dir))
	if err != nil {
		t.Fatalf("failed to read event log: %v", err)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
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

	data, err := os.ReadFile(getEventFilePath(dir))
	if err != nil {
		t.Fatalf("failed to read event log: %v", err)
	}
	tombstones := strings.Count(string(data), "tombstone")
	if tombstones != 1 {
		t.Fatalf("expected exactly one tombstone event, got %d (stdout1=%q stderr1=%q stdout2=%q stderr2=%q)", tombstones, r1.stdout, r1.stderr, r2.stdout, r2.stderr)
	}
}

func TestCreateAndClaim_Atomic(t *testing.T) {
	dir := setupErgo(t)

	// Create task with state=doing and claim in one operation
	stdout, _, code := runNewTaskWithBody(t, dir,
		"Urgent task",
		`{"title":"Urgent task","state":"doing","claim":"agent-1"}`,
		"--json")

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
	stdout, _, code := runNewTaskWithBody(t, dir, "Epic", `{"title":"Epic"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	// Create two tasks in the epic
	stdout, _, code = runNewTaskWithBody(t, dir, "T1", `{"title":"T1","epic":"`+epicID+`"}`)
	if code != 0 {
		t.Fatalf("new task T1 failed: exit %d", code)
	}
	t1 := strings.TrimSpace(stdout)

	stdout, _, code = runNewTaskWithBody(t, dir, "T2", `{"title":"T2","epic":"`+epicID+`"}`)
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
	_, stderr, code := runSetTaskWithBody(t, dir, t1, "T1\\n\\n## v2\\nmore", `{"claim":"agent-1","state":"doing"}`)
	if code != 0 {
		t.Fatalf("set %s failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = runSetTask(t, dir, t1, `{"state":"error","claim":"agent-1"}`)
	if code != 0 {
		t.Fatalf("set %s state=error failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = runSetTask(t, dir, t1, `{"state":"doing","claim":"agent-1"}`)
	if code != 0 {
		t.Fatalf("set %s state=doing failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = runSetTask(t, dir, t1, `{"state":"done"}`)
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
	_, _, code = runSetTask(t, dir, t1, `{"result":"docs/r1.md"}`)
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

func TestNewContainer_HappyPath(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runNewTaskWithBody(t, dir, "Test Epic", `{"title":"Test Epic"}`)

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
	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Update multiple fields in one call
	_, _, code = runSetTask(t, dir, taskID,
		`{"title":"Updated title","state":"doing","claim":"agent-1"}`)
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

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
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

func TestClaim_JSONIncludesReminder(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	agentID := "sonnet@agent-host"
	stdout, _, code = runErgo(t, dir, "", "--json", "claim", taskID, "--agent", agentID)
	if code != 0 {
		t.Fatalf("claim --json failed: exit %d", code)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse claim output: %v", err)
	}
	if out["reminder"] != "When you have completed this claimed task, you MUST mark it done." {
		t.Fatalf("expected reminder field, got %v", out["reminder"])
	}
}

func TestClaimOldestReady_JSONIncludesReminder(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	_ = strings.TrimSpace(stdout)

	agentID := "sonnet@agent-host"
	stdout, _, code = runErgo(t, dir, "", "--json", "claim", "--agent", agentID)
	if code != 0 {
		t.Fatalf("claim oldest-ready --json failed: exit %d", code)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse claim output: %v", err)
	}
	if out["reminder"] != "When you have completed this claimed task, you MUST mark it done." {
		t.Fatalf("expected reminder field, got %v", out["reminder"])
	}
}

// TestTitleAndBodyStoredCorrectly verifies that title and body are stored separately.
func TestTitleAndBodyStoredCorrectly(t *testing.T) {
	dir := setupErgo(t)

	// Create task with distinct title and body
	stdout, _, code := runNewTaskWithBody(t, dir,
		"This is the detailed body text",
		`{"title":"My Important Task"}`)
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
	stdout, _, code := runNewTaskWithBody(t, dir, "Test body", `{"title":"Test"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set state and verify output
	stdout, _, code = runSetTask(t, dir, taskID, `{"state":"done"}`)
	if code != 0 {
		t.Fatalf("set failed: exit %d", code)
	}

	output := strings.TrimSpace(stdout)
	if output != taskID {
		t.Errorf("expected set to output %q, got %q", taskID, output)
	}
}

// TestSetRejectsEpicState verifies that containers cannot have state/claim set.
func TestSetRejectsEpicState(t *testing.T) {
	dir := setupErgo(t)

	// Create a container (task with children)
	stdout, _, code := runNewTask(t, dir, `{"title":"Test Epic"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	epicID := strings.TrimSpace(stdout)

	// Add a child to make it a container
	_, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"Child","epic":"%s"}`, epicID))
	if code != 0 {
		t.Fatalf("new child task failed: exit %d", code)
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"state rejected", `{"state":"done"}`, "containers do not have state"},
		{"claim rejected", `{"claim":"agent-1"}`, "containers cannot be claimed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runSetTask(t, dir, epicID, tt.input)
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
	stdout, _, _ := runNewTask(t, dir, `{"title":"Test Epic"}`)
	epicID := strings.TrimSpace(stdout)

	// Create tasks: one done, one canceled, one todo
	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Done task","epic":"%s"}`, epicID))
	doneID := strings.TrimSpace(stdout)
	runSetTask(t, dir, doneID, `{"state":"done"}`)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Canceled task","epic":"%s"}`, epicID))
	canceledID := strings.TrimSpace(stdout)
	runSetTask(t, dir, canceledID, `{"state":"canceled"}`)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Todo task","epic":"%s"}`, epicID))
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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Ready task"}`)
	readyID := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	runSetTask(t, dir, doneID, `{"state":"done"}`)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Blocked task"}`)
	blockedID := strings.TrimSpace(stdout)
	runSetTask(t, dir, blockedID, `{"state":"blocked"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Epic A"}`)
	epicA := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Epic B"}`)
	epicB := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"A1","epic":"%s"}`, epicA))
	taskA1 := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"B1","epic":"%s"}`, epicB))
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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Blocker"}`)
	blockerID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", blockerID, "--agent", "test@local")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Blocked by dependency"}`)
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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Error task"}`)
	errorID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", errorID, "--agent", "test@local")
	_, _, _ = runSetTask(t, dir, errorID, `{"state":"error"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Epic"}`)
	epicID := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Done","epic":"%s"}`, epicID))
	doneID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, doneID, `{"state":"done"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Ready task"}`)
	readyID := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	runSetTask(t, dir, doneID, `{"state":"done"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Epic A"}`)
	epicA := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Epic B"}`)
	epicB := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"A1","epic":"%s"}`, epicA))
	taskA1 := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"A2","epic":"%s"}`, epicA))
	taskA2 := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, taskA2, `{"state":"done"}`)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"B1","epic":"%s"}`, epicB))
	taskB1 := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Orphan"}`)
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
	if !strings.Contains(stderr, "Coding agents should call 'ergo --json list'") {
		t.Errorf("expected agents hint in stderr, got: %s", stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "list", "--epic", "ZZZZZZ")
	if code == 0 {
		t.Fatalf("expected error for invalid epic ID")
	}
	if !strings.Contains(stderr, "no such container: ZZZZZZ") {
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
}

func TestListReadyEmptyStateWithContext(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runNewTask(t, dir, `{"title":"Doing task"}`)
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Blocked task"}`)
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, blockedID, `{"state":"blocked"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, doneID, `{"state":"done"}`)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Canceled task"}`)
	canceledID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, canceledID, `{"state":"canceled"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Ready task"}`)
	_ = strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"In progress task"}`)
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Blocked task"}`)
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, blockedID, `{"state":"blocked"}`)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Error task"}`)
	errorID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", errorID, "--agent", "test@local")
	_, _, _ = runSetTask(t, dir, errorID, `{"state":"error"}`)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, doneID, `{"state":"done"}`)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Canceled task"}`)
	canceledID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, canceledID, `{"state":"canceled"}`)

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

	stdout, _, _ := runNewTask(t, dir, `{"title":"Epic"}`)
	epicID := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Doing","epic":"%s"}`, epicID))
	doingID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", doingID, "--agent", "test@local")

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Blocked","epic":"%s"}`, epicID))
	blockedID := strings.TrimSpace(stdout)
	_, _, _ = runSetTask(t, dir, blockedID, `{"state":"blocked"}`)

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

func TestPlan_JSONOutput_HappyPath(t *testing.T) {
	dir := setupErgo(t)
	planInput := `# Add auth middleware
Middleware body
---
# Add login endpoint
Login body
---
# Add signup endpoint
Signup body
---
# Write integration tests
Test body
`

	stdout, stderr, code := runPlan(t, dir, planInput, `{"title":"Add user auth"}`, "--json")
	if code != 0 {
		t.Fatalf("plan --json failed: exit %d, stderr=%s, stdout=%s", code, stderr, stdout)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to parse plan --json output: %v", err)
	}

	if out["kind"] != "create" {
		t.Fatalf("expected kind=create, got %v", out["kind"])
	}
	if out["container"] != true {
		t.Fatalf("expected container=true, got %v", out["container"])
	}
	containerID := fmt.Sprint(out["id"])
	if strings.TrimSpace(containerID) == "" {
		t.Fatalf("expected non-empty container id, got %v", out["id"])
	}
	if out["title"] != "Add user auth" {
		t.Fatalf("expected container title 'Add user auth', got %v", out["title"])
	}
	if out["state"] != "todo" {
		t.Fatalf("expected state=todo, got %v", out["state"])
	}
	if _, hasEpic := out["epic"]; hasEpic {
		t.Fatalf("expected no 'epic' field in plan output")
	}
	eventLog, err := os.ReadFile(getEventFilePath(dir))
	if err != nil {
		t.Fatalf("failed to read event log: %v", err)
	}
	if strings.Contains(string(eventLog), `"type":"new_epic"`) {
		t.Fatalf("expected plan to write unified new_task events, got log: %s", eventLog)
	}

	childrenRaw, ok := out["children"].([]interface{})
	if !ok {
		t.Fatalf("expected children array, got %T", out["children"])
	}
	if len(childrenRaw) != 4 {
		t.Fatalf("expected 4 children, got %d", len(childrenRaw))
	}

	seenTitles := map[string]bool{}
	for _, raw := range childrenRaw {
		child, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected child object, got %T", raw)
		}
		title := fmt.Sprint(child["title"])
		if title == "" || fmt.Sprint(child["id"]) == "" {
			t.Fatalf("expected non-empty child title/id, got %v", child)
		}
		seenTitles[title] = true
	}
	for _, expected := range []string{"Add auth middleware", "Add login endpoint", "Add signup endpoint", "Write integration tests"} {
		if !seenTitles[expected] {
			t.Fatalf("expected child title %q in output, got %v", expected, seenTitles)
		}
	}

	stdout, _, code = runErgo(t, dir, "", "list", "--ready", "--json")
	if code != 0 {
		t.Fatalf("list --ready --json failed: exit %d", code)
	}
	var ready []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &ready); err != nil {
		t.Fatalf("failed to parse ready list: %v", err)
	}
	if len(ready) != 4 {
		t.Fatalf("expected all 4 leaf tasks to be ready, got %d: %v", len(ready), ready)
	}
}

func TestPlan_FailuresReturnErrorsAndDoNotWritePartialState(t *testing.T) {
	tests := []struct {
		name          string
		planContent   string
		inlineJSON    string
		jsonOutput    bool
		expectedError string
		expectedStderr string
	}{
		{
			name:          "duplicate task title",
			planContent:   "# A\nfirst\n---\n# A\nsecond\n",
			inlineJSON:    `{"title":"Epic"}`,
			expectedStderr: "duplicate task title",
		},
		{
			name:           "chunk missing heading",
			planContent:    "not a heading\nbody\n",
			inlineJSON:     `{"title":"Epic"}`,
			expectedStderr: "chunk must start with '# Title'",
		},
		{
			name:           "empty plan file",
			planContent:    "\n\n",
			inlineJSON:     `{"title":"Epic"}`,
			expectedStderr: "plan file contains no task chunks",
		},
		{
			name:          "missing inline title",
			planContent:   "# A\nbody\n",
			inlineJSON:    `{}`,
			jsonOutput:    true,
			expectedError: "validation_failed",
		},
		{
			name:          "malformed json",
			planContent:   "# A\nbody\n",
			inlineJSON:    `{"title":"Epic"`,
			jsonOutput:    true,
			expectedError: "parse_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupErgo(t)

			extraArgs := []string{}
			if tt.jsonOutput {
				extraArgs = append(extraArgs, "--json")
			}
			stdout, stderr, code := runPlan(t, dir, tt.planContent, tt.inlineJSON, extraArgs...)
			if code == 0 {
				t.Fatalf("expected non-zero exit for %s (stdout=%q stderr=%q)", tt.name, stdout, stderr)
			}
			if tt.jsonOutput {
				var out map[string]interface{}
				if err := json.Unmarshal([]byte(stdout), &out); err != nil {
					t.Fatalf("expected JSON error output, got parse error: %v (stdout=%q)", err, stdout)
				}
				if out["error"] != tt.expectedError {
					t.Fatalf("expected error=%s, got %v", tt.expectedError, out["error"])
				}
			} else if !strings.Contains(stderr, tt.expectedStderr) {
				t.Fatalf("expected stderr to contain %q, got %q", tt.expectedStderr, stderr)
			}

			stdout, _, code = runErgo(t, dir, "", "list", "--json")
			if code != 0 {
				t.Fatalf("list --json failed: exit %d", code)
			}
			var tasks []map[string]interface{}
			if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
				t.Fatalf("failed to parse list --json output: %v", err)
			}
			if len(tasks) != 0 {
				t.Fatalf("expected no tasks after failed bulk-create, got %d", len(tasks))
			}
		})
	}
}

// TestContainerPromotion_RejectsIfLeafIsDirty verifies PMY5QR: assigning a first
// child to a non-todo leaf is rejected to prevent impossible container state.
func TestContainerPromotion_RejectsIfLeafIsDirty(t *testing.T) {
	t.Run("claimed leaf", func(t *testing.T) {
		dir := setupErgo(t)
		stdout, _, _ := runNewTask(t, dir, `{"title":"Parent"}`)
		parentID := strings.TrimSpace(stdout)
		_, _, _ = runSetTask(t, dir, parentID, `{"state":"doing","claim":"agent-1"}`)

		_, stderr, code := runNewTask(t, dir, fmt.Sprintf(`{"title":"Child","epic":"%s"}`, parentID))
		if code == 0 {
			t.Fatalf("expected error assigning child to claimed leaf")
		}
		if !strings.Contains(stderr, "claimed") {
			t.Errorf("expected 'claimed' in error, got: %s", stderr)
		}
	})

	t.Run("non-todo leaf", func(t *testing.T) {
		dir := setupErgo(t)
		stdout, _, _ := runNewTask(t, dir, `{"title":"Parent"}`)
		parentID := strings.TrimSpace(stdout)
		_, _, _ = runSetTask(t, dir, parentID, `{"state":"done"}`)

		_, stderr, code := runNewTask(t, dir, fmt.Sprintf(`{"title":"Child","epic":"%s"}`, parentID))
		if code == 0 {
			t.Fatalf("expected error assigning child to done leaf")
		}
		if !strings.Contains(stderr, "state") {
			t.Errorf("expected 'state' in error, got: %s", stderr)
		}
	})

	t.Run("existing container still accepts children", func(t *testing.T) {
		dir := setupErgo(t)
		stdout, _, _ := runNewTask(t, dir, `{"title":"Parent"}`)
		parentID := strings.TrimSpace(stdout)
		// First child promotes to container
		_, _, code := runNewTask(t, dir, fmt.Sprintf(`{"title":"Child1","epic":"%s"}`, parentID))
		if code != 0 {
			t.Fatalf("expected first child to succeed")
		}
		// Second child should still work
		_, _, code = runNewTask(t, dir, fmt.Sprintf(`{"title":"Child2","epic":"%s"}`, parentID))
		if code != 0 {
			t.Fatalf("expected second child to succeed on existing container")
		}
	})

	t.Run("clean leaf accepts first child", func(t *testing.T) {
		dir := setupErgo(t)
		stdout, _, _ := runNewTask(t, dir, `{"title":"Parent"}`)
		parentID := strings.TrimSpace(stdout)
		_, _, code := runNewTask(t, dir, fmt.Sprintf(`{"title":"Child","epic":"%s"}`, parentID))
		if code != 0 {
			t.Fatalf("expected clean leaf to accept first child")
		}
	})
}

// TestDepSemantics_ContainerReadiness verifies 2ZYNNT: leaf→container deps use
// child-completion rather than container state; inherited parent deps also work.
func TestDepSemantics_ContainerReadiness(t *testing.T) {
	t.Run("leaf waits for container children", func(t *testing.T) {
		dir := setupErgo(t)

		// Create container B with two children
		out := map[string]interface{}{}
		stdout, _, _ := runPlan(t, dir, "# B1\n\n---\n# B2\n", `{"title":"B"}`, "--json")
		_ = json.Unmarshal([]byte(stdout), &out)
		bID := fmt.Sprint(out["id"])
		children := out["children"].([]interface{})
		b1ID := fmt.Sprint(children[0].(map[string]interface{})["id"])
		b2ID := fmt.Sprint(children[1].(map[string]interface{})["id"])

		// Create leaf A depending on container B
		stdout, _, _ = runNewTask(t, dir, `{"title":"A"}`)
		aID := strings.TrimSpace(stdout)
		// sequence bID aID → A depends on B (A comes after B)
		_, _, code := runErgo(t, dir, "", "sequence", bID, aID)
		if code != 0 {
			t.Fatalf("sequence B->A failed")
		}

		// A should be blocked while B's children are incomplete
		stdout, _, _ = runErgo(t, dir, "", "list", "--json", "--all")
		var tasks []map[string]interface{}
		_ = json.Unmarshal([]byte(stdout), &tasks)
		aBlocked := false
		for _, task := range tasks {
			if task["id"] == aID {
				aBlocked = task["blocked"].(bool)
			}
		}
		if !aBlocked {
			t.Fatalf("expected A to be blocked while container B has incomplete children")
		}

		// Complete B's children → A should become ready
		runSetTask(t, dir, b1ID, `{"state":"done"}`)
		runSetTask(t, dir, b2ID, `{"state":"done"}`)

		stdout, _, _ = runErgo(t, dir, "", "list", "--ready", "--json")
		var ready []map[string]interface{}
		_ = json.Unmarshal([]byte(stdout), &ready)
		found := false
		for _, task := range ready {
			if task["id"] == aID {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected A to be ready after all container B children done")
		}
		_ = b1ID
		_ = b2ID
		_ = bID
	})

	t.Run("inherited blocking: task in container A waits for container A's external dep", func(t *testing.T) {
		dir := setupErgo(t)

		// Create leaf L
		stdout, _, _ := runNewTask(t, dir, `{"title":"L"}`)
		lID := strings.TrimSpace(stdout)

		// Create container A with task T inside
		out := map[string]interface{}{}
		stdout, _, _ = runPlan(t, dir, "# T\n", `{"title":"A"}`, "--json")
		_ = json.Unmarshal([]byte(stdout), &out)
		aID := fmt.Sprint(out["id"])
		tID := fmt.Sprint(out["children"].([]interface{})[0].(map[string]interface{})["id"])

		// Container A depends on leaf L
		// sequence lID aID → A depends on L (A comes after L)
		_, _, code := runErgo(t, dir, "", "sequence", lID, aID)
		if code != 0 {
			t.Fatalf("sequence L->A failed")
		}

		// T (inside A) should be blocked because A's dep L is not done
		stdout, _, _ = runErgo(t, dir, "", "list", "--json", "--all")
		var tasks []map[string]interface{}
		_ = json.Unmarshal([]byte(stdout), &tasks)
		tBlocked := false
		for _, task := range tasks {
			if task["id"] == tID {
				tBlocked = task["blocked"].(bool)
			}
		}
		if !tBlocked {
			t.Fatalf("expected T (inside A) to be blocked because A depends on incomplete L")
		}

		// Complete L → T should become ready
		runSetTask(t, dir, lID, `{"state":"done"}`)
		stdout, _, _ = runErgo(t, dir, "", "list", "--ready", "--json")
		var ready []map[string]interface{}
		_ = json.Unmarshal([]byte(stdout), &ready)
		found := false
		for _, task := range ready {
			if task["id"] == tID {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected T to be ready after L done")
		}
		_ = aID
	})
}

// TestFixtureScripts builds the ergo binary and runs each testdata/*.sh script,
// asserting it exits cleanly and produces a graph with at least one container.
// This catches fixture drift the moment a script uses removed CLI syntax.
func TestFixtureScripts(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("could not resolve repo root: %v", err)
	}

	scripts, err := filepath.Glob(filepath.Join(repoRoot, "testdata", "*.sh"))
	if err != nil {
		t.Fatalf("could not glob fixture scripts: %v", err)
	}
	if len(scripts) == 0 {
		t.Fatal("no fixture scripts found in testdata/")
	}

	for _, script := range scripts {
		script := script
		t.Run(filepath.Base(script), func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()

			// Run the fixture script with ERGO pointing at the test binary
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", script)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "ERGO="+ergoBinary)

			var outBuf, errBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf

			if err := cmd.Run(); err != nil {
				t.Fatalf("fixture script %s failed:\nstdout: %s\nstderr: %s\nerr: %v",
					filepath.Base(script), outBuf.String(), errBuf.String(), err)
			}

			// Find the .ergo directory created by the script (may be nested)
			listDir := ""
			_ = filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
				if err != nil || !d.IsDir() || d.Name() != ".ergo" {
					return nil
				}
				listDir = filepath.Dir(path)
				return filepath.SkipAll
			})
			if listDir == "" {
				t.Fatalf("fixture script %s did not create an .ergo directory", filepath.Base(script))
			}

			// Verify the resulting graph has at least one epic (task with children)
			stdout, stderr, code := runErgo(t, listDir, "", "list", "--all", "--json")
			if code != 0 {
				t.Fatalf("list --all --json failed: exit %d, stderr=%s", code, stderr)
			}
			var tasks []map[string]interface{}
			if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
				t.Fatalf("failed to parse tasks JSON: %v (stdout=%q)", err, stdout)
			}
			hasEpicChild := false
			for _, task := range tasks {
				if epicID, ok := task["epic_id"].(string); ok && epicID != "" {
					hasEpicChild = true
					break
				}
			}
			if !hasEpicChild {
				t.Fatalf("expected at least one task with epic_id after running fixture script %s", filepath.Base(script))
			}
		})
	}
}
