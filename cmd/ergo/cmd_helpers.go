// CLI helper utilities for error handling and output formatting.
// Keeps Cobra execution paths thin and consistent.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
		} else if errors.Is(err, ergo.ErrLockBusy) {
			fmt.Fprintln(os.Stderr, "hint: another process is writing; retry")
		} else if strings.Contains(err.Error(), "require human") {
			fmt.Fprintln(os.Stderr, "hint: mark the task worker=human and ask a human to claim it")
		}
	}
	os.Exit(1)
}
