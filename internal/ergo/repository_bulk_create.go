package ergo

import (
	"errors"
	"time"
)

// runBulkCreate creates an epic, its child tasks, and dependency edges.
// It backs the `new epic` command.
func runBulkCreate(dir string, opts GlobalOptions, epicTitle string, epicBody string, tasks []EpicTaskInput, draft bool) (bulkCreateOutput, error) {
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return bulkCreateOutput{}, err
	}

	var out bulkCreateOutput
	if _, err := repository.UpdateWithJournal(func(graph *Graph) ([]Event, []JournalEntry, error) {
		working := graph
		workingIDs := make(map[string]*Task, len(working.Tasks)+len(tasks)+1)
		for id, task := range working.Tasks {
			workingIDs[id] = task
		}

		now := time.Now().UTC()
		epicID, err := newShortID(workingIDs)
		if err != nil {
			return nil, nil, err
		}
		epicUUID, err := newUUID()
		if err != nil {
			return nil, nil, err
		}
		workingIDs[epicID] = &Task{ID: epicID}
		createdAt := formatTime(now)
		epicEvent, err := newEvent("new_task", now, NewTaskEvent{
			ID:        epicID,
			UUID:      epicUUID,
			EpicID:    "",
			State:     stateTodo,
			Title:     epicTitle,
			Body:      epicBody,
			CreatedAt: createdAt,
		})
		if err != nil {
			return nil, nil, err
		}

		out = bulkCreateOutput{
			ID:       epicID,
			Title:    epicTitle,
			Children: make([]bulkCreateChildOutput, 0, len(tasks)),
			Edges:    make([]sequenceEdge, 0),
		}

		newEvents := make([]Event, 0, 1+len(tasks))
		newEvents = append(newEvents, epicEvent)
		journal := []JournalEntry{newJournalEntry(epicID, "created", "", "", now)}

		titleToID := make(map[string]string, len(tasks))
		for _, taskInput := range tasks {
			taskTitle := taskInput.Title
			taskBody := taskInput.Body

			taskID, err := newShortID(workingIDs)
			if err != nil {
				return nil, nil, err
			}
			taskUUID, err := newUUID()
			if err != nil {
				return nil, nil, err
			}
			workingIDs[taskID] = &Task{ID: taskID, EpicID: epicID}

			taskNow := time.Now().UTC()
			state := stateTodo
			if draft {
				state = stateDraft
			}
			taskEvent, err := newEvent("new_task", taskNow, NewTaskEvent{
				ID:        taskID,
				UUID:      taskUUID,
				EpicID:    epicID,
				State:     state,
				Title:     taskTitle,
				Body:      taskBody,
				CreatedAt: formatTime(taskNow),
			})
			if err != nil {
				return nil, nil, err
			}
			newEvents = append(newEvents, taskEvent)
			journal = append(journal, newJournalEntry(taskID, "created", "", "", taskNow))
			out.Children = append(out.Children, bulkCreateChildOutput{
				ID:    taskID,
				Title: taskTitle,
			})

			titleToID[taskTitle] = taskID
			working.Tasks[taskID] = &Task{ID: taskID, EpicID: epicID}
			if working.Deps[taskID] == nil {
				working.Deps[taskID] = map[string]struct{}{}
			}
		}

		seenEdges := map[string]struct{}{}
		for _, taskInput := range tasks {
			fromTitle := taskInput.Title
			fromID := titleToID[fromTitle]
			for _, dep := range taskInput.After {
				toID := titleToID[dep]
				edgeKey := fromID + "->" + toID
				if _, exists := seenEdges[edgeKey]; exists {
					continue
				}
				seenEdges[edgeKey] = struct{}{}

				if err := validateDepSelf(fromID, toID); err != nil {
					return nil, nil, err
				}
				if hasCycle(working, fromID, toID) {
					return nil, nil, errors.New("dependency would create a cycle")
				}

				linkNow := time.Now().UTC()
				linkEvent, err := newEvent("link", linkNow, LinkEvent{
					FromID: fromID,
					ToID:   toID,
					Type:   dependsLinkType,
				})
				if err != nil {
					return nil, nil, err
				}
				newEvents = append(newEvents, linkEvent)
				if working.Deps[fromID] == nil {
					working.Deps[fromID] = map[string]struct{}{}
				}
				working.Deps[fromID][toID] = struct{}{}
				out.Edges = append(out.Edges, sequenceEdge{
					FromID: fromID,
					ToID:   toID,
				})
			}
		}

		return newEvents, journal, nil
	}); err != nil {
		return bulkCreateOutput{}, err
	}
	return out, nil
}
