package ergo

import (
	"errors"
	"fmt"
)

type ListRequest = ListOptions
type ListOutcome struct {
	Options      ListOptions
	Graph        *Graph
	Roots        []*treeNode
	ProjectDir   string
	AllTasks     []*Task
	ActiveTasks  []*Task
	ReadyTasks   []*Task
	EpicChildren []*Task
	EpicReady    []*Task
}

func (a *Application) List(request ListRequest) (ListOutcome, error) {
	if request.ReadyOnly && request.ShowAll {
		return ListOutcome{}, classified(ErrorUsage, errors.New("conflicting flags: --ready and --all"))
	}
	var repository Repository
	if err := repository.Open(a.repository); err != nil {
		return ListOutcome{}, classifyRepositoryError(err)
	}
	graph, err := repository.View()
	if err != nil {
		return ListOutcome{}, classifyRepositoryError(err)
	}
	if request.EpicID != "" {
		epic := graph.Tasks[request.EpicID]
		if epic == nil || !graph.IsEpic(epic.ID) {
			return ListOutcome{}, classified(ErrorNotFound, fmt.Errorf("no such epic: %s", request.EpicID))
		}
	}
	all := collectNonContainerTasks(graph)
	outcome := ListOutcome{
		Options: request, Graph: graph,
		Roots:      buildListRoots(graph, request.ShowAll, request.ReadyOnly, request.EpicID),
		ProjectDir: repository.ProjectDir(), AllTasks: all,
		ActiveTasks: filterActiveTasks(all), ReadyTasks: filterReadyTasks(all, graph),
	}
	if request.EpicID != "" {
		outcome.EpicChildren = collectEpicChildren(request.EpicID, graph)
		outcome.EpicReady = filterReadyTasks(outcome.EpicChildren, graph)
	}
	return outcome, nil
}
