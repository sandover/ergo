package ergo

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failingBodyWriter struct{ err error }

func (w failingBodyWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRenderShowBodyPreservesExactBytes(t *testing.T) {
	for _, body := range []string{"", "no trailing newline", "one newline\n", "two newlines\n\n"} {
		t.Run(body, func(t *testing.T) {
			var output bytes.Buffer
			if err := RenderShowBody(&output, ShowBodyOutcome{Body: body}); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != body {
				t.Fatalf("body bytes = %q, want %q", got, body)
			}
		})
	}
}

func TestApplicationShowBodyDoesNotReadJournal(t *testing.T) {
	app := newTestApplication(t)
	created, err := app.CreateTask(CreateTaskRequest{Title: "Task", Body: "literal body\n"})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ergoDir(app.repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, journalFileName), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outcome, err := app.ShowBody(ShowBodyRequest(created))
	if err != nil || outcome.Body != "literal body\n" {
		t.Fatalf("show body = %#v, %v", outcome, err)
	}
}

func TestRenderShowBodyPropagatesWriteError(t *testing.T) {
	want := errors.New("write failed")
	err := RenderShowBody(failingBodyWriter{err: want}, ShowBodyOutcome{Body: "body"})
	if !errors.Is(err, want) {
		t.Fatalf("RenderShowBody() error = %v, want %v", err, want)
	}
}

func TestApplicationShowBodyLeafRoundTripIsLossless(t *testing.T) {
	for _, body := range []string{"", "no trailing newline", "trailing newline\n"} {
		t.Run(body, func(t *testing.T) {
			app := newTestApplication(t)
			created, err := app.CreateTask(CreateTaskRequest{Title: "Task", Body: body})
			if err != nil {
				t.Fatal(err)
			}
			projected, err := app.ShowBody(ShowBodyRequest(created))
			if err != nil {
				t.Fatal(err)
			}
			var pipe bytes.Buffer
			if err := RenderShowBody(&pipe, projected); err != nil {
				t.Fatal(err)
			}
			if _, err := app.UpdateBody(UpdateBodyRequest{ID: created.ID, Body: pipe.Bytes()}); err != nil {
				t.Fatal(err)
			}
			roundTripped, err := app.ShowBody(ShowBodyRequest(created))
			if err != nil {
				t.Fatal(err)
			}
			if roundTripped.Body != body {
				t.Fatalf("round-trip body = %q, want %q", roundTripped.Body, body)
			}
		})
	}
}

func TestApplicationShowBodyEpicOmitsChildren(t *testing.T) {
	app := newTestApplication(t)
	epic, err := app.CreateTask(CreateTaskRequest{Title: "Epic", Body: "epic body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateTask(CreateTaskRequest{
		Title: "Child", EpicID: epic.ID, Body: "child body",
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err := app.ShowBody(ShowBodyRequest(epic))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Body != "epic body" {
		t.Fatalf("epic body = %q", outcome.Body)
	}
}

func TestApplicationShowBodyUnknownAndPrunedFailures(t *testing.T) {
	app := newTestApplication(t)
	_, err := app.ShowBody(ShowBodyRequest{ID: "UNKNOWN"})
	requireApplicationError(t, err, ErrorNotFound)

	created, err := app.CreateTask(CreateTaskRequest{Title: "Prune me"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Lifecycle(LifecycleRequest{Kind: "done", ID: created.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Prune(PruneRequest{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	_, err = app.ShowBody(ShowBodyRequest(created))
	requireApplicationError(t, err, ErrorNotFound)
}

func TestRenderClaimContractUnaffectedByBodyProjection(t *testing.T) {
	task := &Task{ID: "ABC123", Title: "Claimed", State: stateDoing, ClaimedBy: "agent"}
	graph := &Graph{
		Tasks: map[string]*Task{task.ID: task},
		Deps:  map[string]map[string]struct{}{task.ID: {}},
	}
	var output bytes.Buffer
	RenderClaim(&output, ClaimOutcome{Graph: graph, Task: task, ProjectDir: "/project"}, false)
	text := output.String()
	for _, want := range []string{
		`id: "ABC123"`, `claimed_by: "agent"`, "# Claimed", "## Next",
		"- `ergo done ABC123`", "- `ergo block ABC123`",
		"- `ergo cancel ABC123`", "- `ergo release ABC123`",
	} {
		if !bytes.Contains(output.Bytes(), []byte(want)) {
			t.Fatalf("claim output missing %q:\n%s", want, text)
		}
	}
}
