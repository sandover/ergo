// Entry point and command dispatch, plus top-level error handling.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func main() {
	opts, args, err := parseGlobalOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}
	if args[0] == "--help" || args[0] == "help" {
		printUsage()
		return
	}
	if args[0] == "--version" || args[0] == "version" {
		printVersion()
		return
	}
	cmd := args[0]
	cmdArgs := args[1:]

	commands := map[string]func([]string, GlobalOptions) error{
		"init":       runInit,
		"new":        runNew,
		"list":       runList,
		"show":       runShow,
		"next":       runNext,
		"set":        runSet,
		"dep":        runDep,
		"where":      runWhere,
		"compact":    runCompact,
		"quickstart": wrapNoOpts(runQuickstart),
	}
	handler, ok := commands[cmd]
	if !ok {
		printUsage()
		os.Exit(1)
	}
	if err := handler(cmdArgs, opts); err != nil {
		exitErr(err, &opts)
	}
}

func wrapNoOpts(fn func([]string) error) func([]string, GlobalOptions) error {
	return func(args []string, _ GlobalOptions) error {
		return fn(args)
	}
}

func printUsage() {
	fmt.Println(usageText(stdoutIsTTY()))
}

func printVersion() {
	fmt.Println("ergo dev")
}

func exitErr(err error, opts *GlobalOptions) {
	fmt.Fprintln(os.Stderr, "error:", err)
	if opts == nil || !opts.Quiet {
		if strings.HasPrefix(err.Error(), "usage:") {
			fmt.Fprintln(os.Stderr, "hint: run `ergo --help`")
		} else if errors.Is(err, errNoErgoDir) {
			fmt.Fprintln(os.Stderr, "hint: run `ergo init` in your repo")
		} else if errors.Is(err, errLockBusy) {
			fmt.Fprintln(os.Stderr, "hint: another process is writing; retry or pass `--lock-timeout 30s`")
		} else if errors.Is(err, errLockTimeout) {
			fmt.Fprintln(os.Stderr, "hint: lock wait timed out; retry or increase `--lock-timeout`")
		} else if strings.Contains(err.Error(), "require human") {
			fmt.Fprintln(os.Stderr, "hint: run `ergo ready --as human` and ask the human to handle decision tasks")
		} else if strings.HasPrefix(err.Error(), "readonly:") {
			fmt.Fprintln(os.Stderr, "hint: remove `--readonly` (or switch to a read-only command)")
		}
	}
	os.Exit(1)
}
