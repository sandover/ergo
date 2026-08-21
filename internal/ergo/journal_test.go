package ergo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitializeCreatesAndRepairsJournal(t *testing.T) {
	project := t.TempDir()
	outcome, err := InitializeRepository(project)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outcome.Path, journalFileName)
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("journal stat = %#v, %v", info, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outcome, err = InitializeRepository(project)
	if err != nil || outcome.Status != "repaired" {
		t.Fatalf("repair = %#v, %v", outcome, err)
	}
}

func TestJournalReaderRejectsCompleteCorruptionAndRepairsTruncatedTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, journalFileName)
	entry := newJournalEntry("ABCDEF", "created", "", "", time.Now().UTC())
	data, err := marshalJournal([]JournalEntry{entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(`{"version":1`)...), 0644); err != nil {
		t.Fatal(err)
	}
	read, err := readJournal(path)
	if err != nil || !read.truncatedTail || len(read.entries) != 1 {
		t.Fatalf("truncated read = %#v, %v", read, err)
	}
	repository := Repository{dir: dir, journalPath: path, io: systemRepositoryIO()}
	if err := repository.appendJournal([]JournalEntry{newJournalEntry("ABCDEF", "claim", "agent@host", "", time.Now().UTC())}); err != nil {
		t.Fatal(err)
	}
	read, err = readJournal(path)
	if err != nil || len(read.entries) != 2 || read.truncatedTail {
		t.Fatalf("repaired read = %#v, %v", read, err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJournal(path); err == nil || !strings.Contains(err.Error(), "unsupported journal version") {
		t.Fatalf("complete corruption error = %v", err)
	}
}

func TestCompactMigratesLegacyEvidenceOnce(t *testing.T) {
	project := t.TempDir()
	if _, err := InitializeRepository(project); err != nil {
		t.Fatal(err)
	}
	var repository Repository
	if err := repository.Open(RepositoryOptions{StartDir: project}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{ID: "ABCDEF", UUID: "uuid", State: stateDone, Title: "Task", CreatedAt: formatTime(now)}),
		mustNewEvent("message", now.Add(time.Second), MessageEvent{TaskID: "ABCDEF", Kind: "done", Text: "Finished", TS: formatTime(now.Add(time.Second))}),
		mustNewEvent("result", now.Add(2*time.Second), ResultEvent{TaskID: "ABCDEF", Summary: "Evidence", Path: "evidence.txt", Sha256AtAttach: "abc", TS: formatTime(now.Add(2 * time.Second))}),
	}
	if err := repositoryAppendEvents(repository.eventsPath, events); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Compact(); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Compact(); err != nil {
		t.Fatal(err)
	}
	read, err := readJournal(repository.journalPath)
	if err != nil || len(read.entries) != 2 {
		t.Fatalf("journal = %#v, %v", read.entries, err)
	}
	graph, err := repository.View()
	if err != nil || len(graph.Tasks["ABCDEF"].Results) != 1 || len(graph.Tasks["ABCDEF"].Messages) != 1 {
		t.Fatalf("hydrated evidence = %#v, %v", graph.Tasks["ABCDEF"], err)
	}
}

