// Purpose: Implement the `ergo plan` command for atomic epic/task graph creation.
// Exports: RunPlan.
// Role: Command layer that compiles a structured JSON plan into events.
// Invariants: Validation occurs before mutation; writes happen under one lock.
// Notes: Dependencies follow existing semantics (`from_id` depends on `to_id`).
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func RunPlan(args []string, opts GlobalOptions) error {
	if len(args) != 0 {
		return errors.New("usage: ergo plan")
	}

	input, verr := ParsePlanInput()
	if verr != nil {
		if opts.JSON {
			_ = verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}
	if verr := input.Validate(); verr != nil {
		if opts.JSON {
			_ = verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)

	var out planOutput
	if err := withLock(lockPath, syscall.LOCK_EX, func() error {
		events, err := readEvents(eventsPath)
		if err != nil {
			return err
		}
		graph, err := replayEvents(events)
		if err != nil {
			return err
		}

		workingIDs := make(map[string]*Task, len(graph.Tasks)+len(input.Tasks)+1)
		for id, task := range graph.Tasks {
			workingIDs[id] = task
		}

		epicTitle := *input.Title
		epicBody := ""
		if input.Body != nil {
			epicBody = *input.Body
		}

		now := time.Now().UTC()
		epicID, err := newShortID(workingIDs)
		if err != nil {
			return err
		}
		epicUUID, err := newUUID()
		if err != nil {
			return err
		}
		workingIDs[epicID] = &Task{ID: epicID, IsEpic: true}
		createdAt := formatTime(now)
		epicEvent, err := newEvent("new_epic", now, NewTaskEvent{
			ID:        epicID,
			UUID:      epicUUID,
			EpicID:    "",
			State:     stateTodo,
			Title:     epicTitle,
			Body:      epicBody,
			CreatedAt: createdAt,
		})
		if err != nil {
			return err
		}

		out = planOutput{
			Kind: "plan",
			Epic: planEntityOutput{
				ID:        epicID,
				UUID:      epicUUID,
				Title:     epicTitle,
				CreatedAt: createdAt,
			},
			Tasks: make([]planTaskOutput, 0, len(input.Tasks)),
			Edges: make([]sequenceEdgeOutput, 0),
		}

		newEvents := make([]Event, 0, 1+len(input.Tasks))
		newEvents = append(newEvents, epicEvent)

		titleToID := make(map[string]string, len(input.Tasks))
		for _, taskInput := range input.Tasks {
			taskTitle := *taskInput.Title
			taskBody := ""
			if taskInput.Body != nil {
				taskBody = *taskInput.Body
			}

			taskID, err := newShortID(workingIDs)
			if err != nil {
				return err
			}
			taskUUID, err := newUUID()
			if err != nil {
				return err
			}
			workingIDs[taskID] = &Task{ID: taskID, EpicID: epicID}

			taskNow := time.Now().UTC()
			taskEvent, err := newEvent("new_task", taskNow, NewTaskEvent{
				ID:        taskID,
				UUID:      taskUUID,
				EpicID:    epicID,
				State:     stateTodo,
				Title:     taskTitle,
				Body:      taskBody,
				CreatedAt: formatTime(taskNow),
			})
			if err != nil {
				return err
			}
			newEvents = append(newEvents, taskEvent)
			out.Tasks = append(out.Tasks, planTaskOutput{
				ID:    taskID,
				Title: taskTitle,
			})

			titleToID[taskTitle] = taskID
			graph.Tasks[taskID] = &Task{ID: taskID, EpicID: epicID}
			if graph.Deps[taskID] == nil {
				graph.Deps[taskID] = map[string]struct{}{}
			}
		}

		seenEdges := map[string]struct{}{}
		for _, taskInput := range input.Tasks {
			fromTitle := *taskInput.Title
			fromID := titleToID[fromTitle]
			for _, dep := range taskInput.After {
				toID := titleToID[dep]
				edgeKey := fromID + "->" + toID
				if _, exists := seenEdges[edgeKey]; exists {
					continue
				}
				seenEdges[edgeKey] = struct{}{}

				if err := validateDepSelf(fromID, toID); err != nil {
					return err
				}
				if hasCycle(graph, fromID, toID) {
					return errors.New("dependency would create a cycle")
				}

				linkNow := time.Now().UTC()
				linkEvent, err := newEvent("link", linkNow, LinkEvent{
					FromID: fromID,
					ToID:   toID,
					Type:   dependsLinkType,
				})
				if err != nil {
					return err
				}
				newEvents = append(newEvents, linkEvent)
				if graph.Deps[fromID] == nil {
					graph.Deps[fromID] = map[string]struct{}{}
				}
				graph.Deps[fromID][toID] = struct{}{}
				out.Edges = append(out.Edges, sequenceEdgeOutput{
					FromID: fromID,
					ToID:   toID,
					Type:   dependsLinkType,
				})
			}
		}

		return appendEventsAtomically(eventsPath, events, newEvents)
	}); err != nil {
		return err
	}

	if opts.JSON {
		return writeJSON(os.Stdout, out)
	}
	if opts.Quiet {
		return nil
	}
	fmt.Printf("Created epic %s: %s (%d tasks, %d dependencies)\n", out.Epic.ID, out.Epic.Title, len(out.Tasks), len(out.Edges))
	return nil
}
