// Prune computes one deterministic deletion plan for finished leaves and epics
// left empty by that plan. It must use the shared finished-state boundary so
// unsuccessful finished work receives the same explicit cleanup semantics.
package ergo

import (
	"fmt"
	"sort"
	"time"
)

type PrunePlan struct {
	PrunedIDs      []string
	Items          []PruneItem
	JournalEntries int
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
	err := withLock(repository.lockPath, repository.opts, func() error {
		graph, eventRead, err := repository.loadWithRead()
		if err != nil {
			return err
		}
		journalRead, err := repository.readJournal()
		if err != nil {
			return err
		}
		journal := journalRead.entries
		plan = buildPrunePlan(graph)
		selected := make(map[string]struct{}, len(plan.PrunedIDs))
		for _, id := range plan.PrunedIDs {
			selected[id] = struct{}{}
		}
		retained := make([]JournalEntry, 0, len(journal))
		for _, entry := range journal {
			if _, remove := selected[entry.TaskID]; remove {
				plan.JournalEntries++
				continue
			}
			retained = append(retained, entry)
		}
		if !apply || len(plan.PrunedIDs) == 0 {
			return nil
		}
		events, err := buildTombstoneEvents(plan.PrunedIDs, "")
		if err != nil {
			return err
		}
		if _, err := replayEventsOnto(graph, events); err != nil {
			return err
		}
		if err := repository.appendValidated(events, eventRead); err != nil {
			return err
		}
		if err := repository.replaceJournal(retained); err != nil {
			return fmt.Errorf("backlog changed, but journal update failed: %w", err)
		}
		return nil
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
		if isFinishedState(task.State) {
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
