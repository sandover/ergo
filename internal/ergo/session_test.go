package ergo

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionSecondListDoesNotRescan(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: dir})
	var repo Repository
	if err := repo.Open(RepositoryOptions{StartDir: dir}); err != nil {
		t.Fatal(err)
	}
	reads := 0
	io := systemRepositoryIO()
	io.inspectEvents = func(path string) (eventLogRead, error) {
		reads++
		return inspectEventLog(path)
	}
	repo.io = io

	session := &Session{app: app, repo: repo}
	if err := session.reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.List(ListRequest{OmitJournal: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.List(ListRequest{OmitJournal: true}); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("inspectEvents called %d times, want 1", reads)
	}
}

func TestSessionWarmClaimDoesNotRescan(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: dir})
	if _, err := app.CreateTask(CreateTaskRequest{Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	var repo Repository
	if err := repo.Open(RepositoryOptions{StartDir: dir}); err != nil {
		t.Fatal(err)
	}
	reads := 0
	io := systemRepositoryIO()
	io.inspectEvents = func(path string) (eventLogRead, error) {
		reads++
		return inspectEventLog(path)
	}
	repo.io = io
	session := &Session{app: app, repo: repo}
	if err := session.reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Claim(ClaimRequest{AgentID: "agent@test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.List(ListRequest{OmitJournal: true}); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("inspectEvents called %d times after warm claim, want 1", reads)
	}
}

func TestSessionReloadsAfterOutOfBandAppend(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: dir})
	session, err := app.OpenSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.List(ListRequest{OmitJournal: true}); err != nil {
		t.Fatal(err)
	}
	eventsPath, err := selectEventsPath(filepath.Join(dir, dataDirName))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	event := mustNewEvent("new_task", now, NewTaskEvent{
		ID: "OUTBND", UUID: "uuid-out", State: stateTodo, Title: "Out of band", CreatedAt: formatTime(now),
	})
	if err := repositoryAppendEvents(eventsPath, []Event{event}); err != nil {
		t.Fatal(err)
	}
	out, err := session.List(ListRequest{OmitJournal: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Graph.Tasks["OUTBND"] == nil {
		t.Fatal("session did not reload after out-of-band append")
	}
}
