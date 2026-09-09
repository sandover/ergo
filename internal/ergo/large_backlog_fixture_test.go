// Purpose: Prove the large-backlog fixture is deterministic and replay-safe.
// Coverage: full history, compacted snapshots, and both journal projections.
// Invariants: all representations expose the same current graph and expected
// terminal/active state mix; compaction must not change that graph.
package ergo

import (
	"path/filepath"
	"testing"
)

func TestLargeBacklogFixtureReplaysAcrossRepresentations(t *testing.T) {
	modes := []struct {
		name, backlog, journal string
	}{
		{"full-full", LargeBacklogModeFull, LargeBacklogModeFull},
		{"compacted-full", LargeBacklogModeCompacted, LargeBacklogModeFull},
		{"compacted-compacted", LargeBacklogModeCompacted, LargeBacklogModeCompacted},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			dir := t.TempDir()
			stats, err := WriteLargeBacklogFixture(dir, LargeBacklogFixtureOptions{
				BacklogMode: mode.backlog, JournalMode: mode.journal,
			})
			if err != nil {
				t.Fatal(err)
			}
			if stats.TaskCount != 1500 || stats.EpicCount != 24 || stats.LeafCount != 1476 {
				t.Fatalf("fixture shape = %+v", stats)
			}
			if stats.EventCount != 30380 || stats.FullJournalEntries != 5984 {
				t.Fatalf("fixture history = %+v", stats)
			}
			wantBacklogRecords := stats.EventCount
			if mode.backlog == LargeBacklogModeCompacted {
				wantBacklogRecords = stats.TaskCount + stats.DependencyCount + 1
			}
			if stats.BacklogRecords != wantBacklogRecords {
				t.Fatalf("backlog records = %d, want %d", stats.BacklogRecords, wantBacklogRecords)
			}

			var repository Repository
			if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
				t.Fatal(err)
			}
			graph, journal, err := repository.ViewWithJournal()
			if err != nil {
				t.Fatal(err)
			}
			assertLargeBacklogGraph(t, graph, stats)
			wantJournal := stats.FullJournalEntries
			if mode.journal == LargeBacklogModeCompacted {
				wantJournal = len(compactJournal(journal, graph))
			}
			if len(journal) != wantJournal {
				t.Fatalf("journal entries = %d, want %d", len(journal), wantJournal)
			}
		})
	}
}

func TestLargeBacklogFixtureCompactionPreservesGraph(t *testing.T) {
	dir := t.TempDir()
	stats, err := WriteLargeBacklogFixture(dir, LargeBacklogFixtureOptions{
		BacklogMode: LargeBacklogModeFull, JournalMode: LargeBacklogModeFull,
	})
	if err != nil {
		t.Fatal(err)
	}

	var repository Repository
	if err := repository.Open(RepositoryOptions{StartDir: dir}); err != nil {
		t.Fatal(err)
	}
	before, beforeJournal, err := repository.ViewWithJournal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Compact(); err != nil {
		t.Fatal(err)
	}
	after, afterJournal, err := repository.ViewWithJournal()
	if err != nil {
		t.Fatal(err)
	}
	assertGraphStateEqual(t, before, after)
	assertLargeBacklogGraph(t, after, stats)
	if len(beforeJournal) != stats.FullJournalEntries {
		t.Fatalf("before journal entries = %d, want %d", len(beforeJournal), stats.FullJournalEntries)
	}
	if len(afterJournal) != len(compactJournal(beforeJournal, before)) {
		t.Fatalf("after journal entries = %d, want compacted count %d", len(afterJournal), len(compactJournal(beforeJournal, before)))
	}

	read, err := inspectEventLog(filepath.Join(dir, dataDirName, backlogFileName))
	if err != nil {
		t.Fatal(err)
	}
	if read.snapshot == nil || len(read.events) != 0 {
		t.Fatalf("compacted backlog read = snapshot %v, trailing events %d", read.snapshot != nil, len(read.events))
	}
}

func assertLargeBacklogGraph(t *testing.T, graph *Graph, stats LargeBacklogFixtureStats) {
	t.Helper()
	if graph == nil || len(graph.Tasks) != stats.TaskCount {
		t.Fatalf("task count = %d, want %d", len(graph.Tasks), stats.TaskCount)
	}
	counts := map[string]int{}
	epics := 0
	for id, task := range graph.Tasks {
		if graph.IsEpic(id) {
			epics++
			continue
		}
		counts[task.State]++
	}
	if epics != stats.EpicCount {
		t.Fatalf("epic count = %d, want %d", epics, stats.EpicCount)
	}
	if counts[stateDone] != stats.FinalDone || counts[stateCanceled] != stats.FinalCanceled ||
		counts[stateFailed] != stats.FinalFailed || counts[stateBlocked] != stats.FinalBlocked ||
		counts[stateDoing] != stats.FinalDoing || counts[stateTodo] != stats.FinalTodo {
		t.Fatalf("final state counts = %v, want done=%d canceled=%d failed=%d blocked=%d doing=%d todo=%d",
			counts, stats.FinalDone, stats.FinalCanceled, stats.FinalFailed, stats.FinalBlocked, stats.FinalDoing, stats.FinalTodo)
	}
	if countDependencies(graph) != stats.DependencyCount {
		t.Fatalf("dependency count = %d, want %d", countDependencies(graph), stats.DependencyCount)
	}
}
