// Graph queries derive every relationship from tasks and forward dependency
// edges. Completion and readiness must use the shared finished-state boundary;
// duplicating state lists here can release dependencies incorrectly.
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
		return isFinishedState(task.State)
	}
	for _, child := range graph.Children(id) {
		if !isFinishedState(child.State) {
			return false
		}
	}
	return true
}

func derivedEpicStateForTasks(children []*Task) string {
	if len(children) == 0 {
		return "empty"
	}
	allFinished := true
	allCanceled := true
	anyFailed := false
	for _, child := range children {
		allFinished = allFinished && isFinishedState(child.State)
		allCanceled = allCanceled && child.State == stateCanceled
		anyFailed = anyFailed || child.State == stateFailed
	}
	if !allFinished {
		return "active"
	}
	if anyFailed {
		return stateFailed
	}
	if allCanceled {
		return stateCanceled
	}
	return stateDone
}

func (graph *Graph) EpicState(id string) string {
	if graph == nil || !graph.IsEpic(id) {
		return ""
	}
	return derivedEpicStateForTasks(graph.Children(id))
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

func readyTasks(graph *Graph) []*Task {
	tasks := filterNonContainers(listTasks(graph, "", true), graph)
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

func filterNonContainers(tasks []*Task, graph *Graph) []*Task {
	filtered := tasks[:0]
	for _, task := range tasks {
		if !graph.IsEpic(task.ID) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func listTasks(graph *Graph, epicID string, readyOnly bool) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if epicID != "" && task.EpicID != epicID {
			continue
		}
		if readyOnly && !graph.IsReady(task.ID) {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

func sortedTasks(tasks map[string]*Task) []*Task {
	values := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		values = append(values, task)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}

func isDepComplete(depID string, graph *Graph) bool {
	return graph != nil && graph.IsComplete(depID)
}

func isReady(task *Task, graph *Graph) bool {
	return task != nil && graph != nil && graph.IsReady(task.ID)
}

func isBlocked(task *Task, graph *Graph) bool {
	return task != nil && graph != nil && graph.IsBlocked(task.ID)
}

func isEpicComplete(epicID string, graph *Graph) bool {
	return graph != nil && graph.IsEpic(epicID) && graph.IsComplete(epicID)
}

func hasCycle(graph *Graph, from, to string) bool {
	if from == to {
		return true
	}
	return isReachable(graph, to, from, make(map[string]bool))
}

func isReachable(graph *Graph, start, target string, visited map[string]bool) bool {
	if start == target {
		return true
	}
	if visited[start] {
		return false
	}
	visited[start] = true
	for dep := range graph.Deps[start] {
		if isReachable(graph, dep, target, visited) {
			return true
		}
	}
	return false
}
