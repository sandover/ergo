// Purpose: Implement init and create commands for tasks.
// Exports: RunInit, RunNewTask, RunNewEpic.
// Role: Command layer for creation workflows and repo initialization.
// Invariants: Writes are append-only under lock; create is safe under concurrent writers.
// Notes: Titles are positional, bodies are optional stdin, and new tasks are todo.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	target := filepath.Join(dir, dataDirName)
	_, dirErr := os.Stat(target)
	eventsPath, err := selectEventsPath(target)
	if err != nil {
		return err
	}
	_, eventsErr := os.Stat(eventsPath)
	_, lockErr := os.Stat(filepath.Join(target, "lock"))
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	lockPath := filepath.Join(target, "lock")
	if err := ensureFileExists(eventsPath, 0644); err != nil {
		return err
	}
	if err := ensureFileExists(lockPath, 0644); err != nil {
		return err
	}
	resolved, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	switch {
	case os.IsNotExist(dirErr):
		fmt.Printf("Initialized Ergo at %s\n", resolved)
	case os.IsNotExist(eventsErr) || os.IsNotExist(lockErr):
		fmt.Printf("Repaired Ergo at %s\n", resolved)
	default:
		fmt.Printf("Ergo already initialized at %s\n", resolved)
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
	lockPath := filepath.Join(dir, "lock")
	eventsPath, err := selectEventsPath(dir)
	if err != nil {
		return err
	}

	var out bulkCreateOutput
	if err := withLock(lockPath, opts, func() error {
		events, err := readEvents(eventsPath)
		if err != nil {
			return err
		}
		graph, err := replayEvents(events)
		if err != nil {
			return err
		}

		workingIDs := make(map[string]*Task, len(graph.Tasks)+len(tasks)+1)
		for id, task := range graph.Tasks {
			workingIDs[id] = task
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
			return err
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
			out.Children = append(out.Children, bulkCreateChildOutput{
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
				out.Edges = append(out.Edges, sequenceEdge{
					FromID: fromID,
					ToID:   toID,
				})
			}
		}

		return appendEvents(eventsPath, newEvents)
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
