package ergo

import (
	"errors"
	"fmt"
)

type SequenceRequest struct {
	Command, EventType string
	IDs                []string
}
type SequenceOutcome struct {
	EventType string
	Edges     []sequenceEdge
}

func (a *Application) Sequence(request SequenceRequest) (SequenceOutcome, error) {
	usage := fmt.Sprintf("usage: ergo %s <A> <B> [<C>...]", request.Command)
	if len(request.IDs) < 2 {
		return SequenceOutcome{}, classified(ErrorUsage, errors.New(usage))
	}
	dir, err := ergoDir(a.repository)
	if err != nil {
		return SequenceOutcome{}, classifyRepositoryError(err)
	}
	changed, err := writeLinkEvents(dir, a.repository, request.EventType, buildSequenceEdges(request.IDs))
	if err != nil {
		return SequenceOutcome{}, classifyRepositoryError(err)
	}
	return SequenceOutcome{EventType: request.EventType, Edges: changed}, nil
}
