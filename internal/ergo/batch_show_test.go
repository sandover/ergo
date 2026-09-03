package ergo

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchShowJSONMissingID(t *testing.T) {
	app := newTestApplication(t)
	created, err := app.CreateTask(CreateTaskRequest{Title: "Present", Body: "body bytes"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := app.BatchShow(BatchShowRequest{IDs: []string{created.ID, "ZZZZZZ"}, WithBody: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tasks) != 1 {
		t.Fatalf("tasks = %#v", out.Tasks)
	}
	if got := out.Tasks[created.ID].Body; got != "body bytes" {
		t.Fatalf("body = %q", got)
	}
	if len(out.Missing) != 1 || out.Missing[0] != "ZZZZZZ" {
		t.Fatalf("missing = %#v", out.Missing)
	}
	warnings := FormatBatchShowWarnings(out.Missing)
	if !strings.Contains(warnings, "unknown task id ZZZZZZ") {
		t.Fatalf("warnings = %q", warnings)
	}
	var output bytes.Buffer
	if err := RenderBatchShowJSON(&output, out); err != nil {
		t.Fatal(err)
	}
	var document struct {
		Version int                         `json:"version"`
		Tasks   map[string]taskJSONProjection `json:"tasks"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != 1 || len(document.Tasks) != 1 {
		t.Fatalf("document = %#v", document)
	}
}

func TestBatchShowEpicProjection(t *testing.T) {
	app := newTestApplication(t)
	path := filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte("# Step one\n\n---\n\n# Step two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	created, err := app.CreateEpic(CreateEpicRequest{Title: "Epic title", FilePath: path, Body: "epic body"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := app.BatchShow(BatchShowRequest{IDs: []string{created.ID}, WithBody: true})
	if err != nil {
		t.Fatal(err)
	}
	task := out.Tasks[created.ID]
	if task.Kind != "epic" || task.Title != "Epic title" || task.Body != "epic body" {
		t.Fatalf("epic projection = %#v", task)
	}
	if task.State == "" {
		t.Fatalf("epic state missing: %#v", task)
	}
}

func TestBatchShowBodyMatchesShowBody(t *testing.T) {
	app := newTestApplication(t)
	body := "literal body\n"
	created, err := app.CreateTask(CreateTaskRequest{Title: "Task", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	showBody, err := app.ShowBody(ShowBodyRequest(created))
	if err != nil {
		t.Fatal(err)
	}
	out, err := app.BatchShow(BatchShowRequest{IDs: []string{created.ID}, WithBody: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Tasks[created.ID].Body != showBody.Body {
		t.Fatalf("batch body = %q, show body = %q", out.Tasks[created.ID].Body, showBody.Body)
	}
}
