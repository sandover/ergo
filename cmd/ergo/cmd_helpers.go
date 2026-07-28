// Purpose: Provide CLI error formatting, hints, and version output.
// Exports: none (package-private helpers).
// Role: Shared error/exit utilities for the cmd package.
// Invariants: exitErr always exits with code 1 after printing.
// Notes: Hints depend on error classification and global options.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/sandover/ergo/internal/ergo"
)

func printVersion() {
	fmt.Println("ergo " + version)
}

func exitErr(err error, opts *ergo.GlobalOptions) {
	fmt.Fprintln(os.Stderr, "error:", err)
	if handled := writeApplicationErrorHint(os.Stderr, err, os.Args[1:]); handled {
		// Classified application errors never fall through to text matching.
	} else if strings.Contains(err.Error(), `unknown command "set"`) {
		fmt.Fprintln(os.Stderr, "hint: use claim, done, block, cancel, release, title, body, or move")
	} else if strings.Contains(err.Error(), `unknown command "reopen"`) {
		fmt.Fprintln(os.Stderr, "hint: use claim <id> --agent <identity> to resume closed work")
	} else if strings.HasPrefix(err.Error(), "usage:") {
		fmt.Fprintf(os.Stderr, "hint: run `%s --help`\n", helpInvocation(os.Args[1:]))
	} else if errors.Is(err, ergo.ErrNoErgoDir) {
		fmt.Fprintln(os.Stderr, "hint: run `ergo init` or target an existing graph with `ergo --dir <path>`")
	} else if isPermissionError(err) {
		fmt.Fprintln(os.Stderr, "hint: permission error accessing .ergo/; check repo permissions (ergo needs read/write)")
	} else if strings.Contains(err.Error(), ".ergo") && strings.Contains(err.Error(), "exists but is not a directory") {
		fmt.Fprintln(os.Stderr, "hint: .ergo must be a directory; delete/rename the file and run `ergo init`")
	} else if errors.Is(err, ergo.ErrLockBusy) {
		fmt.Fprintln(os.Stderr, "hint: another ergo process is still running; try again in a moment")
	}
	os.Exit(1)
}

func writeApplicationErrorHint(w io.Writer, err error, args []string) bool {
	kind, ok := ergo.ApplicationErrorKind(err)
	if !ok {
		return false
	}
	switch kind {
	case ergo.ErrorUsage:
		fmt.Fprintf(w, "hint: run `%s --help`\n", helpInvocation(args))
	case ergo.ErrorNotFound:
		if errors.Is(err, ergo.ErrNoErgoDir) {
			fmt.Fprintln(w, "hint: run `ergo init` or target an existing graph with `ergo --dir <path>`")
		}
	case ergo.ErrorBusy:
		fmt.Fprintln(w, "hint: another ergo process is still running; try again in a moment")
	}
	return true
}

func helpInvocation(args []string) string {
	path := "ergo"
	skipValue := false
	for _, arg := range args {
		if skipValue {
			skipValue = false
			continue
		}
		if arg == "--dir" || arg == "--agent" {
			skipValue = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		path += " " + arg
		if arg != "new" {
			break
		}
	}
	return path
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) || errors.Is(err, os.ErrPermission) {
		return true
	}
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}
