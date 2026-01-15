// Init/task creation commands.
//
// Task and epic creation uses stdin-only JSON input:
//
//	echo '{"title":"Do X"}' | ergo new task
//	echo '{"title":"Auth system"}' | ergo new epic
//
// See json_input.go for the unified TaskInput schema.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func runInit(args []string, opts GlobalOptions) error {
	if err := requireWritable(opts, "init"); err != nil {
		return err
	}
	format, positional, err := parseOutputFormatAndArgs(args, outputFormatText)
	if err != nil {
		return err
	}
	dir := "."
	if len(positional) > 0 {
		dir = positional[0]
	}
	if len(positional) > 1 {
		return errors.New("usage: ergo init [dir]")
	}
	target := filepath.Join(dir, dataDirName)
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	eventsPath := filepath.Join(target, "events.jsonl")
	lockPath := filepath.Join(target, "lock")
	if _, err := os.Stat(eventsPath); err == nil {
		return fmt.Errorf("%s already exists", eventsPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(lockPath, []byte{}, 0644); err != nil {
		return err
	}
	if format == outputFormatJSON {
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

func runNew(args []string, opts GlobalOptions) error {
	if err := requireWritable(opts, "new"); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.New("usage: echo '{\"title\":\"...\"}' | ergo new task|epic")
	}

	subcommand := args[0]
	switch subcommand {
	case "epic":
		return runNewEpic(args[1:], opts)
	case "task":
		return runNewTask(args[1:], opts)
	default:
		return fmt.Errorf("unknown subcommand: %s (use 'epic' or 'task')", subcommand)
	}
}

func runNewEpic(args []string, opts GlobalOptions) error {
	format, _, err := parseOutputFormatAndArgs(args, outputFormatText)
	if err != nil {
		return err
	}

	// Parse JSON from stdin
	input, verr := ParseTaskInput()
	if verr != nil {
		if format == outputFormatJSON {
			verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	// Validate for epic creation
	if verr := input.ValidateForNewEpic(); verr != nil {
		if format == outputFormatJSON {
			verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	created, err := createTask(dir, opts, "", true, input.GetBody(), workerAny)
	if err != nil {
		return err
	}

	if format == outputFormatJSON {
		return writeJSON(os.Stdout, created)
	}
	fmt.Println(created.ID)
	return nil
}

func runNewTask(args []string, opts GlobalOptions) error {
	format, _, err := parseOutputFormatAndArgs(args, outputFormatText)
	if err != nil {
		return err
	}

	// Parse JSON from stdin
	input, verr := ParseTaskInput()
	if verr != nil {
		if format == outputFormatJSON {
			verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	// Validate for task creation
	if verr := input.ValidateForNewTask(); verr != nil {
		if format == outputFormatJSON {
			verr.WriteJSON(os.Stdout)
		}
		return verr.GoError()
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	// Create the task
	created, err := createTask(dir, opts, input.GetEpic(), false, input.GetBody(), input.GetWorker())
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
		delete(updates, "worker")

		if len(updates) > 0 {
			if err := applySetUpdates(dir, opts, created.ID, updates); err != nil {
				return err
			}
		}
	}

	if format == outputFormatJSON {
		return writeJSON(os.Stdout, created)
	}
	fmt.Println(created.ID)
	return nil
}
