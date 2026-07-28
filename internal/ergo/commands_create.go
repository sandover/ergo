// Purpose: Implement init and create commands for tasks.
// Exports: RunInit, RunNewTask, RunNewEpic.
// Role: Command layer for creation workflows and repo initialization.
// Invariants: Writes are append-only under lock; create is safe under concurrent writers.
// Notes: Titles are positional, bodies are optional stdin, and new tasks are todo.
package ergo

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func RunInit(args []string, opts GlobalOptions) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	if len(args) > 1 {
		return errors.New("usage: ergo init [dir]")
	}
	outcome, err := InitializeRepository(dir)
	if err != nil {
		return err
	}
	switch outcome.Status {
	case "initialized":
		fmt.Printf("Initialized Ergo at %s\n", outcome.Path)
	case "repaired":
		fmt.Printf("Repaired Ergo at %s\n", outcome.Path)
	default:
		fmt.Printf("Ergo already initialized at %s\n", outcome.Path)
	}
	return nil
}

func RunNewTask(title, epicID string, opts GlobalOptions) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New(NewTaskUsage)
	}
	body, _, err := readOptionalBodyFromStdin()
	if err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	created, err := createTaskWithUpdates(dir, opts, epicID, title, body, nil, "")
	if err != nil {
		return err
	}

	fmt.Println(created.ID)
	return nil
}

func RunNewEpic(title, filePath string, opts GlobalOptions) error {
	if strings.TrimSpace(filePath) == "" {
		return errors.New(NewEpicUsage)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New(NewEpicUsage)
	}
	tasks, err := ParseEpicFile(filePath)
	if err != nil {
		return err
	}
	body, _, err := readOptionalBodyFromStdin()
	if err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	return runBulkCreate(dir, opts, title, body, tasks)
}

// runBulkCreate creates an epic, its child tasks, and dependency edges.
// It backs the `new epic` command.
func runBulkCreate(dir string, opts GlobalOptions, epicTitle string, epicBody string, tasks []EpicTaskInput) error {
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return err
	}

	var out bulkCreateOutput
	if _, err := repository.Update(func(graph *Graph) ([]Event, error) {
		working := cloneGraph(graph)
		workingIDs := make(map[string]*Task, len(working.Tasks)+len(tasks)+1)
		for id, task := range working.Tasks {
			workingIDs[id] = task
		}

		now := time.Now().UTC()
		epicID, err := newShortID(workingIDs)
		if err != nil {
			return nil, err
		}
		epicUUID, err := newUUID()
		if err != nil {
			return nil, err
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
			return nil, err
		}

		out = bulkCreateOutput{
			ID:       epicID,
			Title:    epicTitle,
			Children: make([]bulkCreateChildOutput, 0, len(tasks)),
			Edges:    make([]sequenceEdge, 0),
		}

		newEvents := make([]Event, 0, 1+len(tasks))
		newEvents = append(newEvents, epicEvent)

		titleToID := make(map[string]string, len(tasks))
		for _, taskInput := range tasks {
			taskTitle := taskInput.Title
			taskBody := taskInput.Body

			taskID, err := newShortID(workingIDs)
			if err != nil {
				return nil, err
			}
			taskUUID, err := newUUID()
			if err != nil {
				return nil, err
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
				return nil, err
			}
			newEvents = append(newEvents, taskEvent)
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
					return nil, err
				}
				if hasCycle(working, fromID, toID) {
					return nil, errors.New("dependency would create a cycle")
				}

				linkNow := time.Now().UTC()
				linkEvent, err := newEvent("link", linkNow, LinkEvent{
					FromID: fromID,
					ToID:   toID,
					Type:   dependsLinkType,
				})
				if err != nil {
					return nil, err
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

		return newEvents, nil
	}); err != nil {
		return err
	}

	fmt.Printf("Epic %s - %s\n", out.ID, out.Title)
	for _, child := range out.Children {
		fmt.Printf("  %s - %s\n", child.ID, child.Title)
	}
	fmt.Printf("%d tasks, %d dependencies\n", len(out.Children), len(out.Edges))
	return nil
}
