// CLI integration tests for end-to-end command behavior.
// Purpose: validate stdin→validation→events→output wiring across commands.
// Exports: none.
// Role: verifies user-visible behavior including prune/compact semantics.
// Invariants: tests avoid timing assumptions; outputs follow public contracts.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
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
	if runtime.GOOS == "windows" {
		ergoBinary += ".exe"
	}

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
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run ergo: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func showTaskOutput(t *testing.T, dir, id string) string {
	t.Helper()
	stdout, stderr, code := runErgo(t, dir, "", "show", id)
	if code != 0 {
		t.Fatalf("show %s failed: %s", id, stderr)
	}
	return stdout
}

func showTaskFields(t *testing.T, dir, id string) map[string]string {
	t.Helper()
	output := showTaskOutput(t, dir, id)
	lines := strings.Split(output, "\n")
	fields := map[string]string{}
	if len(lines) == 0 || lines[0] != "---" {
		t.Fatalf("show %s lacks YAML front matter: %q", id, output)
	}
	for _, line := range lines[1:] {
		if line == "---" {
			return fields
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("invalid front matter line %q", line)
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		fields[key] = value
	}
	t.Fatalf("show %s has unterminated front matter", id)
	return nil
}

func outputIDs(output string) []string {
	var ids []string
	for _, line := range strings.Split(output, "\n") {
		for _, candidate := range strings.Fields(line) {
			if len(candidate) != 6 {
				continue
			}
			valid := true
			for _, r := range candidate {
				if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
					valid = false
					break
				}
			}
			if valid {
				ids = append(ids, candidate)
				break
			}
		}
	}
	return ids
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

// putTaskInState prepares graph scenarios through the public lifecycle verbs.
// Legacy error is the one exception because v3 intentionally cannot create it.
func putTaskInState(t *testing.T, dir, id, state, agent string) (string, string, int) {
	t.Helper()
	switch state {
	case "todo":
		return runErgo(t, dir, "", "release", id)
	case "doing":
		if agent == "" {
			agent = "fixture@local"
		}
		return runErgo(t, dir, "", "claim", id, "--agent", agent)
	case "blocked":
		return runErgo(t, dir, "", "block", id)
	case "done":
		return runErgo(t, dir, "", "done", id)
	case "canceled":
		return runErgo(t, dir, "", "cancel", id)
	case "error":
		if agent != "" {
			if stdout, stderr, code := runErgo(t, dir, "", "claim", id, "--agent", agent); code != 0 {
				return stdout, stderr, code
			}
		}
		appendLegacyErrorState(t, dir, id)
		return id + "\n", "", 0
	default:
		return "", "invalid fixture state", 1
	}
}

func attachResultForTest(t *testing.T, dir, id, path string) (string, string, int) {
	t.Helper()
	state := showTaskFields(t, dir, id)["state"]
	verb := map[string]string{
		"todo": "release", "doing": "release", "blocked": "block",
		"done": "done", "canceled": "cancel", "error": "release",
	}[state]
	return runErgo(t, dir, "", verb, id, "--result", path)
}

func appendLegacyErrorState(t *testing.T, dir, id string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf("{\"type\":\"state\",\"ts\":%q,\"data\":{\"id\":%q,\"state\":\"error\",\"ts\":%q}}\n", now, id, now)
	file, err := os.OpenFile(getEventFilePath(dir), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(line); err != nil {
		t.Fatal(err)
	}
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

	stdout = showTaskOutput(t, dir, taskID)
	if !strings.Contains(stdout, body) {
		t.Errorf("expected body=%q in show output: %s", body, stdout)
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

	stdout = showTaskOutput(t, dir, epicID)
	if !strings.Contains(stdout, body) {
		t.Errorf("expected body=%q in show output: %s", body, stdout)
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

	stdout, _, code = runErgo(t, dir, "", "show", epicID)
	if code != 0 {
		t.Fatalf("show failed: exit %d", code)
	}
	posA, posB, posC := strings.Index(stdout, "### "+taskA+" -"), strings.Index(stdout, "### "+taskB+" -"), strings.Index(stdout, "### "+taskC+" -")
	if posA < 0 || !(posA < posB && posB < posC) {
		t.Fatalf("expected child order %s -> %s -> %s in:\n%s", taskA, taskB, taskC, stdout)
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
	_, _, code = attachResultForTest(t, dir, task1, "docs/r1.md")
	if code != 0 {
		t.Fatalf("set result failed: exit %d", code)
	}
	_, _, code = putTaskInState(t, dir, task2, "doing", "agent-x")
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
	if !strings.Contains(stdout, "\ncontainer: true\n") || !strings.Contains(stdout, "\nid: \""+epicID+"\"\n") {
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
	if !strings.Contains(task1Section, "#### Results") {
		t.Fatalf("expected child results metadata in task section: %s", task1Section)
	}
	if !strings.Contains(task1Section, "#### Dependencies") || !strings.Contains(task1Section, "blocks `"+task2+"`") {
		t.Fatalf("expected child dependencies in task section: %s", task1Section)
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

	_, _, code = putTaskInState(t, dir, taskB, "doing", "agent-x")
	if code != 0 {
		t.Fatalf("set claim failed: exit %d", code)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "r1.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write result file failed: %v", err)
	}
	_, _, code = attachResultForTest(t, dir, taskB, "docs/r1.md")
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
	if !strings.Contains(stderr, "state=doing requires a claim") {
		t.Fatalf("expected claim invariant error, got stderr=%q", stderr)
	}
}

func TestNewTaskRejectsLegacyErrorState(t *testing.T) {
	dir := setupErgo(t)
	_, stderr, code := runErgo(t, dir, "", "new", "task", `{"title":"Bad state","state":"error","claim":"agent@local"}`)
	if code == 0 || !strings.Contains(stderr, "error is legacy-only") || !strings.Contains(stderr, "blocked") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestParentCommandsRejectUnexpectedArguments(t *testing.T) {
	dir := setupErgo(t)
	for _, args := range [][]string{{"list", "extra"}, {"quickstart", "extra"}, {"version", "extra"}, {"new", "extra"}} {
		_, stderr, code := runErgo(t, dir, "", args...)
		if code == 0 {
			t.Fatalf("%v accepted unexpected arguments; stderr=%q", args, stderr)
		}
	}
}

func TestCommandRegistrationMatchesV3(t *testing.T) {
	registered := map[string]bool{}
	for _, command := range rootCmd.Commands() {
		registered[command.Name()] = true
	}
	for _, name := range []string{"claim", "done", "block", "cancel", "release", "title", "body", "move", "sequence", "unsequence"} {
		if !registered[name] {
			t.Errorf("missing v3 command %s", name)
		}
	}
	for _, name := range []string{"set", "reopen"} {
		if registered[name] {
			t.Errorf("removed command %s is still registered", name)
		}
	}
}

func TestPathAndCreationConfirmations(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runErgo(t, dir, "", "init")
	wantPath := filepath.Join(dir, ".ergo")
	if code != 0 || strings.TrimSpace(stdout) != ".ergo" || stderr != "" {
		t.Fatalf("init: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = runErgo(t, dir, "", "where")
	if code != 0 || strings.TrimSpace(stdout) != wantPath || stderr != "" {
		t.Fatalf("where: code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, wantPath)
	}
	stdout, stderr, code = runNewTask(t, dir, `{"title":"Confirmed task"}`)
	if code != 0 || len(strings.TrimSpace(stdout)) != 6 || stderr != "" {
		t.Fatalf("new task: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestNoReadyClaimIsReadable(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runErgo(t, dir, "", "claim", "--agent", "agent@local")
	if code != 0 || stdout != "No ready ergo tasks.\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestSuccessfulOutputNeverRedirectsAgentsToAnotherEncoding(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runNewTask(t, dir, `{"title":"Readable task"}`)
	if code != 0 {
		t.Fatal("new task failed")
	}
	id := strings.TrimSpace(stdout)
	commands := [][]string{{"list"}, {"show", id}, {"claim", id, "--agent", "agent@local"}}
	for _, args := range commands {
		stdout, stderr, code := runErgo(t, dir, "", args...)
		if code != 0 {
			t.Fatalf("%v failed: %s", args, stderr)
		}
		trimmed := strings.TrimSpace(stdout)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") ||
			strings.Contains(strings.ToLower(stdout), "json output") || strings.Contains(strings.ToLower(stderr), "encoding") {
			t.Fatalf("%v emitted alternate-encoding output: stdout=%q stderr=%q", args, stdout, stderr)
		}
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
	stdout, stderr, code := runNewTask(t, dir, `{}`)

	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	if stdout != "" || !strings.Contains(stderr, "invalid task input") || !strings.Contains(stderr, "missing required: title") {
		t.Fatalf("unexpected validation output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestDoneStateTransition(t *testing.T) {
	dir := setupErgo(t)

	// Create task
	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	// Set state to done
	_, _, code = putTaskInState(t, dir, taskID, "done", "")
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	if state := showTaskFields(t, dir, taskID)["state"]; state != "done" {
		t.Errorf("expected state=done, got %v", state)
	}
}

func TestBodyThenLifecyclePreservesExplicitBodyEdit(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	newBody := "updated\nbody\n"
	_, stderr, code := runErgo(t, dir, newBody, "body", taskID)
	if code != 0 {
		t.Fatalf("body failed: exit %d (stderr=%q)", code, stderr)
	}
	_, stderr, code = runErgo(t, dir, "", "done", taskID)
	if code != 0 {
		t.Fatalf("done failed: exit %d (stderr=%q)", code, stderr)
	}

	stdout = showTaskOutput(t, dir, taskID)
	if !strings.Contains(stdout, newBody) {
		t.Errorf("expected body=%q in show output: %s", newBody, stdout)
	}
	if state := showTaskFields(t, dir, taskID)["state"]; state != "done" {
		t.Errorf("expected state=done, got %v", state)
	}
}

func TestBlockKeepsBody(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	_, stderr, code := runErgo(t, dir, "", "block", taskID)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}

	stdout = showTaskOutput(t, dir, taskID)
	if !strings.Contains(stdout, "Test task") {
		t.Fatalf("expected body to remain unchanged: %s", stdout)
	}
	if state := showTaskFields(t, dir, taskID)["state"]; state != "blocked" {
		t.Fatalf("expected state=blocked, got %v", state)
	}
}

func TestRemovedJSONFlagExplainsMigration(t *testing.T) {
	dir := setupErgo(t)
	for _, args := range [][]string{{"--json", "list"}, {"list", "--json"}} {
		stdout, stderr, code := runErgo(t, dir, "", args...)
		if code == 0 || stdout != "" || !strings.Contains(stderr, "--json was removed in Ergo 3; rerun without it") {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestRemovedMutationCommandsGiveDirectHints(t *testing.T) {
	dir := setupErgo(t)
	for _, test := range []struct {
		args []string
		hint string
	}{
		{[]string{"set", "ABCDEF", `{}`}, "use claim, done, block, cancel, release, title, body, or move"},
		{[]string{"reopen", "ABCDEF"}, "use claim <id> --agent <identity> to resume closed work"},
	} {
		_, stderr, code := runErgo(t, dir, "", test.args...)
		if code == 0 || !strings.Contains(stderr, test.hint) {
			t.Fatalf("args=%v code=%d stderr=%q", test.args, code, stderr)
		}
	}
}

func TestSequenceAndUnsequenceOutput(t *testing.T) {
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

	stdout, stderr, code := runErgo(t, dir, "", "sequence", taskA, taskB)
	if code != 0 {
		t.Fatalf("sequence failed: exit %d stderr=%q", code, stderr)
	}
	if stdout != taskB+" depends on "+taskA+"\n" {
		t.Fatalf("sequence output = %q", stdout)
	}

	path := filepath.Join(dir, ".ergo", "plans.jsonl")
	beforeNoop, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runErgo(t, dir, "", "sequence", taskA, taskB)
	if code != 0 || stdout != "No dependency changes.\n" {
		t.Fatalf("sequence no-op: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	afterNoop, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeNoop, afterNoop) {
		t.Fatal("sequence no-op appended an event")
	}

	stdout, stderr, code = runErgo(t, dir, "", "unsequence", taskA, taskB)
	if code != 0 {
		t.Fatalf("unsequence failed: exit %d stderr=%q", code, stderr)
	}
	if stdout != taskB+" no longer depends on "+taskA+"\n" {
		t.Fatalf("unsequence output = %q", stdout)
	}

	stdout, stderr, code = runErgo(t, dir, "", "unsequence", taskA, taskB)
	if code != 0 || stdout != "No dependency changes.\n" {
		t.Fatalf("unsequence no-op: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	_, stderr, code = runErgo(t, dir, "", "sequence", "rm", taskA, taskB)
	if code == 0 || !strings.Contains(stderr, "use ergo unsequence") {
		t.Fatalf("sequence rm: code=%d stderr=%q", code, stderr)
	}
}

func TestSequenceAndUnsequenceChainsAreAtomic(t *testing.T) {
	dir := setupErgo(t)
	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task A failed: exit %d", code)
	}
	taskA := strings.TrimSpace(stdout)
	stdout, _, code = runNewTask(t, dir, `{"title":"Task B"}`)
	if code != 0 {
		t.Fatalf("new task B failed: exit %d", code)
	}
	taskB := strings.TrimSpace(stdout)

	path := filepath.Join(dir, ".ergo", "plans.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, code = runErgo(t, dir, "", "sequence", taskA, taskB, "UNKNOWN")
	if code == 0 {
		t.Fatal("sequence chain with unknown ID succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed sequence chain appended a partial edge")
	}

	if _, _, code = runErgo(t, dir, "", "sequence", taskA, taskB); code != 0 {
		t.Fatalf("sequence setup failed: exit %d", code)
	}
	before, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, code = runErgo(t, dir, "", "unsequence", taskA, taskB, "UNKNOWN")
	if code == 0 {
		t.Fatal("unsequence chain with unknown ID succeeded")
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed unsequence chain appended a partial unlink")
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

	stdout, _, code = runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if ids := outputIDs(stdout); !reflect.DeepEqual(ids, []string{taskA}) {
		t.Fatalf("expected only Task A ready, got %v in %q", ids, stdout)
	}

	stdout, stderr, code := runErgo(t, dir, "", "unsequence", taskA, taskB, taskC)
	if code != 0 {
		t.Fatalf("unsequence chain failed: exit %d stderr=%q", code, stderr)
	}
	wantUnsequence := taskB + " no longer depends on " + taskA + "\n" + taskC + " no longer depends on " + taskB + "\n"
	if stdout != wantUnsequence {
		t.Fatalf("unsequence chain output = %q, want %q", stdout, wantUnsequence)
	}
	stdout, _, code = runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready after unsequence failed: exit %d", code)
	}
	if ids := outputIDs(stdout); len(ids) != 3 {
		t.Fatalf("expected all tasks ready after unsequence, got %v in %q", ids, stdout)
	}
	if _, _, code = runErgo(t, dir, "", "sequence", taskA, taskB, taskC); code != 0 {
		t.Fatalf("resequence chain failed: exit %d", code)
	}

	_, _, code = putTaskInState(t, dir, taskA, "done", "")
	if code != 0 {
		t.Fatalf("set taskA done failed: exit %d", code)
	}
	stdout, _, code = runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if ids := outputIDs(stdout); !reflect.DeepEqual(ids, []string{taskB}) {
		t.Fatalf("expected only Task B ready, got %v in %q", ids, stdout)
	}

	_, _, code = putTaskInState(t, dir, taskB, "done", "")
	if code != 0 {
		t.Fatalf("set taskB done failed: exit %d", code)
	}
	stdout, _, code = runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if ids := outputIDs(stdout); !reflect.DeepEqual(ids, []string{taskC}) {
		t.Fatalf("expected only Task C ready, got %v in %q", ids, stdout)
	}
}

func TestPrune_DefaultIsDryRun(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = putTaskInState(t, dir, taskID, "done", "")
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

func TestPrune_DryRunNamesPrunedTasks(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = putTaskInState(t, dir, taskID, "done", "")
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	stdout, _, code = runErgo(t, dir, "", "prune")
	if code != 0 {
		t.Fatalf("prune dry-run failed: exit %d", code)
	}
	if !strings.Contains(stdout, taskID) || !strings.Contains(stdout, "preview") {
		t.Fatalf("expected prune preview to name %s: %s", taskID, stdout)
	}
}

func TestPrune_YesWrites(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = putTaskInState(t, dir, taskID, "done", "")
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
	_, _, code = putTaskInState(t, dir, taskB, "done", "")
	if code != 0 {
		t.Fatalf("set state=done failed: exit %d", code)
	}

	stdout = showTaskOutput(t, dir, taskA)
	if !strings.Contains(stdout, "depends on `"+taskB+"`") {
		t.Fatalf("expected dependency on %s in %s", taskB, stdout)
	}

	_, _, code = runErgo(t, dir, "", "prune", "--yes")
	if code != 0 {
		t.Fatalf("prune --yes failed: exit %d", code)
	}

	_, stderr, code := runErgo(t, dir, "", "show", taskB)
	if code == 0 || !strings.Contains(stderr, "pruned") {
		t.Fatalf("expected show to fail with pruned error, got code=%d stderr=%q", code, stderr)
	}

	stdout = showTaskOutput(t, dir, taskA)
	if strings.Contains(stdout, "depends on `"+taskB+"`") {
		t.Fatalf("expected dependency to disappear after prune: %s", stdout)
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
	_, _, code = putTaskInState(t, dir, taskDone, "done", "")
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
	_, _, code = putTaskInState(t, dir, taskBlocked, "blocked", "")
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
	_, _, code = putTaskInState(t, dir, taskID, "done", "")
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

func TestCompact_ConfirmsCompletion(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runErgo(t, dir, "", "compact")
	if code != 0 {
		t.Fatalf("compact failed: exit %d", code)
	}
	if stdout != "Compacted ergo plan.\n" {
		t.Errorf("unexpected compact output: %q", stdout)
	}
}

func TestLockWaitsThenSucceeds(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	lockPath := filepath.Join(dir, ".ergo", "lock")
	lockFile, err := os.OpenFile(lockPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()
	if err := lockTestFile(lockFile); err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	type result struct {
		stdout string
		stderr string
		code   int
	}
	done := make(chan result, 1)
	go func() {
		out, errOut, exit := runErgo(t, dir, "", "claim", "--agent", "agent-wait")
		done <- result{stdout: out, stderr: errOut, code: exit}
	}()

	select {
	case r := <-done:
		t.Fatalf("claim completed while lock was still held: code=%d stdout=%q stderr=%q", r.code, r.stdout, r.stderr)
	case <-time.After(150 * time.Millisecond):
	}

	if err := unlockTestFile(lockFile); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	select {
	case r := <-done:
		if r.code != 0 {
			t.Fatalf("claim after unlock failed: code=%d stdout=%q stderr=%q", r.code, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout, taskID) || strings.Contains(r.stderr, "lock busy") {
			t.Fatalf("unexpected claim result after unlock: stdout=%q stderr=%q", r.stdout, r.stderr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("claim did not complete after lock release")
	}
}

func TestPrune_ConcurrentRuns(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTask(t, dir, `{"title":"Task A"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)
	_, _, code = putTaskInState(t, dir, taskID, "done", "")
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
			out, errOut, exit := runErgo(t, dir, "", "prune", "--yes")
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
		`{"title":"Urgent task","state":"doing","claim":"agent-1"}`)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	taskID := strings.TrimSpace(stdout)
	if len(taskID) != 6 {
		t.Fatalf("expected 6-char task ID, got %q", taskID)
	}

	fields := showTaskFields(t, dir, taskID)
	if fields["state"] != "doing" || fields["claimed_by"] != "agent-1" {
		t.Errorf("unexpected fields: %v", fields)
	}
}

func TestCompact_PreservesShowOutput(t *testing.T) {
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

	// Mutate T1 across multiple dimensions through focused commands.
	_, stderr, code := runErgo(t, dir, "T1\\n\\n## v2\\nmore", "body", t1)
	if code != 0 {
		t.Fatalf("body %s failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = putTaskInState(t, dir, t1, "error", "agent-1")
	if code != 0 {
		t.Fatalf("prepare %s state=error failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = putTaskInState(t, dir, t1, "doing", "agent-1")
	if code != 0 {
		t.Fatalf("claim %s failed: exit %d stderr=%q", t1, code, stderr)
	}
	_, stderr, code = putTaskInState(t, dir, t1, "done", "")
	if code != 0 {
		t.Fatalf("done %s failed: exit %d stderr=%q", t1, code, stderr)
	}

	// Attach a result to T1 (ensures evidence fields survive compaction).
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "r1.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write result file failed: %v", err)
	}
	_, _, code = attachResultForTest(t, dir, t1, "docs/r1.md")
	if code != 0 {
		t.Fatalf("attach result failed: exit %d", code)
	}

	beforeT1 := showTaskOutput(t, dir, t1)
	beforeT2 := showTaskOutput(t, dir, t2)

	_, _, code = runErgo(t, dir, "", "compact")
	if code != 0 {
		t.Fatalf("compact failed: exit %d", code)
	}

	afterT1 := showTaskOutput(t, dir, t1)
	afterT2 := showTaskOutput(t, dir, t2)

	if !reflect.DeepEqual(beforeT1, afterT1) {
		t.Fatalf("show changed for %s after compact", t1)
	}
	if !reflect.DeepEqual(beforeT2, afterT2) {
		t.Fatalf("show changed for %s after compact", t2)
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

func TestFocusedCommandsCompose(t *testing.T) {
	dir := setupErgo(t)

	// Create task
	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	_, _, code = runErgo(t, dir, "", "title", taskID, "Updated title")
	if code != 0 {
		t.Fatalf("title failed: exit %d", code)
	}
	_, _, code = putTaskInState(t, dir, taskID, "doing", "agent-1")
	if code != 0 {
		t.Fatalf("claim failed: exit %d", code)
	}

	fields := showTaskFields(t, dir, taskID)
	if fields["title"] != "Updated title" || fields["state"] != "doing" || fields["claimed_by"] != "agent-1" {
		t.Errorf("unexpected fields: %v", fields)
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

	fields := showTaskFields(t, dir, taskID)
	if fields["claimed_by"] != agentID || fields["state"] != "doing" {
		t.Errorf("unexpected fields: %v", fields)
	}
}

func TestClaimIncludesTaskAndNextCommands(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	agentID := "sonnet@agent-host"
	stdout, stderr, code := runErgo(t, dir, "", "claim", taskID, "--agent", agentID)
	if code != 0 {
		t.Fatalf("claim failed: exit %d stderr=%q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "---\nid: \""+taskID+"\"\n") {
		t.Fatalf("claim ID is not in fixed front matter position: %s", stdout)
	}
	for _, want := range []string{"state: \"doing\"", "claimed_by: \"" + agentID + "\"", "# Test task", "Test task", "## Next"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("claim output missing %q: %s", want, stdout)
		}
	}
	for _, verb := range []string{"done", "block", "cancel", "release"} {
		if !strings.Contains(stdout, "`ergo "+verb+" "+taskID+"`") {
			t.Fatalf("claim output missing %s command: %s", verb, stdout)
		}
	}
}

func TestClaimOldestReadyIncludesNextCommands(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, code := runNewTaskWithBody(t, dir, "Test task", `{"title":"Test task"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	agentID := "sonnet@agent-host"
	stdout, stderr, code := runErgo(t, dir, "", "claim", "--agent", agentID)
	if code != 0 {
		t.Fatalf("claim oldest-ready failed: exit %d stderr=%q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "---\nid: \""+taskID+"\"\n") {
		t.Fatalf("claim ID is not in fixed front matter position: %s", stdout)
	}
	if !strings.Contains(stdout, "`ergo done "+taskID+"`") || !strings.Contains(stdout, "`ergo release "+taskID+"`") {
		t.Fatalf("unexpected next commands: %s", stdout)
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

	fields := showTaskFields(t, dir, taskID)
	shown := showTaskOutput(t, dir, taskID)
	if fields["title"] != "My Important Task" || !strings.Contains(shown, "This is the detailed body text") {
		t.Errorf("title/body were not distinct: fields=%v output=%s", fields, shown)
	}
}

func TestLifecycleOutputsTaskPostcondition(t *testing.T) {
	dir := setupErgo(t)

	// Create a task
	stdout, _, code := runNewTaskWithBody(t, dir, "Test body", `{"title":"Test"}`)
	if code != 0 {
		t.Fatalf("new task failed: exit %d", code)
	}
	taskID := strings.TrimSpace(stdout)

	stdout, _, code = runErgo(t, dir, "", "done", taskID)
	if code != 0 {
		t.Fatalf("done failed: exit %d", code)
	}

	output := strings.TrimSpace(stdout)
	if output != taskID+" done" {
		t.Errorf("expected lifecycle output %q, got %q", taskID+" done", output)
	}
}

// TestContainersRejectLifecycleAndClaim verifies that containers have no work state.
func TestContainersRejectLifecycleAndClaim(t *testing.T) {
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
		args    []string
		wantErr string
	}{
		{"state rejected", []string{"done", epicID}, "containers do not have state"},
		{"claim rejected", []string{"claim", epicID, "--agent", "agent-1"}, "containers cannot be claimed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runErgo(t, dir, "", tt.args...)
			if code == 0 {
				t.Errorf("expected error, got success")
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tt.wantErr, stderr)
			}
		})
	}
}

// TestListAllIncludesTerminalTasks verifies --all disables terminal-state filtering.
func TestListAllIncludesTerminalTasks(t *testing.T) {
	dir := setupErgo(t)

	// Create an epic with tasks in various states
	stdout, _, _ := runNewTask(t, dir, `{"title":"Test Epic"}`)
	epicID := strings.TrimSpace(stdout)

	// Create tasks: one done, one canceled, one todo
	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Done task","epic":"%s"}`, epicID))
	doneID := strings.TrimSpace(stdout)
	putTaskInState(t, dir, doneID, "done", "")

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Canceled task","epic":"%s"}`, epicID))
	canceledID := strings.TrimSpace(stdout)
	putTaskInState(t, dir, canceledID, "canceled", "")

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"Todo task","epic":"%s"}`, epicID))
	todoID := strings.TrimSpace(stdout)

	stdout, _, code := runErgo(t, dir, "", "list", "--all")
	if code != 0 {
		t.Fatalf("list --all failed: exit %d", code)
	}
	for _, id := range []string{doneID, canceledID, todoID} {
		if !strings.Contains(stdout, id) {
			t.Errorf("task %s missing from list --all: %s", id, stdout)
		}
	}
}

func TestListReadyFilters(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runNewTask(t, dir, `{"title":"Ready task"}`)
	readyID := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	putTaskInState(t, dir, doneID, "done", "")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Blocked task"}`)
	blockedID := strings.TrimSpace(stdout)
	putTaskInState(t, dir, blockedID, "blocked", "")

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
	if strings.Contains(stdout, blockedID) {
		t.Errorf("did not expect blocked task %s in output", blockedID)
	}
}

func TestListEpicFilters(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runNewTask(t, dir, `{"title":"Epic A"}`)
	epicA := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Epic B"}`)
	epicB := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"A1","epic":"%s"}`, epicA))
	taskA1 := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, fmt.Sprintf(`{"title":"B1","epic":"%s"}`, epicB))
	taskB1 := strings.TrimSpace(stdout)

	stdout, _, code := runErgo(t, dir, "", "list", "--epic", epicA)
	if code != 0 {
		t.Fatalf("list --epic failed: exit %d", code)
	}
	if !strings.Contains(stdout, taskA1) {
		t.Errorf("expected epic A task %s in output", taskA1)
	}
	if strings.Contains(stdout, taskB1) {
		t.Errorf("did not expect epic B task %s in output", taskB1)
	}
}

func TestListConflictingFlagsBeforeGraphRead(t *testing.T) {
	dir := setupErgo(t)

	_, stderr, code := runErgo(t, dir, "", "list", "--ready", "--all")
	if code == 0 {
		t.Fatalf("expected error for conflicting --ready and --all")
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
	_, _, _ = putTaskInState(t, dir, errorID, "error", "")

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
	_, _, _ = putTaskInState(t, dir, doneID, "done", "")

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

func TestListInvalidEpicReturnsError(t *testing.T) {
	dir := setupErgo(t)

	stdout, stderr, code := runErgo(t, dir, "", "list", "--epic", "ZZZZZZ")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "no such container: ZZZZZZ") {
		t.Fatalf("unexpected invalid epic result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// TestListReadyExcludesCompletedTasks verifies --ready hides done/canceled tasks in human output.
func TestListReadyExcludesCompletedTasks(t *testing.T) {
	dir := setupErgo(t)

	stdout, _, _ := runNewTask(t, dir, `{"title":"Ready task"}`)
	readyID := strings.TrimSpace(stdout)

	stdout, _, _ = runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	putTaskInState(t, dir, doneID, "done", "")

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
	_, _, _ = putTaskInState(t, dir, taskA2, "done", "")

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
	if stderr != "" {
		t.Errorf("expected no list warning on stderr, got: %s", stderr)
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
	_, _, _ = putTaskInState(t, dir, blockedID, "blocked", "")

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
	_, _, _ = putTaskInState(t, dir, doneID, "done", "")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Canceled task"}`)
	canceledID := strings.TrimSpace(stdout)
	_, _, _ = putTaskInState(t, dir, canceledID, "canceled", "")

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
	_, _, _ = putTaskInState(t, dir, blockedID, "blocked", "")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Error task"}`)
	errorID := strings.TrimSpace(stdout)
	_, _, _ = runErgo(t, dir, "", "claim", errorID, "--agent", "test@local")
	_, _, _ = putTaskInState(t, dir, errorID, "error", "")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Done task"}`)
	doneID := strings.TrimSpace(stdout)
	_, _, _ = putTaskInState(t, dir, doneID, "done", "")

	stdout, _, _ = runNewTask(t, dir, `{"title":"Canceled task"}`)
	canceledID := strings.TrimSpace(stdout)
	_, _, _ = putTaskInState(t, dir, canceledID, "canceled", "")

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
	_, _, _ = putTaskInState(t, dir, blockedID, "blocked", "")

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

func TestPlan_ReadableOutput_HappyPath(t *testing.T) {
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

	stdout, stderr, code := runPlan(t, dir, planInput, `{"title":"Add user auth"}`)
	if code != 0 {
		t.Fatalf("plan failed: exit %d, stderr=%s, stdout=%s", code, stderr, stdout)
	}
	ids := outputIDs(stdout)
	if len(ids) != 5 {
		t.Fatalf("expected container plus 4 child IDs, got %v in %s", ids, stdout)
	}
	if !strings.Contains(stdout, ids[0]+" - Add user auth") || !strings.Contains(stdout, "4 tasks, 0 dependencies") {
		t.Fatalf("unexpected plan summary: %s", stdout)
	}
	eventLog, err := os.ReadFile(getEventFilePath(dir))
	if err != nil {
		t.Fatalf("failed to read event log: %v", err)
	}
	if strings.Contains(string(eventLog), `"type":"new_epic"`) {
		t.Fatalf("expected plan to write unified new_task events, got log: %s", eventLog)
	}

	for _, expected := range []string{"Add auth middleware", "Add login endpoint", "Add signup endpoint", "Write integration tests"} {
		if !strings.Contains(stdout, " - "+expected) {
			t.Fatalf("expected child title %q in output: %s", expected, stdout)
		}
	}

	stdout, _, code = runErgo(t, dir, "", "list", "--ready")
	if code != 0 {
		t.Fatalf("list --ready failed: exit %d", code)
	}
	if readyIDs := outputIDs(stdout); len(readyIDs) != 5 {
		// Tree output includes the container plus its four ready children.
		t.Fatalf("expected container plus 4 ready tasks, got %v in %s", readyIDs, stdout)
	}
}

func TestPlan_FailuresReturnErrorsAndDoNotWritePartialState(t *testing.T) {
	tests := []struct {
		name           string
		planContent    string
		inlineJSON     string
		expectedStderr string
	}{
		{
			name:           "duplicate task title",
			planContent:    "# A\nfirst\n---\n# A\nsecond\n",
			inlineJSON:     `{"title":"Epic"}`,
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
			name:           "missing inline title",
			planContent:    "# A\nbody\n",
			inlineJSON:     `{}`,
			expectedStderr: "invalid plan input",
		},
		{
			name:           "malformed json",
			planContent:    "# A\nbody\n",
			inlineJSON:     `{"title":"Epic"`,
			expectedStderr: "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupErgo(t)

			stdout, stderr, code := runPlan(t, dir, tt.planContent, tt.inlineJSON)
			if code == 0 {
				t.Fatalf("expected non-zero exit for %s (stdout=%q stderr=%q)", tt.name, stdout, stderr)
			}
			if stdout != "" || !strings.Contains(stderr, tt.expectedStderr) {
				t.Fatalf("expected stderr to contain %q, got %q", tt.expectedStderr, stderr)
			}

			stdout, _, code = runErgo(t, dir, "", "list")
			if code != 0 {
				t.Fatalf("list failed: exit %d", code)
			}
			if !strings.Contains(stdout, "No tasks.") {
				t.Fatalf("expected no tasks after failed bulk-create: %s", stdout)
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
		_, _, _ = putTaskInState(t, dir, parentID, "doing", "agent-1")

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
		_, _, _ = putTaskInState(t, dir, parentID, "done", "")

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
		stdout, _, _ := runPlan(t, dir, "# B1\n\n---\n# B2\n", `{"title":"B"}`)
		ids := outputIDs(stdout)
		if len(ids) != 3 {
			t.Fatalf("unexpected plan output: %s", stdout)
		}
		bID, b1ID, b2ID := ids[0], ids[1], ids[2]

		// Create leaf A depending on container B
		stdout, _, _ = runNewTask(t, dir, `{"title":"A"}`)
		aID := strings.TrimSpace(stdout)
		// sequence bID aID → A depends on B (A comes after B)
		_, _, code := runErgo(t, dir, "", "sequence", bID, aID)
		if code != 0 {
			t.Fatalf("sequence B->A failed")
		}

		// A should be blocked while B's children are incomplete
		stdout, _, _ = runErgo(t, dir, "", "list", "--ready")
		if strings.Contains(stdout, aID) {
			t.Fatalf("expected A to be absent while container B has incomplete children: %s", stdout)
		}

		// Complete B's children → A should become ready
		putTaskInState(t, dir, b1ID, "done", "")
		putTaskInState(t, dir, b2ID, "done", "")

		stdout, _, _ = runErgo(t, dir, "", "list", "--ready")
		if !strings.Contains(stdout, aID) {
			t.Fatalf("expected A to be ready after all container B children done: %s", stdout)
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
		stdout, _, _ = runPlan(t, dir, "# T\n", `{"title":"A"}`)
		ids := outputIDs(stdout)
		if len(ids) != 2 {
			t.Fatalf("unexpected plan output: %s", stdout)
		}
		aID, tID := ids[0], ids[1]

		// Container A depends on leaf L
		// sequence lID aID → A depends on L (A comes after L)
		_, _, code := runErgo(t, dir, "", "sequence", lID, aID)
		if code != 0 {
			t.Fatalf("sequence L->A failed")
		}

		// T (inside A) should be blocked because A's dep L is not done
		stdout, _, _ = runErgo(t, dir, "", "list", "--ready")
		if strings.Contains(stdout, tID) {
			t.Fatalf("expected T to be absent because A depends on incomplete L: %s", stdout)
		}

		// Complete L → T should become ready
		putTaskInState(t, dir, lID, "done", "")
		stdout, _, _ = runErgo(t, dir, "", "list", "--ready")
		if !strings.Contains(stdout, tID) {
			t.Fatalf("expected T to be ready after L done: %s", stdout)
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
			stdout, stderr, code := runErgo(t, listDir, "", "list", "--all")
			if code != 0 {
				t.Fatalf("list --all failed: exit %d, stderr=%s", code, stderr)
			}
			if !strings.Contains(stdout, "├") && !strings.Contains(stdout, "└") {
				t.Fatalf("expected at least one container child after running fixture script %s: %s", filepath.Base(script), stdout)
			}
		})
	}
}
