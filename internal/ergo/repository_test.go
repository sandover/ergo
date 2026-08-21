// Tests for the repository persistence boundary and deterministic I/O failures.
package ergo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepositoryOpenAndViewUseExistingSupportedLog(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, dataDirName)
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, plansFileName)
	now := time.Now().UTC()
	if err := appendEvents(path, []Event{mustNewEvent("new_task", now, NewTaskEvent{
		ID: "T1", UUID: "uuid-1", State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
	})}); err != nil {
		t.Fatal(err)
	}

	var repository Repository
	if err := repository.Open(GlobalOptions{StartDir: project}); err != nil {
		t.Fatal(err)
	}
	if repository.eventsPath != path {
		t.Fatalf("events path = %q, want %q", repository.eventsPath, path)
	}
	graph, err := repository.View()
	if err != nil {
		t.Fatal(err)
	}
	if graph.Tasks["T1"] == nil {
		t.Fatal("view did not load T1")
	}
}

func TestRepositoryOpenRejectsEveryConflictingLog(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, dataDirName)
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, name := range []string{backlogFileName, plansFileName, oldEventsFileName} {
		path := filepath.Join(dir, name)
		paths = append(paths, path)
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	var repository Repository
	err := repository.Open(GlobalOptions{StartDir: project})
	if err == nil {
		t.Fatal("expected conflicting logs to fail")
	}
	for _, path := range paths {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("error %q does not identify %q", err, path)
		}
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error lacks reconciliation guidance: %q", err)
	}
}

