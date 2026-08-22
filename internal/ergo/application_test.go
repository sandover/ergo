package ergo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestApplication(t *testing.T) *Application {
	t.Helper()
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	return NewApplication(RepositoryOptions{StartDir: dir})
}

func requireApplicationError(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	got, ok := ApplicationErrorKind(err)
	if !ok || got != want {
		t.Fatalf("ApplicationErrorKind(%v) = %q, %v; want %q, true", err, got, ok, want)
	}
}

func TestApplicationCreateAndShow(t *testing.T) {
	app := newTestApplication(t)
	created, err := app.CreateTask(CreateTaskRequest{Title: "  Explain the boundary  ", Body: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	shown, err := app.Show(ShowRequest(created))
	if err != nil {
		t.Fatal(err)
	}
	if shown.Task.Title != "Explain the boundary" || shown.Task.Body != "Body" {
		t.Fatalf("shown task = %#v", shown.Task)
	}
}

func TestApplicationDraftCreationAndPromotion(t *testing.T) {
	app := newTestApplication(t)
	root, err := app.CreateTask(CreateTaskRequest{Title: "Plan this", Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.CreateTask(CreateTaskRequest{Title: "First step", EpicID: root.ID, Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	shown, err := app.Show(ShowRequest(child))
	if err != nil {
		t.Fatal(err)
	}
	if shown.Task.State != stateDraft || shown.Graph.IsReady(child.ID) {
		t.Fatalf("draft child = %#v, ready=%v", shown.Task, shown.Graph.IsReady(child.ID))
	}
	if got := selectPruneTargets(shown.Graph); len(got) != 0 {
		t.Fatalf("draft work was selected for pruning: %v", got)
	}
	if !shown.Graph.IsEpic(root.ID) {
		t.Fatal("draft root was not promoted to an epic")
	}
}

func TestApplicationBulkCreateDraftChildren(t *testing.T) {
	app := newTestApplication(t)
	path := filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte("# Configure\n\n---\n\n# Verify\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := app.CreateEpic(CreateEpicRequest{Title: "Staged epic", FilePath: path, Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Children) != 2 {
		t.Fatalf("created children = %#v", out.Children)
	}
	for _, child := range out.Children {
		shown, err := app.Show(ShowRequest{ID: child.ID})
		if err != nil {
			t.Fatal(err)
		}
		if shown.Task.State != stateDraft || shown.Graph.IsReady(child.ID) {
			t.Fatalf("child %s = %#v, ready=%v", child.ID, shown.Task, shown.Graph.IsReady(child.ID))
		}
	}
}

func TestApplicationLifecycleReturnsOutcomeWithoutRendering(t *testing.T) {
	app := newTestApplication(t)
	created, err := app.CreateTask(CreateTaskRequest{Title: "Finish this"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := app.Lifecycle(LifecycleRequest{
		Kind: "done", ID: created.ID, Messages: []string{"Implemented and checked."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Task.State != stateDone || !outcome.MessageSet {
		t.Fatalf("lifecycle outcome = %#v", outcome)
	}
}

func TestApplicationClaimIdentityIsRequestScoped(t *testing.T) {
	app := newTestApplication(t)
	first, err := app.CreateTask(CreateTaskRequest{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateTask(CreateTaskRequest{Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Claim(ClaimRequest{ID: first.ID, AgentID: "one@host"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Claim(ClaimRequest{ID: second.ID, AgentID: "two@host"}); err != nil {
		t.Fatal(err)
	}
	firstShown, _ := app.Show(ShowRequest(first))
	secondShown, _ := app.Show(ShowRequest(second))
	if firstShown.Task.ClaimedBy != "one@host" || secondShown.Task.ClaimedBy != "two@host" {
		t.Fatalf("claims = %q, %q", firstShown.Task.ClaimedBy, secondShown.Task.ClaimedBy)
	}
}

func TestApplicationWithRepositoryPreservesBaseWhenNoOverrideIsSet(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: dir}).WithRepository(RepositoryOptions{})
	if _, err := app.CreateTask(CreateTaskRequest{Title: "Uses base repository"}); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationErrorKinds(t *testing.T) {
	app := newTestApplication(t)
	_, err := app.CreateTask(CreateTaskRequest{})
	requireApplicationError(t, err, ErrorUsage)
	_, err = app.Show(ShowRequest{ID: "MISSING"})
	requireApplicationError(t, err, ErrorNotFound)

	created, err := app.CreateTask(CreateTaskRequest{Title: "Claim once"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Claim(ClaimRequest{ID: created.ID, AgentID: "one"}); err != nil {
		t.Fatal(err)
	}
	_, err = app.Claim(ClaimRequest{ID: created.ID, AgentID: "two"})
	requireApplicationError(t, err, ErrorConflict)

	requireApplicationError(t, classifyRepositoryError(ErrLockBusy), ErrorBusy)
	requireApplicationError(t, classifyRepositoryError(errors.New("disk failed")), ErrorInternal)
}

func TestApplicationShowClassifiesCorruptRepository(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, dataDirName, backlogFileName), []byte("{broken\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := NewApplication(RepositoryOptions{StartDir: dir}).Show(ShowRequest{ID: "ANY"})
	requireApplicationError(t, err, ErrorCorruption)
}
