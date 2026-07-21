// Purpose: Exercise direct title and body edits through the compiled CLI.
// Exports: none.
// Role: Black-box coverage for literal stdin, empty bodies, and containers.
// Invariants: same-value edits append no events and affect no other fields.
// Invariants: body refuses interactive stdin and explains the pipe form.
package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestTitleCommandOnTaskAndContainer(t *testing.T) {
	dir := setupErgo(t)
	containerID := createLifecycleTask(t, dir)
	stdout, stderr, code := runNewTask(t, dir, `{"title":"Child","epic":"`+containerID+`"}`)
	if code != 0 {
		t.Fatalf("create child failed: %s", stderr)
	}
	childID := strings.TrimSpace(stdout)
	for _, id := range []string{childID, containerID} {
		stdout, stderr, code = runErgo(t, dir, "", "--json", "title", id, "Renamed "+id)
		if code != 0 {
			t.Fatalf("title failed: %s", stderr)
		}
		if !strings.Contains(stdout, `"updated_fields":["title"]`) {
			t.Fatalf("unexpected title output: %s", stdout)
		}
		before := countEventLines(t, dir)
		stdout, stderr, code = runErgo(t, dir, "", "--json", "title", id, "Renamed "+id)
		if code != 0 || !strings.Contains(stdout, `"updated_fields":[]`) {
			t.Fatalf("title no-op failed: stdout=%s stderr=%s", stdout, stderr)
		}
		if countEventLines(t, dir) != before {
			t.Fatal("same title appended an event")
		}
	}
	_, stderr, code = runErgo(t, dir, "", "title", childID, "   ")
	if code == 0 || !strings.Contains(stderr, "title cannot be empty") {
		t.Fatalf("blank title was accepted: code=%d stderr=%q", code, stderr)
	}
}

func TestBodyCommandLiteralEmptyAndTTY(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	body := "## Goal\n- Preserve this literally\n"
	stdout, stderr, code := runErgo(t, dir, body, "--json", "body", id)
	if code != 0 || !strings.Contains(stdout, `"updated_fields":["body"]`) {
		t.Fatalf("body failed: stdout=%s stderr=%s", stdout, stderr)
	}
	shown := showTaskJSON(t, dir, id)
	if shown["body"] != body {
		t.Fatalf("body = %q, want %q", shown["body"], body)
	}

	stdout, stderr, code = runErgoWithEmptyPipe(t, dir, "--json", "body", id)
	if code != 0 || !strings.Contains(stdout, `"updated_fields":["body"]`) {
		t.Fatalf("empty body failed: stdout=%s stderr=%s", stdout, stderr)
	}
	shown = showTaskJSON(t, dir, id)
	if shown["body"] != "" {
		t.Fatalf("empty pipe did not clear body: %v", shown["body"])
	}

	_, stderr, code = runErgo(t, dir, "", "body", id)
	if code == 0 || !strings.Contains(stderr, "printf") || !strings.Contains(stderr, "| ergo body "+id) {
		t.Fatalf("expected TTY pipe guidance, code=%d stderr=%q", code, stderr)
	}
}

func showTaskJSON(t *testing.T, dir, id string) map[string]any {
	t.Helper()
	stdout, stderr, code := runErgo(t, dir, "", "--json", "show", id)
	if code != 0 {
		t.Fatalf("show failed: %s", stderr)
	}
	var task map[string]any
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatal(err)
	}
	return task
}

func runErgoWithEmptyPipe(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(ergoBinary, args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout.String(), stderr.String(), exitErr.ExitCode()
	}
	t.Fatalf("run ergo: %v", err)
	return "", "", -1
}
