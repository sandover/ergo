package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sandover/ergo/internal/ergo"
)

func TestShowTaskBodyWritesExactStoredBody(t *testing.T) {
	for _, body := range []string{"", "no trailing newline", "trailing newline\n", "two\n\n"} {
		t.Run(body, func(t *testing.T) {
			dir := setupErgo(t)
			var stdout, stderr string
			var code int
			if body == "" {
				stdout, stderr, code = runNewTask(t, dir, "Task")
			} else {
				stdout, stderr, code = runNewTaskWithBody(t, dir, body, "Task")
			}
			if code != 0 {
				t.Fatalf("create: code=%d stderr=%q", code, stderr)
			}
			id := strings.TrimSpace(stdout)
			stdout, stderr, code = runErgo(t, dir, "", "show", id, "--body")
			if code != 0 || stderr != "" {
				t.Fatalf("show --body: code=%d stderr=%q", code, stderr)
			}
			if stdout != body {
				t.Fatalf("stdout = %q, want %q", stdout, body)
			}
		})
	}
}

func TestShowEpicBodyOmitsChildren(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runNewTaskWithBody(t, dir, "epic body", "Epic")
	if code != 0 {
		t.Fatalf("create epic: code=%d stderr=%q", code, stderr)
	}
	epicID := strings.TrimSpace(stdout)
	if _, stderr, code := runNewTaskWithBody(t, dir, "child body", "Child", "--epic", epicID); code != 0 {
		t.Fatalf("create child: code=%d stderr=%q", code, stderr)
	}
	stdout, stderr, code = runErgo(t, dir, "", "show", epicID, "--body")
	if code != 0 || stderr != "" || stdout != "epic body" {
		t.Fatalf("show epic --body: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestShowBodyUnknownRetainsShowFailure(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runErgo(t, dir, "", "show", "UNKNOWN", "--body")
	if code == 0 || stdout != "" || !strings.Contains(stderr, "unknown task id UNKNOWN") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestShowBodyRoundTripsThroughBodyCommand(t *testing.T) {
	for _, body := range []string{"", "no trailing newline", "trailing newline\n"} {
		t.Run(body, func(t *testing.T) {
			dir := setupErgo(t)
			stdout, stderr, code := runNewTask(t, dir, "Task")
			if code != 0 {
				t.Fatalf("create: code=%d stderr=%q", code, stderr)
			}
			id := strings.TrimSpace(stdout)
			if body != "" {
				if _, stderr, code := runErgo(t, dir, body, "body", id); code != 0 {
					t.Fatalf("seed body: code=%d stderr=%q", code, stderr)
				}
			}

			projected, stderr, code := runErgo(t, dir, "", "show", id, "--body")
			if code != 0 {
				t.Fatalf("project body: code=%d stderr=%q", code, stderr)
			}

			var output, errors bytes.Buffer
			streams := Streams{
				In: strings.NewReader(projected), Out: &output, Err: &errors,
				StdinTerminal: false, Width: 80,
			}
			root := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{}), streams, "test")
			if code := runCommand(root, []string{"--dir", dir, "body", id}, streams); code != 0 {
				t.Fatalf("round-trip body: code=%d stderr=%q", code, errors.String())
			}

			roundTripped, stderr, code := runErgo(t, dir, "", "show", id, "--body")
			if code != 0 || roundTripped != body {
				t.Fatalf("round-trip: code=%d body=%q want=%q stderr=%q", code, roundTripped, body, stderr)
			}
		})
	}
}
