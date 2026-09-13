// Purpose: Measure the internal costs behind large-backlog CLI operations.
// Coverage: log inspection, replay, journal reads, graph derivation, and
// list/show rendering for full and compacted physical representations.
// Run: go test ./internal/ergo -run '^$' -bench LargeBacklog -benchmem.
package ergo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type largeBacklogBenchmarkData struct {
	dir         string
	eventsPath  string
	journalPath string
	events      []Event
	graph       *Graph
	journal     []JournalEntry
}

func BenchmarkLargeBacklogCacheWarm(b *testing.B) {
	for _, taskCount := range []int{1500, 5000, 15000} {
		b.Run(fmt.Sprintf("tasks-%d", taskCount), func(b *testing.B) {
			dir := b.TempDir()
			if _, err := WriteLargeBacklogFixture(dir, LargeBacklogFixtureOptions{TaskCount: taskCount}); err != nil {
				b.Fatal(err)
			}
			var repository Repository
			if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
				b.Fatal(err)
			}
			if _, err := repository.ViewGraph(); err != nil {
				b.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, dataDirName, cacheFileName)); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := repository.ViewGraph(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFixedStateHistoryCacheWarm(b *testing.B) {
	for _, transactions := range []int{30000, 100000, 300000} {
		b.Run(fmt.Sprintf("transactions-%d", transactions), func(b *testing.B) {
			dir := b.TempDir()
			if err := writeFixedStateHistoryFixture(filepath.Join(dir, dataDirName), transactions); err != nil {
				b.Fatal(err)
			}
			var repository Repository
			if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
				b.Fatal(err)
			}
			if _, err := repository.ViewGraph(); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := repository.ViewGraph(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLargeBacklogCacheTransitions(b *testing.B) {
	setup := func(b *testing.B) (string, *Repository) {
		b.Helper()
		dir := b.TempDir()
		if _, err := WriteLargeBacklogFixture(dir, LargeBacklogFixtureOptions{}); err != nil {
			b.Fatal(err)
		}
		var repository Repository
		if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
			b.Fatal(err)
		}
		return dir, &repository
	}
	b.Run("first-creation", func(b *testing.B) {
		dir, repository := setup(b)
		cachePath := filepath.Join(dir, dataDirName, cacheFileName)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := repository.ViewGraph(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("tail-64", func(b *testing.B) {
		_, repository := setup(b)
		if _, err := repository.ViewGraph(); err != nil {
			b.Fatal(err)
		}
		if err := appendBenchmarkTitleTail(repository.eventsPath, 64); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := repository.ViewGraph(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("refresh-256", func(b *testing.B) {
		dir, repository := setup(b)
		if _, err := repository.ViewGraph(); err != nil {
			b.Fatal(err)
		}
		cachePath := filepath.Join(dir, dataDirName, cacheFileName)
		baseline, err := os.ReadFile(cachePath)
		if err != nil {
			b.Fatal(err)
		}
		if err := appendBenchmarkTitleTail(repository.eventsPath, cacheRefreshRecords); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			// Restore the old checkpoint so every sample folds in the same tail.
			if err := os.WriteFile(cachePath, baseline, 0600); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := repository.ViewGraph(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("invalid-fallback", func(b *testing.B) {
		dir, repository := setup(b)
		if _, err := repository.ViewGraph(); err != nil {
			b.Fatal(err)
		}
		cachePath := filepath.Join(dir, dataDirName, cacheFileName)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			if err := os.WriteFile(cachePath, []byte("broken\n"), 0600); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			if _, err := repository.ViewGraph(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func appendBenchmarkTitleTail(path string, count int) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		at := time.Date(2026, 9, 10, 12, 0, i, 0, time.UTC)
		event, err := newEvent(eventTitle, at, TitleUpdateEvent{ID: "T00001", Title: fmt.Sprintf("Tail revision %03d", i), TS: formatTime(at)})
		if err != nil {
			file.Close()
			return err
		}
		line, err := marshalTransaction([]Event{event})
		if err != nil {
			file.Close()
			return err
		}
		if _, err := file.Write(line); err != nil {
			file.Close()
			return err
		}
	}
	return file.Close()
}

func writeFixedStateHistoryFixture(dataDir string, transactions int) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(dataDir, backlogFileName))
	if err != nil {
		return err
	}
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for i := 0; i < transactions; i++ {
		var event Event
		if i == 0 {
			event, err = newEvent(eventNewTask, now, NewTaskEvent{ID: "TASK01", UUID: "fixed-state", State: stateTodo,
				Title: "Fixed state", CreatedAt: formatTime(now)})
		} else {
			at := now.Add(time.Duration(i) * time.Second)
			event, err = newEvent(eventTitle, at, TitleUpdateEvent{ID: "TASK01", Title: fmt.Sprintf("Fixed state %06d", i), TS: formatTime(at)})
		}
		if err != nil {
			file.Close()
			return err
		}
		line, err := marshalTransaction([]Event{event})
		if err != nil {
			file.Close()
			return err
		}
		if _, err := file.Write(line); err != nil {
			file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dataDir, journalFileName), nil, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "lock"), nil, 0644)
}

func setupLargeBacklogBenchmark(b *testing.B, backlogMode, journalMode string) largeBacklogBenchmarkData {
	b.Helper()
	dir := b.TempDir()
	if _, err := WriteLargeBacklogFixture(dir, LargeBacklogFixtureOptions{
		BacklogMode: backlogMode, JournalMode: journalMode,
	}); err != nil {
		b.Fatal(err)
	}
	eventsPath := filepath.Join(dir, dataDirName, backlogFileName)
	journalPath := filepath.Join(dir, dataDirName, journalFileName)
	read, err := inspectEventLog(eventsPath)
	if err != nil {
		b.Fatal(err)
	}
	graph := read.snapshot
	if graph == nil {
		graph, err = replayEvents(read.events)
		if err != nil {
			b.Fatal(err)
		}
	} else if len(read.events) > 0 {
		graph, err = replayEventsOnto(graph, read.events)
		if err != nil {
			b.Fatal(err)
		}
	}
	jRead, err := readJournal(journalPath)
	if err != nil {
		b.Fatal(err)
	}
	return largeBacklogBenchmarkData{
		dir: dir, eventsPath: eventsPath, journalPath: journalPath,
		events: read.events, graph: graph, journal: jRead.entries,
	}
}

func BenchmarkLargeBacklogInspectEventLog(b *testing.B) {
	for _, mode := range []string{LargeBacklogModeFull, LargeBacklogModeCompacted} {
		b.Run(mode, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, mode, LargeBacklogModeFull)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := inspectEventLog(data.eventsPath); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLargeBacklogReplay(b *testing.B) {
	for _, mode := range []string{LargeBacklogModeFull, LargeBacklogModeCompacted} {
		b.Run(mode, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, mode, LargeBacklogModeFull)
			read, err := inspectEventLog(data.eventsPath)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if read.snapshot == nil {
					if _, err := replayEvents(data.events); err != nil {
						b.Fatal(err)
					}
					continue
				}
				if _, err := replayEventsOnto(read.snapshot, read.events); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLargeBacklogReadJournal(b *testing.B) {
	for _, mode := range []string{LargeBacklogModeFull, LargeBacklogModeCompacted} {
		b.Run(mode, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, LargeBacklogModeFull, mode)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := readJournal(data.journalPath); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLargeBacklogViewWithJournal(b *testing.B) {
	for _, representation := range []struct {
		name, backlog, journal string
	}{
		{"full-full", LargeBacklogModeFull, LargeBacklogModeFull},
		{"compacted-full", LargeBacklogModeCompacted, LargeBacklogModeFull},
		{"compacted-compacted", LargeBacklogModeCompacted, LargeBacklogModeCompacted},
	} {
		b.Run(representation.name, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, representation.backlog, representation.journal)
			var repository Repository
			if err := repository.Open(RepositoryOptions{StartDir: data.dir}); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := repository.ViewWithJournal(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLargeBacklogGraphDerivation(b *testing.B) {
	for _, mode := range []string{LargeBacklogModeFull, LargeBacklogModeCompacted} {
		b.Run(mode, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, mode, LargeBacklogModeFull)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				data.graph.derivedCached = false
				data.graph.completeByID = nil
				data.graph.blockersByID = nil
				data.graph.readyByID = nil
				data.graph.epicStateByID = nil
				b.StartTimer()
				data.graph.prepareDerivedQueries()
			}
		})
	}
}

func BenchmarkLargeBacklogListRendering(b *testing.B) {
	for _, mode := range []string{LargeBacklogModeFull, LargeBacklogModeCompacted} {
		b.Run(mode, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, mode, LargeBacklogModeFull)
			data.graph.prepareDerivedQueries()
			all, active, ready := collectListTasks(data.graph)
			outcome := ListOutcome{
				Options: ListOptions{ShowAll: true}, Graph: data.graph,
				Roots:    buildListRoots(data.graph, true, false, ""),
				AllTasks: all, ActiveTasks: active, ReadyTasks: ready,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				RenderList(io.Discard, outcome, false, 120)
			}
		})
	}
}

func BenchmarkLargeBacklogShowRendering(b *testing.B) {
	for _, mode := range []string{LargeBacklogModeFull, LargeBacklogModeCompacted} {
		b.Run(mode, func(b *testing.B) {
			data := setupLargeBacklogBenchmark(b, mode, LargeBacklogModeFull)
			data.graph.prepareDerivedQueries()
			task := data.graph.Tasks["T00001"]
			if task == nil {
				b.Fatal("fixture show task missing")
			}
			outcome := ShowOutcome{Graph: data.graph, Task: task, ProjectDir: data.dir, Journal: data.journal}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				RenderShow(io.Discard, outcome, false)
			}
		})
	}
}
