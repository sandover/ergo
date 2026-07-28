// Purpose: Compute and apply prune plans for closed work.
// Exports: RunPrunePlan, RunPruneApply, PrunePlan, PruneItem.
// Role: Prune policy logic used by the CLI and tests.
// Invariants: Only done/canceled tasks are pruned; empty containers follow.
// Notes: Planning is deterministic and sorted by ID.
package ergo

import (
	"sort"
	"time"
)

type PrunePlan struct {
	PrunedIDs []string
	Items     []PruneItem
}

type PruneItem struct {
	ID          string
	Title       string
	State       string
	IsContainer bool
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
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return PrunePlan{}, err
	}
	var plan PrunePlan
	if !apply {
		graph, err := repository.View()
		if err != nil {
			return PrunePlan{}, err
		}
		return buildPrunePlan(graph), nil
	}
	_, err := repository.Update(func(graph *Graph) ([]Event, error) {
		plan = buildPrunePlan(graph)
		if len(plan.PrunedIDs) == 0 {
			return nil, nil
		}
		events, err := buildTombstoneEvents(plan.PrunedIDs, opts.AgentID)
		if err != nil {
			return nil, err
		}
		return events, nil
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
		if graph.IsEpic(task.ID) {
			continue
		}
		if task.State == stateDone || task.State == stateCanceled {
			eligibleTasks[task.ID] = struct{}{}
		}
	}

	remainingChildren := map[string]int{}
	for _, task := range graph.Tasks {
		if graph.IsEpic(task.ID) {
			continue
		}
		if _, willPrune := eligibleTasks[task.ID]; willPrune {
			continue
		}
		if task.EpicID != "" {
			remainingChildren[task.EpicID]++
		}
	}

	eligibleContainers := map[string]struct{}{}
	for _, task := range graph.Tasks {
		if !graph.IsEpic(task.ID) {
			continue
		}
		if remainingChildren[task.ID] == 0 {
			eligibleContainers[task.ID] = struct{}{}
		}
	}

	ids := make([]string, 0, len(eligibleTasks)+len(eligibleContainers))
	for id := range eligibleTasks {
		ids = append(ids, id)
	}
	for id := range eligibleContainers {
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
			ID:          id,
			Title:       task.Title,
			State:       task.State,
			IsContainer: graph.IsEpic(task.ID),
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
