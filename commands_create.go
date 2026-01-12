// Init/task creation commands and create-arg parsing.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	if len(args) < 2 {
		return errors.New("usage: ergo new epic <title> | ergo new task <title> [--epic <id>]")
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
	if len(args) < 1 {
		return errors.New("usage: ergo new epic <title>")
	}

	format, remaining, err := parseOutputFormatAndArgs(args, outputFormatText)
	if err != nil {
		return err
	}

	// Parse title as first positional arg, rest can be flags
	var title string
	var bodyFile string
	var positional []string

	for i := 0; i < len(remaining); i++ {
		arg := remaining[i]
		if strings.HasPrefix(arg, "-") {
			if arg == "--body-file" {
				if i+1 >= len(remaining) {
					return fmt.Errorf("missing value for %s", arg)
				}
				bodyFile = remaining[i+1]
				i++
			} else {
				return fmt.Errorf("unknown flag %s", arg)
			}
		} else {
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return errors.New("title required")
	}
	title = strings.TrimSpace(strings.Join(positional, " "))

	body, err := resolveBodyFromFile(bodyFile, title)
	if err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	created, err := createTask(dir, opts, "", true, body, workerAny)
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
	if len(args) < 1 {
		return errors.New("usage: ergo new task <title> [--epic <id>]")
	}

	format, remaining, err := parseOutputFormatAndArgs(args, outputFormatText)
	if err != nil {
		return err
	}

	// Parse title as first positional arg, rest can be flags
	var title string
	var epicID string
	var bodyFile string
	var worker Worker = workerAny
	var positional []string

	for i := 0; i < len(remaining); i++ {
		arg := remaining[i]
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "--epic":
				if i+1 >= len(remaining) {
					return fmt.Errorf("missing value for %s", arg)
				}
				epicID = remaining[i+1]
				i++
			case "--body-file":
				if i+1 >= len(remaining) {
					return fmt.Errorf("missing value for %s", arg)
				}
				bodyFile = remaining[i+1]
				i++
			case "--worker":
				if i+1 >= len(remaining) {
					return fmt.Errorf("missing value for %s", arg)
				}
				w, err := parseWorker(remaining[i+1])
				if err != nil {
					return err
				}
				worker = w
				i++
			default:
				return fmt.Errorf("unknown flag %s", arg)
			}
		} else {
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return errors.New("title required")
	}
	title = strings.TrimSpace(strings.Join(positional, " "))

	body, err := resolveBodyFromFile(bodyFile, title)
	if err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	created, err := createTask(dir, opts, epicID, false, body, worker)
	if err != nil {
		return err
	}

	if format == outputFormatJSON {
		return writeJSON(os.Stdout, created)
	}
	fmt.Println(created.ID)
	return nil
}

func resolveBodyFromFile(bodyFile, defaultBody string) (string, error) {
	if bodyFile != "" {
		return readBodyFile(bodyFile)
	}
	if stdinIsPiped() {
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	return defaultBody, nil
}
