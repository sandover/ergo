// Purpose: Own graph-derived indexes and domain queries.
// Exports: none.
// Role: Canonical access to relationships, epic identity, completion, and readiness.
// Invariants: Tasks and forward dependency edges are canonical; all other indexes derive from them.
package ergo

import "sort"

func (graph *Graph) rebuildIndexes() {
	if graph == nil {
		return
	}
	graph.reverseDeps = make(map[string]map[string]struct{})
	graph.childrenByEpic = make(map[string][]*Task)
	for from, dependencies := range graph.Deps {
		for to := range dependencies {
			if graph.reverseDeps[to] == nil {
				graph.reverseDeps[to] = make(map[string]struct{})
			}
			graph.reverseDeps[to][from] = struct{}{}
		}
	}
	for _, task := range graph.Tasks {
		if task.EpicID != "" {
			graph.childrenByEpic[task.EpicID] = append(graph.childrenByEpic[task.EpicID], task)
		}
	}
	for epicID := range graph.childrenByEpic {
		sort.Slice(graph.childrenByEpic[epicID], func(i, j int) bool {
			return graph.childrenByEpic[epicID][i].ID < graph.childrenByEpic[epicID][j].ID
		})
	}
}

func (graph *Graph) ensureIndexes() {
	if graph != nil && (graph.reverseDeps == nil || graph.childrenByEpic == nil) {
		graph.rebuildIndexes()
	}
}

func (graph *Graph) Children(epicID string) []*Task {
	if graph == nil {
		return nil
	}
	graph.ensureIndexes()
	return append([]*Task(nil), graph.childrenByEpic[epicID]...)
}

func (graph *Graph) Dependencies(id string) []string {
	if graph == nil {
		return nil
	}
	return sortedKeys(graph.Deps[id])
}

func (graph *Graph) Dependents(id string) []string {
	if graph == nil {
		return nil
	}
	graph.ensureIndexes()
	return sortedKeys(graph.reverseDeps[id])
}

func (graph *Graph) IsEpic(id string) bool {
	if graph == nil || graph.Tasks[id] == nil {
		return false
	}
	graph.ensureIndexes()
	if len(graph.childrenByEpic[id]) > 0 {
		return true
	}
	_, legacy := graph.legacyEmptyEpics[id]
	return legacy
}

func (graph *Graph) IsComplete(id string) bool {
	task := graph.Tasks[id]
	if task == nil {
		return false
	}
	if !graph.IsEpic(id) {
		return task.State == stateDone || task.State == stateCanceled
	}
	for _, child := range graph.Children(id) {
		if child.State != stateDone && child.State != stateCanceled {
			return false
		}
	}
	return true
}

func (graph *Graph) Blockers(id string) []string {
	task := graph.Tasks[id]
	if task == nil {
		return nil
	}
	set := make(map[string]struct{})
	for dependencyID := range graph.Deps[id] {
		if !graph.IsComplete(dependencyID) {
			set[dependencyID] = struct{}{}
		}
	}
	if task.EpicID != "" {
		for dependencyID := range graph.Deps[task.EpicID] {
			if !graph.IsComplete(dependencyID) {
				set[dependencyID] = struct{}{}
			}
		}
	}
	blockers := make([]string, 0, len(set))
	for id := range set {
		blockers = append(blockers, id)
	}
	sort.Strings(blockers)
	return blockers
}

func (graph *Graph) IsReady(id string) bool {
	task := graph.Tasks[id]
	return task != nil &&
		!graph.IsEpic(id) &&
		task.State == stateTodo &&
		task.ClaimedBy == "" &&
		len(graph.Blockers(id)) == 0
}

func (graph *Graph) IsBlocked(id string) bool {
	task := graph.Tasks[id]
	if task == nil || graph.IsEpic(id) {
		return false
	}
	if task.State == stateBlocked {
		return true
	}
	return task.State == stateTodo && task.ClaimedBy == "" && len(graph.Blockers(id)) > 0
}
