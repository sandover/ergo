package ergo

import (
	"errors"
	"fmt"
	"io"
	"time"
)

func RunClaim(id, agentID string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Claim(ClaimRequest{ID: id, AgentID: agentID})
	if err != nil {
		return err
	}
	RenderClaim(render.writer(), outcome)
	return nil
}

func RunClaimOldestReady(agentID string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Claim(ClaimRequest{AgentID: agentID})
	if err != nil {
		return err
	}
	if outcome.NoReady {
		fmt.Fprintln(render.writer(), "No ready ergo tasks.")
		return nil
	}
	RenderClaim(render.writer(), outcome)
	return nil
}

func RenderClaim(w io.Writer, outcome ClaimOutcome) {
	task, graph, repoDir := outcome.Task, outcome.Graph, outcome.ProjectDir
	if task == nil {
		return
	}
	id := task.ID
	next := map[string]string{
		"done":    "ergo done " + id,
		"block":   "ergo block " + id,
		"cancel":  "ergo cancel " + id,
		"release": "ergo release " + id,
	}
	printTaskDocument(w, task, graph, repoDir)
	fmt.Fprintln(w, "## Next")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- `"+next["done"]+"`")
	fmt.Fprintln(w, "- `"+next["block"]+"`")
	fmt.Fprintln(w, "- `"+next["cancel"]+"`")
	fmt.Fprintln(w, "- `"+next["release"]+"`")
}

type sequenceEdge struct {
	FromID string
	ToID   string
}

func buildSequenceEdges(order []string) []sequenceEdge {
	if len(order) < 2 {
		return nil
	}
	edges := make([]sequenceEdge, 0, len(order)-1)
	for i := 0; i < len(order)-1; i++ {
		edges = append(edges, sequenceEdge{
			FromID: order[i+1],
			ToID:   order[i],
		})
	}
	return edges
}

func RunSequence(args []string, opts GlobalOptions, render RenderOptions) error {
	if len(args) > 0 && args[0] == "rm" {
		return errors.New("sequence rm is not accepted; use ergo unsequence <A> <B> [<C>...]")
	}
	return runSequenceChange("sequence", "link", args, opts, render.writer())
}

func RunUnsequence(args []string, opts GlobalOptions, render RenderOptions) error {
	return runSequenceChange("unsequence", "unlink", args, opts, render.writer())
}

func runSequenceChange(command, eventType string, args []string, opts GlobalOptions, w io.Writer) error {
	outcome, err := NewApplication(opts).Sequence(SequenceRequest{
		Command: command, EventType: eventType, IDs: args,
	})
	if err != nil {
		return err
	}
	RenderSequence(w, outcome)
	return nil
}

func RenderSequence(w io.Writer, outcome SequenceOutcome) {
	if len(outcome.Edges) == 0 {
		fmt.Fprintln(w, "No dependency changes.")
		return
	}
	for _, edge := range outcome.Edges {
		if outcome.EventType == "unlink" {
			fmt.Fprintf(w, "%s no longer depends on %s\n", edge.FromID, edge.ToID)
			continue
		}
		fmt.Fprintf(w, "%s depends on %s\n", edge.FromID, edge.ToID)
	}
}

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