func TestRepositoryUpdateSupportsDeterministicShortWrites(t *testing.T) {
	repository, path := newInjectedRepository(t, systemRepositoryIO())
	writes := 0
	repository.io.write = func(file *os.File, data []byte) (int, error) {
		writes++
		if len(data) > 1 {
			data = data[:1]
		}
		return file.Write(data)
	}
	now := time.Now().UTC()
	err := repository.update(func(*Graph) ([]Event, error) {
		return []Event{mustNewEvent("new_task", now, NewTaskEvent{
			ID: "T1", UUID: "uuid-1", State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
		})}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if writes < 2 {
		t.Fatalf("expected repeated writes, got %d", writes)
	}
	events, err := readEvents(path)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
}

func TestRepositoryUpdateReturnsPreappliedCandidateWithoutReload(t *testing.T) {
	repository, _ := newInjectedRepository(t, systemRepositoryIO())
	event := repositoryTestTaskEvent(t, "CAND01")
	outcome, err := repository.Update(func(*Graph) ([]Event, error) {
		return []Event{event}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Graph == nil || outcome.Graph.Tasks["CAND01"] == nil {
		t.Fatal("update did not return the preapplied candidate")
	}
}

func TestRepositoryUpdateKeepsCallbackMutationOutOfCandidate(t *testing.T) {
	repository, _ := newInjectedRepository(t, systemRepositoryIO())
	event := repositoryTestTaskEvent(t, "CAND01")
	outcome, err := repository.Update(func(graph *Graph) ([]Event, error) {
		graph.Tasks["LEAK01"] = &Task{ID: "LEAK01", State: stateTodo}
		return []Event{event}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Graph.Tasks["LEAK01"] != nil {
		t.Fatal("callback-only mutation leaked into the validated candidate")
	}
}

func TestRepositoryUpdateRejectsInvalidBatchBeforeWrite(t *testing.T) {
	repository, path := newInjectedRepository(t, systemRepositoryIO())
	before, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = repository.Update(func(*Graph) ([]Event, error) {
		return []Event{mustNewEvent("state", now, StateEvent{
			ID: "MISSING", NewState: stateDone, TS: formatTime(now),
		})}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "orphan state event") {
		t.Fatalf("invalid batch error = %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatal("invalid batch changed repository bytes")
	}
}

func TestRepositoryUpdateSupportsDeterministicWriteFailure(t *testing.T) {
	repository, _ := newInjectedRepository(t, systemRepositoryIO())
	injected := errors.New("injected write failure")
	repository.io.write = func(*os.File, []byte) (int, error) {
		return 0, injected
	}
	err := repository.update(func(*Graph) ([]Event, error) {
		return []Event{repositoryTestTaskEvent(t, "WRITE1")}, nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected failure", err)
	}
}

func TestRepositoryUpdateSupportsDeterministicOpenFailure(t *testing.T) {
	repository, _ := newInjectedRepository(t, systemRepositoryIO())
	injected := errors.New("injected open failure")
	repository.io.openFile = func(string, int, os.FileMode) (*os.File, error) {
		return nil, injected
	}
	err := repository.update(func(*Graph) ([]Event, error) {
		return []Event{repositoryTestTaskEvent(t, "OPEN01")}, nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected failure", err)
	}
}

func TestRepositoryUpdateSupportsDeterministicPostWriteFailure(t *testing.T) {
	repository, path := newInjectedRepository(t, systemRepositoryIO())
	injected := errors.New("injected post-write failure")
	repository.io.postWrite = func() error { return injected }
	err := repository.update(func(*Graph) ([]Event, error) {
		return []Event{repositoryTestTaskEvent(t, "POST01")}, nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected failure", err)
	}
	if info, statErr := os.Stat(path); statErr != nil || info.Size() == 0 {
		t.Fatalf("post-write hook did not run after bytes were written: info=%v err=%v", info, statErr)
	}
}

func repositoryTestTaskEvent(t *testing.T, id string) Event {
	t.Helper()
	now := time.Now().UTC()
	return mustNewEvent("new_task", now, NewTaskEvent{
		ID: id, UUID: "uuid-" + id, State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
	})
}

func TestRepositoryIOCanInjectSyncFailure(t *testing.T) {
	repository, path := newInjectedRepository(t, systemRepositoryIO())
	injected := errors.New("injected sync failure")
	repository.io.sync = func(*os.File) error { return injected }
	now := time.Now().UTC()
	err := repository.update(func(*Graph) ([]Event, error) {
		return []Event{mustNewEvent("new_task", now, NewTaskEvent{
			ID: "T1", UUID: "uuid-1", State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
		})}, nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("sync error = %v, want injected failure", err)
	}
	if info, statErr := os.Stat(path); statErr != nil || info.Size() == 0 {
		t.Fatalf("sync failure was not injected after the record write: info=%v err=%v", info, statErr)
	}
}

func TestRepositoryUpdateRepairsPartialTransactionBeforeRecoveryAppend(t *testing.T) {
	repository, path := newInjectedRepository(t, systemRepositoryIO())
	injected := errors.New("injected interrupted write")
	writes := 0
	repository.io.write = func(file *os.File, data []byte) (int, error) {
		writes++
		if writes == 1 {
			return file.Write(data[:len(data)/2])
		}
		return 0, injected
	}
	now := time.Now().UTC()
	event := mustNewEvent("new_task", now, NewTaskEvent{
		ID: "T1", UUID: "uuid-1", State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
	})
	if err := repository.update(func(*Graph) ([]Event, error) {
		return []Event{event}, nil
	}); !errors.Is(err, injected) {
		t.Fatalf("write error = %v, want injected failure", err)
	}

	repository.io = systemRepositoryIO()
	if err := repository.update(func(graph *Graph) ([]Event, error) {
		if len(graph.Tasks) != 0 {
			t.Fatalf("partial transaction replayed: %#v", graph.Tasks)
		}
		return []Event{event}, nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.TrimSpace(string(data)), "\n") != 0 {
		t.Fatalf("repaired log should contain exactly one record: %s", data)
	}
	events, err := readEvents(path)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
}

func newInjectedRepository(t *testing.T, io repositoryIO) (*Repository, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, backlogFileName)
	repository := &Repository{}
	if err := repository.openAt(dir, GlobalOptions{}, io); err != nil {
		t.Fatal(err)
	}
	return repository, path
}

func TestUpdateScansEachLogOnceBeforeAppending(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	var repository Repository
	if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
		t.Fatal(err)
	}

	eventReads, journalReads := 0, 0
	io := systemRepositoryIO()
	io.inspectEvents = func(path string) (eventLogRead, error) {
		eventReads++
		return inspectEventLog(path)
	}
	io.readJournal = func(path string) (journalRead, error) {
		journalReads++
		return readJournal(path)
	}
	repository.io = io

	now := time.Now().UTC()
	_, err := repository.UpdateWithJournal(func(*Graph) ([]Event, []JournalEntry, error) {
		event := mustNewEvent("new_task", now, NewTaskEvent{
			ID: "ABCDEF", UUID: "uuid", State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
		})
		journal := newJournalEntry("ABCDEF", "created", "", "", now)
		return []Event{event}, []JournalEntry{journal}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if eventReads != 1 || journalReads != 1 {
		t.Fatalf("reads = backlog %d, journal %d; want one each", eventReads, journalReads)
	}
}

func TestGraphOnlyReadAndUpdateDoNotReadJournal(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	var repository Repository
	if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
		t.Fatal(err)
	}
	io := systemRepositoryIO()
	eventReads := 0
	io.inspectEvents = func(path string) (eventLogRead, error) {
		eventReads++
		return inspectEventLog(path)
	}
	io.readJournal = func(string) (journalRead, error) {
		return journalRead{}, errors.New("journal should not be read")
	}
	repository.io = io

	if _, err := repository.ViewGraph(); err != nil {
		t.Fatal(err)
	}
	if eventReads != 1 {
		t.Fatalf("graph-only view read backlog %d times, want 1", eventReads)
	}
	eventReads = 0
	now := time.Now().UTC()
	if _, err := repository.Update(func(*Graph) ([]Event, error) {
		return []Event{mustNewEvent("new_task", now, NewTaskEvent{
			ID: "GRAPH1", UUID: "uuid", State: stateTodo, Title: "Task", CreatedAt: formatTime(now),
		})}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if eventReads != 1 {
		t.Fatalf("graph-only update read backlog %d times, want 1", eventReads)
	}
}

func TestJournalFreeListIgnoresJournalCorruption(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: dir})
	if _, err := app.CreateTask(CreateTaskRequest{Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(dir, dataDirName, journalFileName)
	if err := os.WriteFile(journalPath, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.List(ListRequest{OmitJournal: true}); err != nil {
		t.Fatalf("journal-free list failed: %v", err)
	}
	if _, err := app.List(ListRequest{}); err == nil {
		t.Fatal("evidence-bearing list accepted corrupt journal")
	}
}
