// Purpose: Implement init and create commands for tasks/epics.
// Exports: RunInit, RunNewTask, RunNewEpic.
// Role: Command layer for creation workflows and repo initialization.
// Invariants: Writes are append-only under lock; create is safe under concurrent writers.
// Notes: New tasks start in todo state; epics cannot nest.
//
// Task and epic creation supports multiple input styles:
// - JSON object on stdin (default)
// - Flags-only input (e.g. --title/--body) when stdin is a TTY
// - `--body-stdin` to treat stdin as literal body text (metadata via flags)
//
//	echo '{"title":"Do X"}' | ergo new task
//	echo '{"title":"Auth system"}' | ergo new epic
//
// See json_input.go for the unified TaskInput schema.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	eventsPath := filepath.Join(target, "events.jsonl")
	lockPath := filepath.Join(target, "lock")
	if err := ensureFileExists(eventsPath, 0644); err != nil {
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

func RunNewEpic(opts GlobalOptions) error {
	if opts.BodyStdin {
		if err := validateBodyStdinExclusions(opts.BodyFlag); err != nil {
			return err
		}
		title := strings.TrimSpace(opts.TitleFlag)
		if title == "" {
			return errors.New("new epic --body-stdin requires --title")
		}
		body, err := readBodyFromStdinOrEmpty()
		if err != nil {
			return err
		}

		dir, err := ergoDir(opts)
		if err != nil {
			return err
		}
		created, err := createTask(dir, opts, "", true, title, body)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writeJSON(os.Stdout, created)
		}
		fmt.Println(created.ID)
		return nil
	}

	if !stdinIsPiped() && strings.TrimSpace(opts.TitleFlag) != "" {
		title := strings.TrimSpace(opts.TitleFlag)

		dir, err := ergoDir(opts)
		if err != nil {
			return err
		}
		created, err := createTask(dir, opts, "", true, title, opts.BodyFlag)
		if err != nil {
			return err
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
	if verr := input.ValidateForNewEpic(); verr != nil {
		if opts.JSON {
			if err := verr.WriteJSON(os.Stdout); err != nil {
				return err
			}
		}
		return verr.GoError()
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	created, err := createTask(dir, opts, "", true, input.GetTitle(), input.GetBody())
	if err != nil {
		return err
	}

	if opts.JSON {
		return writeJSON(os.Stdout, created)
	}
	fmt.Println(created.ID)
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
