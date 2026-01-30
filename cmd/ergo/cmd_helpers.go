// CLI helper utilities for error handling and output formatting.
// Keeps Cobra execution paths thin and consistent.
package main

import (
	"errors"
	"fmt"
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
	if opts == nil || !opts.Quiet {
		if strings.HasPrefix(err.Error(), "usage:") {
			fmt.Fprintln(os.Stderr, "hint: run `ergo --help`")
		} else if errors.Is(err, ergo.ErrNoErgoDir) {
			fmt.Fprintln(os.Stderr, "hint: run `ergo init` in your repo")
		} else if isPermissionError(err) {
			fmt.Fprintln(os.Stderr, "hint: permission error accessing .ergo/; check repo permissions (ergo needs read/write)")
		} else if strings.Contains(err.Error(), ".ergo") && strings.Contains(err.Error(), "exists but is not a directory") {
			fmt.Fprintln(os.Stderr, "hint: .ergo must be a directory; delete/rename the file and run `ergo init`")
		} else if errors.Is(err, ergo.ErrLockBusy) {
			fmt.Fprintln(os.Stderr, "hint: another process is writing; retry")
		}
	}
	os.Exit(1)
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
