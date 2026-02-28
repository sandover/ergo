// Purpose: Cover `RunPlan` command-layer behavior directly in the internal package.
// Exports: none.
// Role: Verifies successful plan mutation and structured JSON error output paths.
// Invariants: Success writes one epic+tasks+deps; validation/parse failures emit error JSON.
// Notes: Complements CLI integration tests by exercising internal command coverage.
package ergo

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPlan_SuccessCreatesGraph(t *testing.T) {
	repoDir := setupPlanRepo(t)
	restoreStdin := setStdin(t, `{
		"title":"  Epic  ",
		"body":"  Epic body  ",
		"tasks":[
			{"title":"  A  ","body":"  alpha  "},
			{"title":"  B  ","after":["  A  "]}
		]
	}`)
	defer restoreStdin()

	err := RunPlan(nil, GlobalOptions{StartDir: repoDir, Quiet: true})
	if err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}

	graph, err := loadGraph(filepath.Join(repoDir, dataDirName))
	if err != nil {
		t.Fatalf("loadGraph: %v", err)
	}
	if len(graph.Tasks) != 3 {
		t.Fatalf("expected 3 tasks total (1 epic + 2 tasks), got %d", len(graph.Tasks))
	}

	var epicID, taskAID, taskBID string
	for id, task := range graph.Tasks {
		if task.IsEpic {
			epicID = id
			if task.Title != "  Epic  " || task.Body != "  Epic body  " {
				t.Fatalf("expected preserved epic title/body, got %q / %q", task.Title, task.Body)
			}
			continue
		}
		if task.Title == "  A  " {
			taskAID = id
			if task.Body != "  alpha  " {
				t.Fatalf("expected preserved task A body, got %q", task.Body)
			}
		}
		if task.Title == "  B  " {
			taskBID = id
		}
	}
	if epicID == "" || taskAID == "" || taskBID == "" {
		t.Fatalf("missing expected epic/task IDs: epic=%q A=%q B=%q", epicID, taskAID, taskBID)
	}
	if depSet := graph.Deps[taskBID]; depSet == nil {
		t.Fatalf("expected deps for task B")
	} else if _, ok := depSet[taskAID]; !ok {
		t.Fatalf("expected B to depend on A; deps=%v", depSet)
	}
}

func TestRunPlan_JSONValidationError(t *testing.T) {
	repoDir := setupPlanRepo(t)
	restoreStdin := setStdin(t, `{"title":"Epic","tasks":[{"title":"A"},{"title":"A"}]}`)
	defer restoreStdin()

	stdout, err := captureStdout(t, func() error {
		return RunPlan(nil, GlobalOptions{StartDir: repoDir, JSON: true})
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var out map[string]interface{}
	if parseErr := json.Unmarshal([]byte(stdout), &out); parseErr != nil {
		t.Fatalf("failed to parse JSON error output: %v; stdout=%q", parseErr, stdout)
	}
	if out["error"] != "validation_failed" {
		t.Fatalf("expected validation_failed, got %v", out["error"])
	}
}

func TestRunPlan_JSONParseError(t *testing.T) {
	repoDir := setupPlanRepo(t)
	restoreStdin := setStdin(t, `{"title":"Epic","tasks":[{"title":"A"}],"unk":"x"}`)
	defer restoreStdin()

	stdout, err := captureStdout(t, func() error {
		return RunPlan(nil, GlobalOptions{StartDir: repoDir, JSON: true})
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	var out map[string]interface{}
	if parseErr := json.Unmarshal([]byte(stdout), &out); parseErr != nil {
		t.Fatalf("failed to parse JSON error output: %v; stdout=%q", parseErr, stdout)
	}
	if out["error"] != "parse_error" {
		t.Fatalf("expected parse_error, got %v", out["error"])
	}
}

func setupPlanRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	ergoDir := filepath.Join(repoDir, dataDirName)
	if err := os.MkdirAll(ergoDir, 0755); err != nil {
		t.Fatalf("mkdir .ergo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ergoDir, plansFileName), []byte{}, 0644); err != nil {
		t.Fatalf("create plans.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ergoDir, "lock"), []byte{}, 0644); err != nil {
		t.Fatalf("create lock: %v", err)
	}
	return repoDir
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read captured stdout: %v", readErr)
	}
	return string(out), runErr
}
