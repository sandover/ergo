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
	graph, err := repository.ViewGraph()
	if err != nil {
		return ListOutcome{}, classifyRepositoryError(err)
	}
	graph.prepareDerivedQueries()
	if request.EpicID != "" {
		epic := graph.Tasks[request.EpicID]
		if epic == nil || !graph.IsEpic(epic.ID) {
			return ListOutcome{}, classified(ErrorNotFound, fmt.Errorf("no such epic: %s", request.EpicID))
		}
	}
	all, active, ready := collectListTasks(graph)
	outcome := ListOutcome{
		Options: request, Graph: graph,
		Roots:       buildListRoots(graph, request.ShowAll, request.ReadyOnly, request.EpicID),
		AllTasks:    all,
		ActiveTasks: active, ReadyTasks: ready,
	}
	if request.EpicID != "" {
		outcome.EpicChildren = collectEpicChildren(request.EpicID, graph)
		outcome.EpicReady = filterReadyTasks(outcome.EpicChildren, graph)
	}
	return outcome, nil
}
