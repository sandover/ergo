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
			fmt.Fprintln(os.Stderr, "hint: another process is writing; retry or pass `--lock-timeout 30s`")
		} else if errors.Is(err, ergo.ErrLockTimeout) {
			fmt.Fprintln(os.Stderr, "hint: lock wait timed out; retry or increase `--lock-timeout`")
		} else if strings.Contains(err.Error(), "require human") {
			fmt.Fprintln(os.Stderr, "hint: run `ergo ready --as human` and ask the human to handle decision tasks")
		} else if strings.HasPrefix(err.Error(), "readonly:") {
			fmt.Fprintln(os.Stderr, "hint: remove `--readonly` (or switch to a read-only command)")
		}
	}
	os.Exit(1)
}
