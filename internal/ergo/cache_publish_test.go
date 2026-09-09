// Purpose: Prove automatic cache publication is invisible and self-maintaining.
// Coverage: creation and refresh thresholds, concurrent readers, replacement,
// compact invalidation, ignore preservation, and publication failure fallback.
// Invariants: cache maintenance never changes authoritative command behavior.
package ergo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestViewGraphCreatesCacheAndConcurrentReadersKeepItValid(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestLargeTask(t, "TASK01")})
	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			graph, err := repository.ViewGraph()
			if err == nil && graph.Tasks["TASK01"] == nil {
				err = os.ErrInvalid
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	cachePath := filepath.Join(dir, dataDirName, cacheFileName)
	cacheFile, err := os.Open(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCache(cacheFile); err != nil {
		cacheFile.Close()
		t.Fatal(err)
	}
	if err := cacheFile.Close(); err != nil {
		t.Fatal(err)
	}
	_, read, err := repository.loadWithRead()
	if err != nil {
		t.Fatal(err)
	}
	if !read.cacheHit {
		t.Fatal("second graph load did not use the published cache")
	}
}

func TestBacklogCacheRefreshesAfterRecordThreshold(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
	writeCacheForRepository(t, repository)
	path := filepath.Join(dir, dataDirName, backlogFileName)
	for i := 0; i < cacheRefreshRecords; i++ {
		now := time.Date(2026, 9, 9, 13, 0, i, 0, time.UTC)
		event, err := newEvent(eventTitle, now, TitleUpdateEvent{ID: "TASK01", Title: "Revision " + strings.Repeat("x", i%8), TS: formatTime(now)})
		if err != nil {
			t.Fatal(err)
		}
		if err := repositoryAppendEvents(path, []Event{event}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.ViewGraph(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(dir, dataDirName, cacheFileName))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCache(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.source.Bytes != info.Size() || decoded.source.Records != cacheRefreshRecords+1 {
		t.Fatalf("refreshed source = %+v, size = %d", decoded.source, info.Size())
	}
}

func TestCompactInvalidatesBacklogCache(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
	writeCacheForRepository(t, repository)
	if _, err := repository.Compact(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, dataDirName, cacheFileName)); !os.IsNotExist(err) {
		t.Fatalf("cache after compact: %v", err)
	}
}

func TestCacheIgnoreRulesPreserveExistingContent(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, dataDirName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, ".gitignore")
	if err := os.WriteFile(path, []byte("/journal.jsonl"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCacheIgnored(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := ensureCacheIgnored(dataDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "/journal.jsonl\n# Ergo performance cache\n/cache.jsonl\n/cache.tmp-*\n"
	if string(data) != want {
		t.Fatalf("ignore contents = %q, want %q", data, want)
	}
	for _, rule := range []string{cacheIgnoreFileRule, cacheIgnoreTempRule} {
		if bytes.Count(data, []byte(rule)) != 1 {
			t.Fatalf("ignore rule %q was duplicated: %q", rule, data)
		}
	}
}

func TestCachePublicationFailureDoesNotFailView(t *testing.T) {
	dir, repository := cacheTestRepository(t, []Event{cacheTestLargeTask(t, "TASK01")})
	cachePath := filepath.Join(dir, dataDirName, cacheFileName)
	if err := os.Mkdir(cachePath, 0755); err != nil {
		t.Fatal(err)
	}
	graph, err := repository.ViewGraph()
	if err != nil {
		t.Fatal(err)
	}
	if graph.Tasks["TASK01"] == nil {
		t.Fatal("cache publication failure changed graph output")
	}
}

func TestCacheRefreshThresholds(t *testing.T) {
	base := cacheSource{Identity: "id", Bytes: 10, Records: 10}
	read := eventLogRead{cacheGraph: newGraph(), cacheSource: base}
	if cacheRefreshNeeded(read) {
		t.Fatal("small missing cache should not publish")
	}
	read.cacheSource.Bytes = cacheCreateBytes
	if !cacheRefreshNeeded(read) {
		t.Fatal("creation byte threshold did not publish")
	}
	read.cacheHit = true
	read.cacheBase = base
	read.cacheSource = base
	if cacheRefreshNeeded(read) {
		t.Fatal("unchanged cache should not refresh")
	}
	read.cacheSource.Records += cacheRefreshRecords
	read.cacheSource.Bytes++
	if !cacheRefreshNeeded(read) {
		t.Fatal("record threshold did not refresh")
	}
}

func cacheTestLargeTask(t *testing.T, id string) Event {
	t.Helper()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	event, err := newEvent(eventNewTask, now, NewTaskEvent{ID: id, UUID: strings.ToLower(id), State: stateTodo,
		Title: "Large", Body: strings.Repeat("x", cacheCreateBytes), CreatedAt: formatTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
