// Benchmarks for prune/tombstone impact on replay/list/compact performance.
// Purpose: keep performance regressions visible for replay/list/compact and prune planning.
// Exports: none.
// Role: benchmark and regression guardrails for internal performance.
// Invariants: benchmarks are deterministic and avoid external I/O.
package ergo

import (
	"fmt"
	"io"
	"testing"
	"time"
)

func buildTombstoneEventLog(taskCount int, tombstoneEvery int, withDeps bool) []Event {
	if taskCount <= 0 {
		return nil
	}
	base := time.Unix(1_700_000_000, 0).UTC()
	now := func(i int) time.Time { return base.Add(time.Duration(i) * time.Millisecond) }

	var events []Event
	for i := 0; i < taskCount; i++ {
		id := fmt.Sprintf("T%05d", i+1)
		ts := now(len(events))
		events = append(events, mustNewEvent("new_task", ts, NewTaskEvent{
			ID:        id,
			UUID:      "uuid-" + id,
			State:     stateTodo,
			Title:     "Task " + id,
			Body:      "Body " + id,
			CreatedAt: formatTime(ts),
		}))

		if withDeps && i > 0 {
			prevID := fmt.Sprintf("T%05d", i)
			ts = now(len(events))
			events = append(events, mustNewEvent("link", ts, LinkEvent{
				FromID: id,
				ToID:   prevID,
				Type:   dependsLinkType,
			}))
		}

		if tombstoneEvery > 0 && i%tombstoneEvery == 0 {
			ts = now(len(events))
			events = append(events, mustNewEvent("tombstone", ts, TombstoneEvent{
				ID:      id,
				AgentID: "bench",
				TS:      formatTime(ts),
			}))
		}
	}
	return events
}

func buildPruneGraph(taskCount int, epicEvery int) *Graph {
	graph := &Graph{Tasks: map[string]*Task{}}
	if taskCount <= 0 {
		return graph
	}
	epicIndex := 0
	for i := 0; i < taskCount; i++ {
		id := fmt.Sprintf("T%05d", i+1)
		state := stateTodo
		if i%5 == 0 {
			state = stateDone
		} else if i%7 == 0 {
			state = stateCanceled
		}
		epicID := ""
		if epicEvery > 0 {
			if i%epicEvery == 0 {
				epicIndex++
				epicID = fmt.Sprintf("E%03d", epicIndex)
				graph.Tasks[epicID] = &Task{ID: epicID, IsEpic: true}
			} else if epicIndex > 0 {
				epicID = fmt.Sprintf("E%03d", epicIndex)
			}
		}
		graph.Tasks[id] = &Task{ID: id, EpicID: epicID, State: state}
	}
	return graph
}

func BenchmarkTombstoneReplay1000(b *testing.B) {
	events := buildTombstoneEventLog(1000, 4, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := replayEvents(events); err != nil {
			b.Fatalf("replayEvents failed: %v", err)
		}
	}
}

func BenchmarkTombstoneList1000(b *testing.B) {
	events := buildTombstoneEventLog(1000, 4, true)
	graph, err := replayEvents(events)
	if err != nil {
		b.Fatalf("replayEvents failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		roots := buildListRoots(graph, true, false, "")
		renderTreeView(io.Discard, roots, graph, ".", false)
	}
}

func BenchmarkTombstoneCompact1000(b *testing.B) {
	events := buildTombstoneEventLog(1000, 4, true)
	graph, err := replayEvents(events)
	if err != nil {
		b.Fatalf("replayEvents failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := compactEvents(graph); err != nil {
			b.Fatalf("compactEvents failed: %v", err)
		}
	}
}

func BenchmarkPrunePlanSelection10000(b *testing.B) {
	graph := buildPruneGraph(10000, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildPrunePlan(graph)
	}
}

// Regression budget: replay 2000 tasks with tombstones should remain sub-500ms
// on typical CI hardware (very generous threshold to avoid flakiness).
func TestPerformance_ReplayWithTombstones(t *testing.T) {
	events := buildTombstoneEventLog(2000, 4, true)
	start := time.Now()
	if _, err := replayEvents(events); err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("replay with tombstones took %v, expected <500ms", elapsed)
	}
	t.Logf("replay with tombstones: %v", elapsed)
}
