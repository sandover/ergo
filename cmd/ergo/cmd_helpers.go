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

func writeCLIError(w io.Writer, err error, args []string) {
	fmt.Fprintln(w, "error:", err)
	var removed *removedCommandError
	if handled := writeApplicationErrorHint(w, err, args); handled {
	} else if errors.As(err, &removed) {
		switch removed.command {
		case "set":
			fmt.Fprintln(w, "hint: use claim, done, block, cancel, release, title, body, or move")
		case "reopen":
			fmt.Fprintln(w, "hint: use claim <id> --agent <identity> to resume closed work")
		}
	} else if strings.HasPrefix(err.Error(), "usage:") {
		fmt.Fprintf(w, "hint: run `%s --help`\n", helpInvocation(args))
	} else if errors.Is(err, ergo.ErrNoErgoDir) {
		fmt.Fprintln(w, "hint: run `ergo init` or target an existing graph with `ergo --dir <path>`")
	} else if isPermissionError(err) {
		fmt.Fprintln(w, "hint: permission error accessing .ergo/; check repo permissions (ergo needs read/write)")
	} else if strings.Contains(err.Error(), ".ergo") && strings.Contains(err.Error(), "exists but is not a directory") {
		fmt.Fprintln(w, "hint: .ergo must be a directory; delete/rename the file and run `ergo init`")
	} else if errors.Is(err, ergo.ErrLockBusy) {
		fmt.Fprintln(w, "hint: another ergo process is still running; try again in a moment")
	}
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
	case ergo.ErrorInternal:
		if isPermissionError(err) {
			fmt.Fprintln(w, "hint: permission error accessing .ergo/; check repo permissions (ergo needs read/write)")
		}
	}
	return true
}

func helpInvocation(args []string) string {
	path, skip := "ergo", false
	for _, arg := range args {
		if skip {
			skip = false
			continue
		}
		if arg == "--dir" || arg == "--agent" {
			skip = true
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
	return os.IsPermission(err) || errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}
