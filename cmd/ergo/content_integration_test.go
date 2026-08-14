// Purpose: Exercise direct title and body edits through the compiled CLI.
// Exports: none.
// Role: Black-box coverage for literal stdin, empty bodies, and containers.
// Invariants: same-value edits append no events and affect no other fields.
// Invariants: body refuses interactive stdin and explains the pipe form.
package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

func TestTitleCommandOnTaskAndContainer(t *testing.T) {
	dir := setupErgo(t)
	containerID := createLifecycleTask(t, dir)
	stdout, stderr, code := runNewTask(t, dir, "Child", "--epic", containerID)
	if code != 0 {
		t.Fatalf("create child failed: %s", stderr)
	}
	childID := strings.TrimSpace(stdout)
	for _, id := range []string{childID, containerID} {
		stdout, stderr, code = runErgo(t, dir, "", "title", id, "Renamed "+id)
		if code != 0 {
			t.Fatalf("title failed: %s", stderr)
		}
		if stdout != id+" - Renamed "+id+"\n" {
			t.Fatalf("unexpected title output: %s", stdout)
		}
		before := countEventLines(t, dir)
		stdout, stderr, code = runErgo(t, dir, "", "title", id, "Renamed "+id)
		if code != 0 || stdout != id+" - Renamed "+id+" (title unchanged)\n" {
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

func TestBodyAppendIsLiteralForTaskAndEpic(t *testing.T) {
	dir := setupErgo(t)
	epicID := createLifecycleTask(t, dir)
	stdout, stderr, code := runNewTask(t, dir, "Child", "--epic", epicID)
	if code != 0 {
		t.Fatalf("create child failed: %s", stderr)
	}
	childID := strings.TrimSpace(stdout)

	for _, id := range []string{childID, epicID} {
		if _, stderr, code := runErgo(t, dir, "start", "body", id); code != 0 {
			t.Fatalf("set initial body for %s: %s", id, stderr)
		}
		stdout, stderr, code := runErgo(t, dir, "+next", "body", id, "--append")
		if code != 0 || stdout != id+" body: 10 bytes\n" {
			t.Fatalf("append for %s: stdout=%q stderr=%q code=%d", id, stdout, stderr, code)
		}
		shown, stderr, code := runErgo(t, dir, "", "show", id, "--body")
		if code != 0 || shown != "start+next" {
			t.Fatalf("literal body for %s: body=%q stderr=%q code=%d", id, shown, stderr, code)
		}

		before := countEventLines(t, dir)
		stdout, stderr, code = runErgoWithEmptyPipe(t, dir, "body", id, "--append")
		if code != 0 || stdout != id+" body unchanged\n" {
			t.Fatalf("empty append for %s: stdout=%q stderr=%q code=%d", id, stdout, stderr, code)
		}
		if countEventLines(t, dir) != before {
			t.Fatalf("empty append for %s wrote an event", id)
		}
	}
}

func TestConcurrentBodyAppendsPreserveEveryCommittedWrite(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	if _, stderr, code := runErgo(t, dir, "base", "body", id); code != 0 {
		t.Fatalf("set initial body: %s", stderr)
	}

	const appendCount = 8
	errors := make(chan string, appendCount)
	var group sync.WaitGroup
	for index := 0; index < appendCount; index++ {
		fragment := fmt.Sprintf("[%d]", index)
		group.Add(1)
		go func() {
			defer group.Done()
			cmd := exec.Command(ergoBinary, "body", id, "--append")
			cmd.Dir = dir
			cmd.Stdin = strings.NewReader(fragment)
			if output, err := cmd.CombinedOutput(); err != nil {
				errors <- fmt.Sprintf("append %q failed: %v: %s", fragment, err, output)
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}

	body, stderr, code := runErgo(t, dir, "", "show", id, "--body")
	if code != 0 {
		t.Fatalf("show body failed: %s", stderr)
	}
	if !strings.HasPrefix(body, "base") {
		t.Fatalf("body lost its initial bytes: %q", body)
	}
	for index := 0; index < appendCount; index++ {
		fragment := fmt.Sprintf("[%d]", index)
		if strings.Count(body, fragment) != 1 {
			t.Fatalf("body does not contain %q exactly once: %q", fragment, body)
		}
	}
}

func TestBodyCommandLiteralEmptyAndTTY(t *testing.T) {
	dir := setupErgo(t)
	id := createLifecycleTask(t, dir)
	body := "## Goal\n- Preserve this literally\n"
	stdout, stderr, code := runErgo(t, dir, body, "body", id)
	if code != 0 || stdout != id+" body: 34 bytes\n" {
		t.Fatalf("body failed: stdout=%s stderr=%s", stdout, stderr)
	}
	shown := showTaskOutput(t, dir, id)
	if !strings.Contains(shown, body) {
		t.Fatalf("show output does not contain body %q: %s", body, shown)
	}

	stdout, stderr, code = runErgoWithEmptyPipe(t, dir, "body", id)
	if code != 0 || stdout != id+" body: 0 bytes\n" {
		t.Fatalf("empty body failed: stdout=%s stderr=%s", stdout, stderr)
	}
	shown = showTaskOutput(t, dir, id)
	if strings.Contains(shown, "Preserve this literally") {
		t.Fatalf("empty pipe did not clear body: %s", shown)
	}

	_, stderr, code = runErgo(t, dir, "", "body", id)
	if code == 0 || !strings.Contains(stderr, "printf") || !strings.Contains(stderr, "| ergo body "+id) {
		t.Fatalf("expected TTY pipe guidance, code=%d stderr=%q", code, stderr)
	}
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