func TestCompactFinishesAfterJournalOnlyReplacement(t *testing.T) {
	project := t.TempDir()
	if _, err := InitializeRepository(project); err != nil {
		t.Fatal(err)
	}
	var repository Repository
	if err := repository.Open(RepositoryOptions{StartDir: project}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	events := []Event{
		mustNewEvent("new_task", now, NewTaskEvent{ID: "ABCDEF", UUID: "uuid", State: stateDone, Title: "Task", CreatedAt: formatTime(now)}),
		mustNewEvent("result", now.Add(time.Second), ResultEvent{TaskID: "ABCDEF", Summary: "Evidence", Path: "evidence.txt", Sha256AtAttach: "abc", TS: formatTime(now.Add(time.Second))}),
	}
	if err := repositoryAppendEvents(repository.eventsPath, events); err != nil {
		t.Fatal(err)
	}
	graph, err := repository.load()
	if err != nil {
		t.Fatal(err)
	}
	journal := compactJournal(mergeLegacyJournal(nil, graph), graph)
	if err := repository.replaceJournal(journal); err != nil {
		t.Fatal(err)
	}
	// This is the on-disk state left if compact replaces the journal but not the
	// legacy backlog. A later compact must finish without duplicating evidence.
	if _, err := repository.Compact(); err != nil {
		t.Fatal(err)
	}
	read, err := readJournal(repository.journalPath)
	if err != nil || len(read.entries) != 1 || read.entries[0].Kind != "result" {
		t.Fatalf("recovered journal = %#v, %v", read.entries, err)
	}
}

func TestUpdateReportsJournalFailureAfterBacklogChange(t *testing.T) {
	repository, path := newInjectedRepository(t, systemRepositoryIO())
	calls := 0
	repository.io.postWrite = func() error {
		calls++
		if calls == 2 {
			return errors.New("journal unavailable")
		}
		return nil
	}
	now := time.Now().UTC()
	_, err := repository.UpdateWithJournal(func(*Graph) ([]Event, []JournalEntry, error) {
		return []Event{repositoryTestTaskEvent(t, "PART01")}, []JournalEntry{newJournalEntry("PART01", "created", "", "", now)}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "backlog changed") {
		t.Fatalf("partial error = %v", err)
	}
	events, readErr := readEvents(path)
	if readErr != nil || len(events) != 1 {
		t.Fatalf("backlog events = %d, %v", len(events), readErr)
	}
}

func TestPruneReportsAndRemovesSelectedJournalEntries(t *testing.T) {
	project := t.TempDir()
	if _, err := InitializeRepository(project); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: project})
	created, err := app.CreateTask(CreateTaskRequest{Title: "Finished task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Lifecycle(LifecycleRequest{Kind: "done", ID: created.ID, Messages: []string{"Finished"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Result(ResultRequest{ID: created.ID, Text: "Verified"}); err != nil {
		t.Fatal(err)
	}
	preview, err := app.Prune(PruneRequest{})
	if err != nil || preview.JournalEntries != 3 {
		t.Fatalf("preview = %#v, %v", preview, err)
	}
	applied, err := app.Prune(PruneRequest{Confirm: true})
	if err != nil || applied.JournalEntries != 3 {
		t.Fatalf("applied = %#v, %v", applied, err)
	}
	read, err := readJournal(filepath.Join(project, dataDirName, journalFileName))
	if err != nil || len(read.entries) != 0 {
		t.Fatalf("retained journal = %#v, %v", read.entries, err)
	}
}

func TestLifecycleAndResultsWriteOnlyMeaningfulJournalEntries(t *testing.T) {
	project := t.TempDir()
	if _, err := InitializeRepository(project); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: project})
	created, err := app.CreateTask(CreateTaskRequest{Title: "Investigate failure"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.UpdateTitle(UpdateTitleRequest{ID: created.ID, Title: "Investigate constraint"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Claim(ClaimRequest{ID: created.ID, AgentID: "model@host"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Lifecycle(LifecycleRequest{Kind: "fail", ID: created.ID, Messages: []string{"Constraint disproved"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Lifecycle(LifecycleRequest{Kind: "fail", ID: created.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Result(ResultRequest{ID: created.ID, Text: "Reproduced on Windows"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Result(ResultRequest{ID: created.ID, Text: "Second independent reproduction"}); err != nil {
		t.Fatal(err)
	}

	read, err := readJournal(filepath.Join(project, dataDirName, journalFileName))
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, entry := range read.entries {
		kinds = append(kinds, entry.Kind)
	}
	if strings.Join(kinds, ",") != "created,claim,fail,result,result" {
		t.Fatalf("journal kinds = %v", kinds)
	}
	if read.entries[2].Text != "Constraint disproved" || read.entries[2].Agent != "model@host" {
		t.Fatalf("failed entry = %#v", read.entries[2])
	}
	shown, err := app.Show(ShowRequest(created))
	if err != nil || shown.Task.State != stateFailed {
		t.Fatalf("result changed lifecycle: %#v, %v", shown.Task, err)
	}
	before := len(read.entries)
	if _, err := app.Result(ResultRequest{ID: created.ID, Text: "two\nlines"}); err == nil {
		t.Fatal("multiline result text succeeded")
	}
	read, _ = readJournal(filepath.Join(project, dataDirName, journalFileName))
	if len(read.entries) != before {
		t.Fatal("invalid result appended a journal entry")
	}
}
