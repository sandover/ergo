package ergo

import (
	"errors"
	"fmt"
	"time"
)

func writeLinkEvents(dir string, opts GlobalOptions, eventType string, edges []sequenceEdge) ([]sequenceEdge, error) {
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return nil, err
	}
	var changed []sequenceEdge
	_, err := repository.Update(func(graph *Graph) ([]Event, error) {
		working := cloneGraph(graph)
		events := make([]Event, 0, len(edges))
		now := time.Now().UTC()
		for _, edge := range edges {
			from := edge.FromID
			to := edge.ToID
			if _, ok := working.Tombstones[from]; ok {
				return nil, prunedErr(from)
			}
			if _, ok := graph.Tombstones[to]; ok {
				return nil, prunedErr(to)
			}
			fromItem, ok := working.Tasks[from]
			if !ok {
				return nil, fmt.Errorf("unknown id %s", from)
			}
			toItem, ok := working.Tasks[to]
			if !ok {
				return nil, fmt.Errorf("unknown id %s", to)
			}
			if err := validateDepSelf(from, to); err != nil {
				return nil, err
			}
			if eventType == "link" {
				if err := validateDepAncestry(fromItem, toItem); err != nil {
					return nil, err
				}
				if _, exists := working.Deps[from][to]; exists {
					continue
				}
				if hasCycle(working, from, to) {
					return nil, errors.New("dependency would create a cycle")
				}
			} else {
				if _, exists := working.Deps[from][to]; !exists {
					continue
				}
			}
			event, err := newEvent(eventType, now, LinkEvent{
				FromID: from,
				ToID:   to,
				Type:   dependsLinkType,
			})
			if err != nil {
				return nil, err
			}
			events = append(events, event)
			changed = append(changed, edge)
			if eventType == "link" {
				if working.Deps[from] == nil {
					working.Deps[from] = map[string]struct{}{}
				}
				working.Deps[from][to] = struct{}{}
			} else if working.Deps[from] != nil {
				delete(working.Deps[from], to)
			}
		}
		return events, nil
	})
	return changed, err
}
