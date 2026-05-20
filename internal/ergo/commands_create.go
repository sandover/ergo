// Purpose: Implement init and create commands for tasks.
// Exports: RunInit, RunNewTask.
// Role: Command layer for creation workflows and repo initialization.
// Invariants: Writes are append-only under lock; create is safe under concurrent writers.
// Notes: New tasks start in todo state; containers cannot nest.
//
// Task creation supports multiple input styles:
// - JSON object on stdin (default)
// - Flags-only input (e.g. --title/--body) when stdin is a TTY
// - `--body-stdin` to treat stdin as literal body text (metadata via flags)
// - `tasks:[...]` array creates a container with child tasks and deps
//
//	printf '%s' '{"title":"Do X"}' | ergo new task
//	printf '%s' '{"title":"Auth system","tasks":[...]}' | ergo new task
//
// See json_input.go for the unified TaskInput schema.
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
	if !opts.Quiet {
		fmt.Fprintln(os.Stderr, "Initialized ergo at", target)
	}
	return nil
}

func RunNewTask(opts GlobalOptions) error {
	if opts.BodyStdin {
		if err := validateBodyStdinExclusions(opts.BodyFlag); err != nil {
			return err
		}
		title := strings.TrimSpace(opts.TitleFlag)
		if title == "" {
			return errors.New("new task --body-stdin requires --title")
		}
		body, err := readBodyFromStdinOrEmpty()
		if err != nil {
			return err
		}

		dir, err := ergoDir(opts)
		if err != nil {
			return err
		}
		created, err := createTask(dir, opts, opts.EpicFlag, false, title, body)
		if err != nil {
			return err
		}

		updates := buildFlagUpdates(opts)
		delete(updates, "title")
		delete(updates, "epic")
		if len(updates) > 0 {
			agentID := opts.AgentID
			if err := applySetUpdates(dir, opts, created.ID, updates, agentID, true); err != nil {
				return err
			}
		}

		if opts.JSON {
			return writeJSON(os.Stdout, created)
		}
		fmt.Println(created.ID)
		return nil
	}

	hasFlagInput := strings.TrimSpace(opts.TitleFlag) != "" ||
		opts.BodyFlag != "" ||
		opts.EpicFlag != "" ||
		opts.StateFlag != "" ||
		opts.ClaimFlag != ""
	if !stdinIsPiped() && hasFlagInput {
		title := strings.TrimSpace(opts.TitleFlag)
		if title == "" {
			return errors.New("new task requires --title (or pipe JSON to stdin)")
		}

		dir, err := ergoDir(opts)
		if err != nil {
			return err
		}
		created, err := createTask(dir, opts, opts.EpicFlag, false, title, opts.BodyFlag)
		if err != nil {
			return err
		}

		updates := buildFlagUpdates(opts)
		delete(updates, "title")
		delete(updates, "epic")
		if len(updates) > 0 {
			agentID := opts.AgentID
			if err := applySetUpdates(dir, opts, created.ID, updates, agentID, true); err != nil {
				return err
			}
		}

		if opts.JSON {
			return writeJSON(os.Stdout, created)
		}
		fmt.Println(created.ID)
		return nil
	}

	// Parse JSON from stdin
	input, verr := ParseTaskInput()
	if verr != nil {
		if opts.JSON {
			_ = verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	// Validate for task creation
	if verr := input.ValidateForNewTask(); verr != nil {
		if opts.JSON {
			_ = verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	// Bulk creation: if tasks array is present, create a container with children
	if len(input.Tasks) > 0 {
		return runBulkCreate(dir, opts, input)
	}

	// Create the task
	created, err := createTask(dir, opts, input.GetEpic(), false, input.GetTitle(), input.GetBody())
	if err != nil {
		return err
	}

	// If state/claim were provided, apply them via set logic
	if input.State != nil || input.Claim != nil || input.ResultPath != nil {
		updates := input.ToKeyValueMap()
		// Remove fields already handled by createTask
		delete(updates, "title")
		delete(updates, "body")
		delete(updates, "epic")
		if len(updates) > 0 {
			agentID := opts.AgentID
			if err := applySetUpdates(dir, opts, created.ID, updates, agentID, true); err != nil {
				return err
			}
		}
	}

	if opts.JSON {
		return writeJSON(os.Stdout, created)
	}
	fmt.Println(created.ID)
	return nil
}

// runBulkCreate creates a container task with child tasks and dependency edges.
// This is the unified replacement for the old `ergo plan` command.
func runBulkCreate(dir string, opts GlobalOptions, input *TaskInput) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)

	var out bulkCreateOutput
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

		containerTitle := *input.Title
		containerBody := ""
		if input.Body != nil {
			containerBody = *input.Body
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
		workingIDs[containerID] = &Task{ID: containerID, IsEpic: true}
		createdAt := formatTime(now)
		containerEvent, err := newEvent("new_epic", now, NewTaskEvent{
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
			Children:  make([]bulkCreateChildOutput, 0, len(input.Tasks)),
			Edges:     make([]sequenceEdgeOutput, 0),
		}

		newEvents := make([]Event, 0, 1+len(input.Tasks))
		newEvents = append(newEvents, containerEvent)

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
	fmt.Printf("Created container %s: %s (%d tasks, %d dependencies)\n", out.ID, out.Title, len(out.Children), len(out.Edges))
	return nil
}
