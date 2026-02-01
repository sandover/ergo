// Purpose: Compute and apply prune plans for closed work.
// Exports: RunPrunePlan, RunPruneApply, PrunePlan, PruneItem.
// Role: Prune policy logic used by the CLI and tests.
// Invariants: Only done/canceled tasks are pruned; empty epics pruned after tasks.
// Notes: Planning is deterministic and sorted by ID.
package ergo

import (
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

type PrunePlan struct {
	PrunedIDs []string
	Items     []PruneItem
}

type PruneItem struct {
	ID     string
	Title  string
	State  string
	IsEpic bool
}

// RunPrunePlan computes the prune plan under the lock without writing events.
func RunPrunePlan(dir string) (PrunePlan, error) {
	return runPrune(dir, GlobalOptions{}, false)
}

// RunPruneApply computes the prune plan and appends delete-marker events for all targets.
func RunPruneApply(dir string, opts GlobalOptions) (PrunePlan, error) {
	return runPrune(dir, opts, true)
}

func runPrune(dir string, opts GlobalOptions, apply bool) (PrunePlan, error) {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	var plan PrunePlan
	err := withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		plan = buildPrunePlan(graph)
		if !apply || len(plan.PrunedIDs) == 0 {
			return nil
		}
		events, err := buildTombstoneEvents(plan.PrunedIDs, opts.AgentID)
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, events)
	})
	return plan, err
}

func buildPrunePlan(graph *Graph) PrunePlan {
	pruned := selectPruneTargets(graph)
	items := buildPruneItems(graph, pruned)
	return PrunePlan{PrunedIDs: pruned, Items: items}
}

func selectPruneTargets(graph *Graph) []string {
	if graph == nil {
		return nil
	}
	eligibleTasks := map[string]struct{}{}
	for _, task := range graph.Tasks {
		if task.IsEpic {
			continue
		}
		if task.State == stateDone || task.State == stateCanceled {
			eligibleTasks[task.ID] = struct{}{}
		}
	}

	remainingChildren := map[string]int{}
	for _, task := range graph.Tasks {
		if task.IsEpic {
			continue
		}
		if _, willPrune := eligibleTasks[task.ID]; willPrune {
			continue
		}
		if task.EpicID != "" {
			remainingChildren[task.EpicID]++
		}
	}

	eligibleEpics := map[string]struct{}{}
	for _, task := range graph.Tasks {
		if !task.IsEpic {
			continue
		}
		if remainingChildren[task.ID] == 0 {
			eligibleEpics[task.ID] = struct{}{}
		}
	}

	ids := make([]string, 0, len(eligibleTasks)+len(eligibleEpics))
	for id := range eligibleTasks {
		ids = append(ids, id)
	}
	for id := range eligibleEpics {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func buildPruneItems(graph *Graph, ids []string) []PruneItem {
	if graph == nil || len(ids) == 0 {
		return nil
	}
	items := make([]PruneItem, 0, len(ids))
	for _, id := range ids {
		task, ok := graph.Tasks[id]
		if !ok {
			continue
		}
		items = append(items, PruneItem{
			ID:     id,
			Title:  task.Title,
			State:  task.State,
			IsEpic: task.IsEpic,
		})
	}
	return items
}

func buildTombstoneEvents(ids []string, agentID string) ([]Event, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	events := make([]Event, 0, len(ids))
	for _, id := range ids {
		event, err := newEvent("tombstone", now, TombstoneEvent{
			ID:      id,
			AgentID: agentID,
			TS:      formatTime(now),
		})
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
