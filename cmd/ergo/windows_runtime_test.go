//go:build windows

// Purpose: Prove the Windows executable handles normal lifecycle work in a path with spaces.
// Exports: none (test coverage only).
// Role: Exercise the user-facing binary on the Windows CI runner.
// Invariants: --dir resolves the intended project and lifecycle output remains usable.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsRuntimeSmokeInPathWithSpaces(t *testing.T) {
	workspace := t.TempDir()
	project := filepath.Join(workspace, "ergo project with spaces")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}

	if _, stderr, code := runErgo(t, workspace, "", "--dir", project, "init"); code != 0 {
		t.Fatalf("init failed: exit %d, stderr=%s", code, stderr)
	}

	stdout, stderr, code := runErgo(t, workspace, "", "--dir", project, "new", "task", "Windows path smoke test")
	if code != 0 {
		t.Fatalf("new task failed: exit %d, stderr=%s", code, stderr)
	}
	id := strings.TrimSpace(stdout)
	if id == "" {
		t.Fatal("new task returned an empty task ID")
	}

	if _, stderr, code := runErgo(t, workspace, "", "--dir", project, "claim", id, "--agent", "windows-ci"); code != 0 {
		t.Fatalf("claim failed: exit %d, stderr=%s", code, stderr)
	}
	if _, stderr, code := runErgo(t, workspace, "", "--dir", project, "done", id, "-m", "Windows runtime smoke test"); code != 0 {
		t.Fatalf("done failed: exit %d, stderr=%s", code, stderr)
	}

	stdout, stderr, code = runErgo(t, workspace, "", "--dir", project, "list", "--all", "--json")
	if code != 0 {
		t.Fatalf("list --all --json failed: exit %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, id) {
		t.Fatalf("list output did not contain %s: %s", id, stdout)
	}
}
