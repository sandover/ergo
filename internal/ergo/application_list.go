package ergo

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
	session, err := a.OpenSession()
	if err != nil {
		return ListOutcome{}, err
	}
	defer session.Close()
	return session.List(request)
}
