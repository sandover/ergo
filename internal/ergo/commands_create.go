// Purpose: Implement init and create commands for tasks.
// Exports: RunInit, RunNewTask.
// Role: Command layer for creation workflows and repo initialization.
// Invariants: Writes are append-only under lock; create is safe under concurrent writers.
// Notes: New tasks start in todo state; containers cannot nest.
//
// Task creation uses one forward input style:
// - optional inline JSON argument for metadata (`new task '{...}'`)
// - optional piped stdin for the body (`printf ... | ergo new task '{...}'`)
// - `plan --file` for markdown-driven bulk creation
//
//	printf '%s\n' '## Goal' '- Do X' | ergo new task '{"title":"Do X"}'
//	ergo plan --file tasks.md '{"title":"Auth system"}'
//
// See cli_input.go for the user-facing parser contract.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	plansPath := filepath.Join(target, plansFileName)
	lockPath := filepath.Join(target, "lock")
	if err := ensureFileExists(plansPath, 0644); err != nil {
		return err
	}
	if err := ensureFileExists(lockPath, 0644); err != nil {
		return err
	}
	if opts.JSON {
		if err := writeJSON(os.Stdout, initOutput{ErgoDir: target}); err != nil {
			return err
		}
	} else {
		fmt.Println(target)
	}
	fmt.Fprintln(os.Stderr, "Initialized ergo at", target)
	return nil
}

func RunNewTask(args []string, opts GlobalOptions) error {
	const usage = "usage: ergo new task [json]"

	input, verr, err := parseInlineTaskArgs(args, usage)
	if err != nil {
		return err
	}
	if verr != nil {
		if opts.JSON {
			_ = verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}
	if verr := input.ValidateForNew(); verr != nil {
		if opts.JSON {
			_ = verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	body, _, err := readOptionalBodyFromStdin()
	if err != nil {
		return err
	}

	title := strings.TrimSpace(*input.Title)
	epicID := ""
	if input.Epic != nil {
		epicID = *input.Epic
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	updates := input.ToUpdates()
	delete(updates, "title")
	delete(updates, "epic")
	created, err := createTaskWithUpdates(dir, opts, epicID, title, body, updates, opts.AgentID)
	if err != nil {
		return err
	}

	if opts.JSON {
		return writeJSON(os.Stdout, created)
	}
	fmt.Println(created.ID)
	return nil
}

func RunPlan(filePath string, args []string, opts GlobalOptions) error {
	const usage = "usage: ergo plan --file <path> [json]"
	if strings.TrimSpace(filePath) == "" {
		return errors.New(usage)
	}

	input, verr, err := parsePlanCommandArgs(args, usage)
	if err != nil {
		return err
	}
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

	tasks, err := ParsePlanFile(filePath)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(*input.Title)

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	return runBulkCreate(dir, opts, title, "", tasks)
}

// runBulkCreate creates a container task with child tasks and dependency edges.
// It backs the current `plan --file` command.
func runBulkCreate(dir string, opts GlobalOptions, containerTitle string, containerBody string, tasks []PlanTaskInput) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)

	var out bulkCreateOutput
	if err := withLock(lockPath, syscall.LOCK_EX, opts, func() error {
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
		containerID, err := newShortID(workingIDs)
		if err != nil {
			return err
		}
		containerUUID, err := newUUID()
		if err != nil {
			return err
		}
		workingIDs[containerID] = &Task{ID: containerID}
		createdAt := formatTime(now)
		containerEvent, err := newEvent("new_task", now, NewTaskEvent{
			ID:        containerID,
			UUID:      containerUUID,
			EpicID:    "",
			State:     stateTodo,
			Title:     containerTitle,
			Body:      containerBody,
			CreatedAt: createdAt,
		})
		if err != nil {
			return err
		}

		out = bulkCreateOutput{
			Kind:      "create",
			Container: true,
			ID:        containerID,
			UUID:      containerUUID,
			Title:     containerTitle,
			State:     stateTodo,
			CreatedAt: createdAt,
			Children:  make([]bulkCreateChildOutput, 0, len(tasks)),
			Edges:     make([]sequenceEdgeOutput, 0),
		}

		newEvents := make([]Event, 0, 1+len(tasks))
		newEvents = append(newEvents, containerEvent)

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
			workingIDs[taskID] = &Task{ID: taskID, EpicID: containerID}

			taskNow := time.Now().UTC()
			taskEvent, err := newEvent("new_task", taskNow, NewTaskEvent{
				ID:        taskID,
				UUID:      taskUUID,
				EpicID:    containerID,
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
			graph.Tasks[taskID] = &Task{ID: taskID, EpicID: containerID}
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
	fmt.Printf("Created container %s: %s (%d tasks, %d dependencies)\n", out.ID, out.Title, len(out.Children), len(out.Edges))
	return nil
}
