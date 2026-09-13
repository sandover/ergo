// Purpose: Prove checkpoint-plus-tail loading matches authoritative replay.
// Coverage: hits, ordinary appends, source replacement/truncation, corrupt
// caches, malformed tails, and whole-source repair coordinates.
// Invariants: every accelerated-path failure falls back to the full loader.
package ergo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBacklogCacheLoadsCheckpointAndAppendedTail(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
	writeCacheForRepository(t, repository)

	graph, read, err := repository.loadWithRead()
	if err != nil {
		t.Fatal(err)
	}
	if !read.cacheHit || graph.Tasks["TASK01"] == nil {
		t.Fatalf("cache hit = %v, graph = %+v", read.cacheHit, graph.Tasks)
	}

	if err := repositoryAppendEvents(filepath.Join(dir, dataDirName, backlogFileName), []Event{cacheTestNewTask(t, "TASK02", "Second")}); err != nil {
		t.Fatal(err)
	}
	graph, read, err = repository.loadWithRead()
	if err != nil {
		t.Fatal(err)
	}
	if !read.cacheHit || graph.Tasks["TASK01"] == nil || graph.Tasks["TASK02"] == nil {
		t.Fatalf("tail replay cache hit = %v, graph = %+v", read.cacheHit, graph.Tasks)
	}
	if read.log.recordCount != 2 || read.log.lineCount != 2 || len(read.log.events) != 1 || read.log.validBytes != read.log.source.Bytes {
		t.Fatalf("tail coordinates = records %d lines %d bytes %d source %+v", read.log.recordCount, read.log.lineCount, read.log.validBytes, read.log.source)
	}
}

func TestBacklogCacheMissesAfterSourceReplacementOrTruncation(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
		writeCacheForRepository(t, repository)
		path := filepath.Join(dir, dataDirName, backlogFileName)
		replacement := filepath.Join(dir, dataDirName, "replacement.jsonl")
		if err := writeCacheTestEvents(replacement, []Event{cacheTestNewTask(t, "TASK02", "Replacement")}); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
		graph, read, err := repository.loadWithRead()
		if err != nil {
			t.Fatal(err)
		}
		if read.cacheHit || graph.Tasks["TASK01"] != nil || graph.Tasks["TASK02"] == nil {
			t.Fatalf("replacement used stale cache: hit=%v tasks=%v", read.cacheHit, graph.Tasks)
		}
	})

	t.Run("truncation", func(t *testing.T) {
		dir, repository := cacheTestRepository(t, []Event{
			cacheTestNewTask(t, "TASK01", "First"), cacheTestNewTask(t, "TASK02", "Second"),
		})
		writeCacheForRepository(t, repository)
		path := filepath.Join(dir, dataDirName, backlogFileName)
		if err := writeCacheTestEvents(path, []Event{cacheTestNewTask(t, "TASK03", "Short")}); err != nil {
			t.Fatal(err)
		}
		graph, read, err := repository.loadWithRead()
		if err != nil {
			t.Fatal(err)
		}
		if read.cacheHit || len(graph.Tasks) != 1 || graph.Tasks["TASK03"] == nil {
			t.Fatalf("truncation used stale cache: hit=%v tasks=%v", read.cacheHit, graph.Tasks)
		}
	})
}

func TestBacklogCacheFailuresFallBackToAuthoritativeReplay(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
	writeCacheForRepository(t, repository)
	cachePath := filepath.Join(dir, dataDirName, cacheFileName)
	if err := os.WriteFile(cachePath, []byte("broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	graph, read, err := repository.loadWithRead()
	if err != nil {
		t.Fatal(err)
	}
	if read.cacheHit || graph.Tasks["TASK01"] == nil {
		t.Fatalf("corrupt cache changed full replay: hit=%v tasks=%v", read.cacheHit, graph.Tasks)
	}

	writeCacheForRepository(t, repository)
	path := filepath.Join(dir, dataDirName, backlogFileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{broken\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.loadWithRead()
	if err == nil || !strings.Contains(err.Error(), "invalid JSON in events log") {
		t.Fatalf("authoritative malformed-tail error = %v", err)
	}
}

func TestBacklogCachePreservesInterruptedTailRepairCoordinates(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
	writeCacheForRepository(t, repository)
	path := filepath.Join(dir, dataDirName, backlogFileName)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"transaction"`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, read, err := repository.loadWithRead()
	if err != nil {
		t.Fatal(err)
	}
	if !read.cacheHit || !read.log.truncatedTail || read.log.validBytes != before.Size() {
		t.Fatalf("repair coordinates = hit %v truncated %v valid %d want %d", read.cacheHit, read.log.truncatedTail, read.log.validBytes, before.Size())
	}
}

func TestBacklogCacheAcceptsUndetectablePrefixEdit(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
	writeCacheForRepository(t, repository)
	path := filepath.Join(dir, dataDirName, backlogFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(data, []byte(`"title":"First"`), []byte(`"title":"Other"`), 1)
	if bytes.Equal(data, changed) {
		t.Fatal("test did not change represented prefix")
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt(changed, 0); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	graph, read, err := repository.loadWithRead()
	if err != nil {
		t.Fatal(err)
	}
	if !read.cacheHit || graph.Tasks["TASK01"].Title != "First" {
		t.Fatalf("represented prefix was consulted: hit=%v task=%+v", read.cacheHit, graph.Tasks["TASK01"])
	}
}

func cacheTestRepository(t *testing.T, events []Event) (string, *Repository) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, dataDirName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeCacheTestEvents(filepath.Join(dataDir, backlogFileName), events); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, journalFileName), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	repository := &Repository{}
	if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
		t.Fatal(err)
	}
	return dir, repository
}

func writeCacheForRepository(t *testing.T, repository *Repository) {
	t.Helper()
	read, err := inspectEventLog(repository.eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	raw := read.snapshot
	if raw == nil {
		raw = newGraph()
	}
	raw, err = replayEventsOntoRaw(raw, read.events)
	if err != nil {
		t.Fatal(err)
	}
	source := read.source
	if source.Identity == "" {
		t.Fatal("backlog was not eligible for a checkpoint")
	}
	data, err := marshalCache(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository.dir, cacheFileName), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeCacheTestEvents(path string, events []Event) error {
	var output []byte
	for _, event := range events {
		line, err := marshalTransaction([]Event{event})
		if err != nil {
			return err
		}
		output = append(output, line...)
	}
	return os.WriteFile(path, output, 0644)
}

func cacheTestNewTask(t *testing.T, id, title string) Event {
	t.Helper()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	event, err := newEvent(eventNewTask, now, NewTaskEvent{ID: id, UUID: strings.ToLower(id), State: stateTodo,
		Title: title, CreatedAt: formatTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
